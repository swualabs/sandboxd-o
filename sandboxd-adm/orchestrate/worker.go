package orchestrate

import (
	"context"
	"fmt"
	"strings"
	"time"

	"sandboxd-o/sandboxd-adm/awsx"
	"sandboxd-o/sandboxd-adm/stepper"
	"sandboxd-o/sandboxd-adm/store"
	"sandboxd-o/sandboxd-adm/userdata"
	"sandboxd-o/sandboxd-ctl/client"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
)

const defaultWorkerRootVolumeGiB = 64

type CreateWorkerInput struct {
	Name         string
	Version      string
	ClusterName  string
	InstanceType string
	RootVolume   string // e.g. "64Gi"
	External     string // optional hostname; defaults to the worker's EIP when left empty
	PublicEIP    string // ARN or allocation id; empty means "auto-allocate a managed EIP"
	ConfigPath   string // optional JSON overrides file
	ECRRepos     string // comma-separated ECR repo name patterns to grant pull access to, e.g. "my-repo-1,ctf-*"
	OrchServer   string // explicit orch base URL override (SBXADM_ORCH_SERVER)
	OrchTimeout  time.Duration
}

func CreateWorker(ctx context.Context, ec2c *ec2.Client, iamc *iam.Client, ssmc *ssm.Client, st *store.Store, accountID string, in CreateWorkerInput, s *stepper.Stepper) error {
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return fmt.Errorf("worker name is required")
	}

	newECRPatterns, err := awsx.ParseECRRepoPatterns(in.ECRRepos)
	if err != nil {
		return fmt.Errorf("--ecr-repos: %w", err)
	}

	s.Step("loading cluster %q", in.ClusterName)
	cluster, err := st.GetCluster(ctx, in.ClusterName)
	if err != nil {
		return err
	}

	for _, w := range cluster.Workers {
		if w.Name == name {
			return fmt.Errorf("worker %q already exists in cluster %q", name, in.ClusterName)
		}
	}

	if len(cluster.PublicSubnets) == 0 {
		return fmt.Errorf("cluster %q has no public subnets recorded; workers always launch in a public subnet", in.ClusterName)
	}
	s.Done("cluster %q has %d existing worker(s)", in.ClusterName, len(cluster.Workers))

	rootGiB := int32(defaultWorkerRootVolumeGiB)
	if strings.TrimSpace(in.RootVolume) != "" {
		rootGiB, err = awsx.ParseSizeGiB(in.RootVolume)
		if err != nil {
			return fmt.Errorf("--root-volume-size: %w", err)
		}
	}

	mergedECRPatterns := cluster.WorkerECRRepoPatterns
	if len(newECRPatterns) > 0 {
		mergedECRPatterns = awsx.MergeECRRepoPatterns(cluster.WorkerECRRepoPatterns, newECRPatterns)
		s.Step("granting ECR pull access for repositories: %v", mergedECRPatterns)
		workerRoleBase := fmt.Sprintf("sbxcluster-%s-worker", in.ClusterName)
		if err := awsx.PutECRPullPolicy(ctx, iamc, workerRoleBase, cluster.Region, accountID, mergedECRPatterns); err != nil {
			return fmt.Errorf("grant ecr pull access: %w", err)
		}
		s.Done("ECR pull allowlist updated for cluster %q (shared by all its workers)", in.ClusterName)
	}

	var eipAllocID string
	if in.PublicEIP != "" {
		eipAllocID, err = awsx.AllocationIDFromARNOrID(in.PublicEIP)
		if err != nil {
			return fmt.Errorf("--public-eip: %w", err)
		}
	}

	var rollback rollbackStack
	defer rollback.run(s)

	var eipManaged bool
	var preAssociatedIP string
	if eipAllocID == "" {
		s.Step("allocating Elastic IP for worker %q", name)
		allocID, ip, err := awsx.AllocateEIP(ctx, ec2c, fmt.Sprintf("sbxcluster-%s-worker-%s-eip", in.ClusterName, name))
		if err != nil {
			return fmt.Errorf("allocate worker eip: %w", err)
		}

		eipAllocID = allocID
		eipManaged = true
		preAssociatedIP = ip
		rollback.add(fmt.Sprintf("release EIP %s", eipAllocID), func(ctx context.Context) error {
			return awsx.ReleaseEIP(ctx, ec2c, eipAllocID)
		})
		s.Done("allocated eip %s (%s)", eipAllocID, ip)
	}

	external := strings.TrimSpace(in.External)

	s.Step("resolving latest Ubuntu 22.04 AMI in %s", cluster.Region)
	amiID, err := awsx.LatestUbuntuAMI(ctx, ec2c)
	if err != nil {
		return err
	}
	s.Done("ami=%s", amiID)

	configJSON, err := userdata.MergeConfig("sbxlet", in.ConfigPath, cluster.SharedSecret, nil)
	if err != nil {
		return fmt.Errorf("build sbxlet config: %w", err)
	}

	script, err := userdata.Render(userdata.Params{Component: "sbxlet", Version: in.Version, ConfigJSON: configJSON})
	if err != nil {
		return fmt.Errorf("render sbxlet userdata: %w", err)
	}

	subnetID := cluster.PublicSubnets[len(cluster.Workers)%len(cluster.PublicSubnets)]

	s.Step("launching worker instance (type=%s subnet=%s)", in.InstanceType, subnetID)
	instanceID, privateIP, publicIP, err := awsx.LaunchInstance(ctx, ec2c, amiID, awsx.LaunchSpec{
		Name:               fmt.Sprintf("sbxcluster-%s-worker-%s", in.ClusterName, name),
		InstanceType:       in.InstanceType,
		SubnetID:           subnetID,
		SecurityGroupIDs:   []string{cluster.WorkerSecurityGroup},
		RootVolumeSizeGB:   rootGiB,
		AssignPublicIP:     false, // always EIP-backed now, see allocation above
		UserData:           script,
		IAMInstanceProfile: cluster.WorkerInstanceProfile,
		Tags:               map[string]string{"sbxadm/cluster": in.ClusterName, "sbxadm/role": "worker", "sbxadm/worker": name},
	})
	if err != nil {
		return fmt.Errorf("launch worker instance: %w", err)
	}

	rollback.add(fmt.Sprintf("terminate instance %s", instanceID), func(ctx context.Context) error {
		return awsx.TerminateInstance(ctx, ec2c, instanceID)
	})
	s.Done("instance %s running (private_ip=%s)", instanceID, privateIP)

	s.Step("associating Elastic IP %s", eipAllocID)
	publicIP, err = awsx.AssociateEIPByAllocationID(ctx, ec2c, instanceID, eipAllocID)
	if err != nil {
		return err
	}

	if !eipManaged {
		rollback.add(fmt.Sprintf("disassociate EIP %s", eipAllocID), func(ctx context.Context) error {
			return awsx.DisassociateEIP(ctx, ec2c, eipAllocID)
		})
	}

	if publicIP == "" {
		publicIP = preAssociatedIP
	}
	s.Done("eip associated public_ip=%s", publicIP)

	if external == "" {
		external = publicIP
		s.Info("no --external given; registering External as the worker's own EIP (%s)", external)
	}

	s.Step("waiting for SSM agent to come online on %s", instanceID)
	if err := awsx.WaitForSSMOnline(ctx, ssmc, instanceID, ssmOnlineTimeout); err != nil {
		return fmt.Errorf("sbxlet instance never reached SSM: %w", err)
	}
	s.Done("SSM agent online")

	s.Step("waiting for sbxlet.service health check (up to %s; installs gVisor/containerd first)", letHealthTimeout)
	if err := awsx.WaitForLocalHealthz(ctx, ssmc, instanceID, letAPIPort, letHealthTimeout); err != nil {
		return fmt.Errorf("sbxlet health check failed: %w (tail /var/log/sbxadm-userdata.log on the instance via SSM for details)", err)
	}
	s.Done("sbxlet is healthy")

	orchServer := resolveOrchServer(in.OrchServer, *cluster)
	if orchServer != "" {
		s.Step("registering Node object %q with orchestrator at %s", name, orchServer)
		oc := client.New(orchServer, orDefault(in.OrchTimeout, 15*time.Second), cluster.SharedSecret)
		nodeCtx, cancel := context.WithTimeout(ctx, orDefault(in.OrchTimeout, 15*time.Second))
		_, err = oc.CreateNodeObject(nodeCtx, map[string]any{
			"id": name,
			"spec": map[string]any{
				"ip":   privateIP,
				"port": letAPIPort,
			},
		})
		cancel()
		if err != nil {
			return fmt.Errorf("register node %q with orchestrator: %w", name, err)
		}
		rollback.add(fmt.Sprintf("delete node object %s", name), func(ctx context.Context) error {
			_, err := oc.DeleteNodeWithForce(ctx, name, true)
			return err
		})
		s.Done("node %q registered (ip=%s port=%d)", name, privateIP, letAPIPort)

		s.Step("registering External object for %q -> %s", name, external)
		extCtx, cancel := context.WithTimeout(ctx, orDefault(in.OrchTimeout, 15*time.Second))
		_, err = oc.CreateExternalObject(extCtx, map[string]any{
			"id": fmt.Sprintf("%s-external", name),
			"spec": map[string]any{
				"node_id":  name,
				"external": external,
			},
		})

		cancel()
		if err != nil {
			return fmt.Errorf("register external for node %q: %w", name, err)
		}
		s.Done("external %q registered", external)
	} else {
		s.Warn("no orchestrator server configured (set SBXADM_ORCH_SERVER or --orch-server); skipping Node/External registration")
	}

	now := time.Now().UTC()
	worker := store.Worker{
		Name:             name,
		InstanceID:       instanceID,
		InstanceType:     in.InstanceType,
		SubnetID:         subnetID,
		SecurityGroup:    cluster.WorkerSecurityGroup,
		PublicEIPAllocID: eipAllocID,
		PublicEIPManaged: eipManaged,
		PublicIP:         publicIP,
		PrivateIP:        privateIP,
		RootVolumeSizeGB: rootGiB,
		External:         external,
		ConfigJSON:       configJSON,
		CreatedAt:        now,
		UpdatedAt:        now,
	}

	cluster.Workers = append(cluster.Workers, worker)
	cluster.WorkerECRRepoPatterns = mergedECRPatterns
	cluster.UpdatedAt = now

	s.Step("persisting worker state to DynamoDB")
	if err := st.SaveCluster(ctx, *cluster); err != nil {
		return fmt.Errorf("persist worker: %w", err)
	}
	s.Done("worker %q persisted under cluster %q", name, in.ClusterName)

	rollback.clear()
	return nil
}

func DeleteWorker(ctx context.Context, ec2c *ec2.Client, st *store.Store, clusterName, workerName string, orchServer string, orchTimeout time.Duration, s *stepper.Stepper) error {
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
	s.Done("found worker %q (instance=%s)", workerName, worker.InstanceID)

	orchSrv := resolveOrchServer(orchServer, *cluster)
	if orchSrv != "" {
		s.Step("removing Node/External objects from orchestrator")
		oc := client.New(orchSrv, orDefault(orchTimeout, 15*time.Second), cluster.SharedSecret)
		if worker.External != "" {
			extCtx, cancel := context.WithTimeout(ctx, orDefault(orchTimeout, 15*time.Second))
			if _, err := oc.DeleteExternal(extCtx, fmt.Sprintf("%s-external", workerName)); err != nil {
				s.Warn("delete external object: %v", err)
			}
			cancel()
		}

		nodeCtx, cancel := context.WithTimeout(ctx, orDefault(orchTimeout, 15*time.Second))
		if _, err := oc.DeleteNodeWithForce(nodeCtx, workerName, true); err != nil {
			s.Warn("delete node object: %v", err)
		}
		cancel()
		s.Done("orchestrator objects removed")
	} else {
		s.Warn("no orchestrator server configured; skipping Node/External cleanup on the orchestrator")
	}

	s.Step("terminating worker instance %s", worker.InstanceID)
	if err := teardownWorker(ctx, ec2c, worker); err != nil {
		return err
	}
	s.Done("worker instance terminated")

	cluster.Workers = append(cluster.Workers[:idx], cluster.Workers[idx+1:]...)
	cluster.UpdatedAt = time.Now().UTC()

	s.Step("persisting cluster state to DynamoDB")
	if err := st.SaveCluster(ctx, *cluster); err != nil {
		return fmt.Errorf("persist cluster after worker deletion: %w", err)
	}
	s.Done("worker %q removed from cluster %q", workerName, clusterName)

	return nil
}

func teardownWorker(ctx context.Context, ec2c *ec2.Client, w store.Worker) error {
	if w.PublicEIPAllocID != "" {
		if w.PublicEIPManaged {
			if err := awsx.ReleaseEIP(ctx, ec2c, w.PublicEIPAllocID); err != nil {
				return fmt.Errorf("release eip: %w", err)
			}
		} else if err := awsx.DisassociateEIP(ctx, ec2c, w.PublicEIPAllocID); err != nil {
			return fmt.Errorf("disassociate eip: %w", err)
		}
	}
	return awsx.TerminateInstance(ctx, ec2c, w.InstanceID)
}

func resolveOrchServer(explicit string, cluster store.Cluster) string {
	if strings.TrimSpace(explicit) != "" {
		return explicit
	}
	if cluster.ControlPlane.PublicIP != "" {
		return fmt.Sprintf("http://%s:%d", cluster.ControlPlane.PublicIP, orchAPIPort)
	}
	if cluster.ControlPlane.PrivateIP != "" {
		return fmt.Sprintf("http://%s:%d", cluster.ControlPlane.PrivateIP, orchAPIPort)
	}
	return ""
}

func orDefault(d, def time.Duration) time.Duration {
	if d <= 0 {
		return def
	}
	return d
}
