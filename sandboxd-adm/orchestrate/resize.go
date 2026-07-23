package orchestrate

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"sandboxd-o/sandboxd-adm/awsx"
	"sandboxd-o/sandboxd-adm/stepper"
	"sandboxd-o/sandboxd-adm/store"
	"sandboxd-o/sandboxd-ctl/client"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
)

type ResizeControlPlaneInput struct {
	ClusterName  string
	InstanceType string
}

type ResizeWorkerInput struct {
	ClusterName  string
	WorkerName   string
	InstanceType string
	OrchServer   string
	OrchTimeout  time.Duration
}

func ResizeControlPlane(ctx context.Context, ec2c *ec2.Client, ssmc *ssm.Client, st *store.Store, in ResizeControlPlaneInput, s *stepper.Stepper) error {
	clusterName := strings.TrimSpace(in.ClusterName)
	instanceType := strings.TrimSpace(in.InstanceType)
	if clusterName == "" {
		return fmt.Errorf("cluster name is required")
	}

	if instanceType == "" {
		return fmt.Errorf("instance type is required")
	}

	s.Step("loading cluster %q", clusterName)
	cluster, err := st.GetCluster(ctx, clusterName)
	if err != nil {
		return err
	}

	cp := cluster.ControlPlane
	if cp.InstanceType == instanceType {
		s.Done("control plane already uses instance type %q", instanceType)
		return nil
	}

	s.Done("found control plane instance=%s current_type=%s target_type=%s", cp.InstanceID, cp.InstanceType, instanceType)
	s.Warn("resizing the control plane stops and starts the EC2 instance; sbxorch API will be unavailable during this operation")

	s.Step("validating target instance type %q", instanceType)
	if _, err := awsx.InstanceTypeArch(ctx, ec2c, instanceType); err != nil {
		return err
	}
	s.Done("target instance type is valid")

	s.Step("stopping, modifying, and starting control plane instance %s", cp.InstanceID)
	if err := awsx.ResizeInstanceType(ctx, ec2c, cp.InstanceID, instanceType); err != nil {
		return err
	}

	s.Done("control plane instance restarted")

	privateIP, publicIP, err := awsx.DescribeInstanceNetwork(ctx, ec2c, cp.InstanceID)
	if err != nil {
		return err
	}
	cp.PrivateIP = privateIP
	cp.PublicIP = publicIP
	cp.InstanceType = instanceType

	s.Step("waiting for SSM agent to come online on %s", cp.InstanceID)
	if err := awsx.WaitForSSMOnline(ctx, ssmc, cp.InstanceID, ssmOnlineTimeout); err != nil {
		return fmt.Errorf("control plane instance never reached SSM after resize: %w", err)
	}

	s.Done("SSM agent online")

	s.Step("waiting for sbxorch.service health check (up to %s)", orchHealthTimeout)
	if err := awsx.WaitForLocalHealthz(ctx, ssmc, cp.InstanceID, orchAPIPort, orchHealthTimeout); err != nil {
		return fmt.Errorf("sbxorch health check failed after resize: %w", err)
	}

	s.Done("sbxorch is healthy")

	cluster.ControlPlane = cp
	cluster.UpdatedAt = time.Now().UTC()

	s.Step("persisting resized control plane state to DynamoDB")
	if err := st.SaveCluster(ctx, *cluster); err != nil {
		return fmt.Errorf("persist cluster after control plane resize: %w", err)
	}

	s.Done("control plane resized to %s", instanceType)

	return nil
}

func ResizeWorker(ctx context.Context, ec2c *ec2.Client, ssmc *ssm.Client, st *store.Store, in ResizeWorkerInput, s *stepper.Stepper) error {
	clusterName := strings.TrimSpace(in.ClusterName)
	workerName := strings.TrimSpace(in.WorkerName)
	instanceType := strings.TrimSpace(in.InstanceType)
	if clusterName == "" {
		return fmt.Errorf("cluster name is required")
	}

	if workerName == "" {
		return fmt.Errorf("worker name is required")
	}

	if instanceType == "" {
		return fmt.Errorf("instance type is required")
	}

	s.Step("loading cluster %q", clusterName)
	cluster, err := st.GetCluster(ctx, clusterName)
	if err != nil {
		return err
	}

	idx := -1
	for i, w := range cluster.Workers {
		if w.Name == workerName {
			idx = i
			break
		}
	}

	if idx < 0 {
		return fmt.Errorf("worker %q not found in cluster %q", workerName, clusterName)
	}

	worker := cluster.Workers[idx]
	oldPublicIP := worker.PublicIP
	if worker.InstanceType == instanceType {
		s.Done("worker %q already uses instance type %q", workerName, instanceType)
		return nil
	}

	s.Done("found worker %q instance=%s current_type=%s target_type=%s", workerName, worker.InstanceID, worker.InstanceType, instanceType)
	s.Warn("resizing a worker stops and starts the EC2 instance; sandboxes running on this worker will experience downtime or may need to be recreated")

	s.Step("validating target instance type %q", instanceType)
	if _, err := awsx.InstanceTypeArch(ctx, ec2c, instanceType); err != nil {
		return err
	}

	s.Done("target instance type is valid")

	s.Step("stopping, modifying, and starting worker instance %s", worker.InstanceID)
	if err := awsx.ResizeInstanceType(ctx, ec2c, worker.InstanceID, instanceType); err != nil {
		return err
	}

	s.Done("worker instance restarted")

	privateIP, publicIP, err := awsx.DescribeInstanceNetwork(ctx, ec2c, worker.InstanceID)
	if err != nil {
		return err
	}

	worker.PrivateIP = privateIP
	worker.PublicIP = publicIP
	worker.InstanceType = instanceType
	worker.UpdatedAt = time.Now().UTC()

	if shouldRefreshWorkerExternal(worker.PublicEIPAllocID, worker.External, oldPublicIP, publicIP) {
		orchSrv := resolveOrchServer(in.OrchServer, *cluster)
		if orchSrv == "" {
			s.Warn("worker public IP changed from %s to %s, but no orchestrator server is configured; External object was not refreshed", oldPublicIP, publicIP)
		} else {
			s.Step("refreshing auto External object for %q (%s -> %s)", workerName, oldPublicIP, publicIP)
			oc := client.New(orchSrv, orDefault(in.OrchTimeout, 15*time.Second), cluster.SharedSecret)
			req := map[string]any{
				"id": fmt.Sprintf("%s-external", workerName),
				"spec": map[string]any{
					"node_id":  workerName,
					"external": publicIP,
				},
			}

			extCtx, cancel := context.WithTimeout(ctx, orDefault(in.OrchTimeout, 15*time.Second))
			_, err := oc.CreateExternalObject(extCtx, req)
			cancel()

			if err != nil {
				return fmt.Errorf("refresh external object after worker resize: %w", err)
			}

			worker.External = publicIP
			s.Done("external object refreshed")
		}
	}

	s.Step("waiting for SSM agent to come online on %s", worker.InstanceID)
	if err := awsx.WaitForSSMOnline(ctx, ssmc, worker.InstanceID, ssmOnlineTimeout); err != nil {
		return fmt.Errorf("worker instance never reached SSM after resize: %w", err)
	}

	s.Done("SSM agent online")

	s.Step("waiting for sbxlet.service health check (up to %s)", letHealthTimeout)
	if err := awsx.WaitForLocalHealthz(ctx, ssmc, worker.InstanceID, letAPIPort, letHealthTimeout); err != nil {
		return fmt.Errorf("sbxlet health check failed after resize: %w", err)
	}

	s.Done("sbxlet is healthy")

	cluster.Workers[idx] = worker
	cluster.UpdatedAt = time.Now().UTC()

	s.Step("persisting resized worker state to DynamoDB")
	if err := st.SaveCluster(ctx, *cluster); err != nil {
		return fmt.Errorf("persist cluster after worker resize: %w", err)
	}

	s.Done("worker %q resized to %s", workerName, instanceType)

	return nil
}

func shouldRefreshWorkerExternal(publicEIPAllocID, external, oldPublicIP, newPublicIP string) bool {
	if publicEIPAllocID != "" || newPublicIP == "" || newPublicIP == oldPublicIP {
		return false
	}

	external = strings.TrimSpace(external)
	if external == "" || external == oldPublicIP {
		return true
	}

	// Auto-assigned public-IP workers cannot keep a literal-IP External stable
	// across stop/start. Hostnames are treated as user-managed DNS and left intact.
	return net.ParseIP(external) != nil
}
