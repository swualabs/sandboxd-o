package orchestrate

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"sandboxd-o/sandboxd-adm/awsx"
	"sandboxd-o/sandboxd-adm/stepper"
	"sandboxd-o/sandboxd-adm/store"
	"sandboxd-o/sandboxd-adm/userdata"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
)

const (
	defaultOrchRootVolumeGiB = 16
	orchAPIPort              = 8082
	letAPIPort               = 8081
	workerPortRangeFrom      = 10000
	workerPortRangeTo        = 32767

	ssmOnlineTimeout  = 3 * time.Minute
	orchHealthTimeout = 6 * time.Minute
	letHealthTimeout  = 10 * time.Minute // sbxlet also installs gVisor/containerd, slower
)

type CreateClusterInput struct {
	Name               string
	Version            string
	VPCID              string
	PublicSubnetIDs    []string
	PrivateSubnetIDs   []string
	Region             string
	OrchInstanceType   string
	OrchPublicEndpoint bool
	OrchPublicEIP      string // ARN or allocation id; empty means "auto IP, may change on restart"
	OrchRootVolume     string // e.g. "16Gi"
	OrchConfigPath     string // optional JSON overrides file
	SharedSecret       string // optional explicit cluster shared secret; empty means random
}

func CreateCluster(ctx context.Context, ec2c *ec2.Client, iamc *iam.Client, ssmc *ssm.Client, st *store.Store, in CreateClusterInput, s *stepper.Stepper) error {
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return fmt.Errorf("cluster name is required")
	}

	if in.OrchPublicEIP != "" && !in.OrchPublicEndpoint {
		return fmt.Errorf("--orch-public-eip requires --orch-public-endpoint")
	}

	s.Step("validating cluster does not already exist (name=%s)", name)
	if _, err := st.GetCluster(ctx, name); err == nil {
		return fmt.Errorf("cluster %q already exists", name)
	} else if err != store.ErrClusterNotFound {
		return fmt.Errorf("check existing cluster: %w", err)
	}

	s.Done("no existing cluster named %q", name)

	s.Step("validating VPC %s and subnets", in.VPCID)
	vpcCIDR, err := awsx.ValidateVPCAndGetCIDR(ctx, ec2c, in.VPCID)
	if err != nil {
		return err
	}

	if _, err := awsx.ValidateSubnets(ctx, ec2c, in.VPCID, in.PublicSubnetIDs); err != nil {
		return fmt.Errorf("public subnets: %w", err)
	}

	if _, err := awsx.ValidateSubnets(ctx, ec2c, in.VPCID, in.PrivateSubnetIDs); err != nil {
		return fmt.Errorf("private subnets: %w", err)
	}
	s.Done("vpc=%s cidr=%s public_subnets=%v private_subnets=%v", in.VPCID, vpcCIDR, in.PublicSubnetIDs, in.PrivateSubnetIDs)

	s.Step("checking public subnets have an internet gateway route")
	for _, sn := range in.PublicSubnetIDs {
		hasIGW, err := awsx.SubnetHasIGWRoute(ctx, ec2c, in.VPCID, sn)
		if err != nil {
			return fmt.Errorf("check public subnet igw route: %w", err)
		}
		if !hasIGW {
			return fmt.Errorf(
				"subnet %s was passed as --public-subnet but has no 0.0.0.0/0 route to an internet gateway; "+
					"workers (and a --orch-public-endpoint control plane) launch here and would never finish booting. "+
					"Attach an internet gateway with a default route to this subnet, or pass a real public subnet",
				sn,
			)
		}
	}
	s.Done("public subnets have internet gateway egress")

	if !in.OrchPublicEndpoint {
		targetPrivateSubnet := pickSubnet(in.PrivateSubnetIDs)
		s.Step("checking internet/SSM egress for private subnet %s", targetPrivateSubnet)
		egress, err := awsx.CheckPrivateEgress(ctx, ec2c, in.VPCID, targetPrivateSubnet)
		if err != nil {
			return fmt.Errorf("check private subnet egress: %w", err)
		}

		if !egress.Sufficient() {
			return fmt.Errorf(
				"private subnet %s has no route to a NAT gateway, so the control plane would never finish booting "+
					"(it needs internet egress for both the SSM agent and to download the sandboxd-o release from GitHub). "+
					"SSM VPC endpoints present: %v (not sufficient on their own -- the release download still needs general internet egress). "+
					"Add a NAT gateway with a 0.0.0.0/0 route in this subnet's route table, or pass --orch-public-endpoint instead",
				targetPrivateSubnet, egress.HasSSMVPCEndpoints,
			)
		}

		s.Done("private subnet %s has NAT gateway egress", targetPrivateSubnet)
	}

	rootGiB := int32(defaultOrchRootVolumeGiB)
	if strings.TrimSpace(in.OrchRootVolume) != "" {
		rootGiB, err = awsx.ParseSizeGiB(in.OrchRootVolume)
		if err != nil {
			return fmt.Errorf("--orch-root-volume-size: %w", err)
		}
	}

	var eipAllocID string
	var eipManaged bool
	if in.OrchPublicEIP != "" {
		eipAllocID, err = awsx.AllocationIDFromARNOrID(in.OrchPublicEIP)
		if err != nil {
			return fmt.Errorf("--orch-public-eip: %w", err)
		}
	}

	var rollback rollbackStack
	defer rollback.run(s)

	if in.OrchPublicEndpoint && eipAllocID == "" {
		s.Step("allocating Elastic IP for control plane")
		allocID, ip, err := awsx.AllocateEIP(ctx, ec2c, fmt.Sprintf("sbxcluster-%s-orch-eip", name))
		if err != nil {
			return fmt.Errorf("allocate control plane eip: %w", err)
		}

		eipAllocID = allocID
		eipManaged = true
		rollback.add(fmt.Sprintf("release EIP %s", eipAllocID), func(ctx context.Context) error {
			return awsx.ReleaseEIP(ctx, ec2c, eipAllocID)
		})

		s.Done("allocated eip %s (%s)", eipAllocID, ip)
	}

	orchSGName := fmt.Sprintf("sbxcluster-%s-orch-sg", name)
	workerSGName := fmt.Sprintf("sbxcluster-%s-worker-sg", name)

	// The orch SG is created first so the worker SG can reference it in an
	// ingress rule. Its rollback is registered before the worker SG's, so
	// the LIFO rollback deletes the worker SG (which references the orch
	// SG) first, avoiding a DependencyViolation. Each rollback is
	// registered immediately after creation, so no SG can leak.
	s.Step("creating security group %s", orchSGName)
	orchOpenCIDR := vpcCIDR
	if in.OrchPublicEndpoint {
		orchOpenCIDR = "0.0.0.0/0"
	}

	orchSGID, err := awsx.EnsureSecurityGroup(ctx, ec2c, in.VPCID, orchSGName, fmt.Sprintf("sbxadm control plane SG for cluster %s", name), []awsx.IngressRule{
		{Port: orchAPIPort, CIDR: orchOpenCIDR, Description: "sbxorch api (sbxctl/sbxadm)"},
	})
	if err != nil {
		return fmt.Errorf("create orch security group: %w", err)
	}

	rollback.add(fmt.Sprintf("delete security group %s", orchSGID), func(ctx context.Context) error {
		return awsx.DeleteSecurityGroup(ctx, ec2c, orchSGID)
	})
	s.Done("security group %s ready", orchSGID)

	s.Step("creating security group %s", workerSGName)
	workerSGID, err := awsx.EnsureSecurityGroup(ctx, ec2c, in.VPCID, workerSGName, fmt.Sprintf("sbxadm worker node SG for cluster %s", name), []awsx.IngressRule{
		{Port: workerPortRangeFrom, ToPort: workerPortRangeTo, CIDR: "0.0.0.0/0", Description: "sandbox workload public ports"},
		{Port: letAPIPort, SourceSGID: orchSGID, Description: "sbxlet api from control plane"},
	})
	if err != nil {
		return fmt.Errorf("create worker security group: %w", err)
	}

	rollback.add(fmt.Sprintf("delete security group %s", workerSGID), func(ctx context.Context) error {
		return awsx.DeleteSecurityGroup(ctx, ec2c, workerSGID)
	})
	s.Done("security group %s ready", workerSGID)

	s.Step("creating IAM role/instance profile for control plane (SSM-managed, no SSH)")
	cpProfile, err := awsx.EnsureInstanceProfile(ctx, iamc, fmt.Sprintf("sbxcluster-%s-control-plane", name), nil)
	if err != nil {
		return fmt.Errorf("control plane instance profile: %w", err)
	}

	rollback.add(fmt.Sprintf("delete instance profile %s", cpProfile), func(ctx context.Context) error {
		return awsx.DeleteInstanceProfile(ctx, iamc, fmt.Sprintf("sbxcluster-%s-control-plane", name), nil)
	})
	s.Done("control plane instance profile %s ready", cpProfile)

	s.Step("creating IAM role/instance profile for workers (SSM-managed, no SSH)")
	workerProfile, err := awsx.EnsureInstanceProfile(ctx, iamc, fmt.Sprintf("sbxcluster-%s-worker", name), nil)
	if err != nil {
		return fmt.Errorf("worker instance profile: %w", err)
	}

	rollback.add(fmt.Sprintf("delete instance profile %s", workerProfile), func(ctx context.Context) error {
		return awsx.DeleteInstanceProfile(ctx, iamc, fmt.Sprintf("sbxcluster-%s-worker", name), nil)
	})
	s.Done("worker instance profile %s ready", workerProfile)

	s.Step("resolving latest Ubuntu 22.04 AMI for %s in %s", in.OrchInstanceType, in.Region)
	amiID, err := awsx.LatestUbuntuAMIForInstanceType(ctx, ec2c, in.OrchInstanceType)
	if err != nil {
		return err
	}
	s.Done("ami=%s", amiID)

	sharedSecret, userProvidedSecret, err := resolveClusterSharedSecret(in.SharedSecret)
	if err != nil {
		return fmt.Errorf("resolve shared secret: %w", err)
	}

	if userProvidedSecret {
		s.Warn("using operator-supplied shared secret for cluster auth; treat this as sensitive and prefer the default random secret unless you have a strong operational reason")
	}

	configJSON, err := userdata.MergeConfig("sbxorch", in.OrchConfigPath, sharedSecret, nil, nil)
	if err != nil {
		return fmt.Errorf("build sbxorch config: %w", err)
	}

	script, err := userdata.Render(userdata.Params{Component: "sbxorch", Version: in.Version, ConfigJSON: configJSON})
	if err != nil {
		return fmt.Errorf("render sbxorch userdata: %w", err)
	}

	subnetID := pickSubnet(in.PrivateSubnetIDs)
	if in.OrchPublicEndpoint {
		subnetID = pickSubnet(in.PublicSubnetIDs)
	}

	s.Step("launching control plane instance (type=%s subnet=%s public=%v)", in.OrchInstanceType, subnetID, in.OrchPublicEndpoint)
	instanceID, privateIP, publicIP, err := awsx.LaunchInstance(ctx, ec2c, amiID, awsx.LaunchSpec{
		Name:             fmt.Sprintf("sbxcluster-%s-orch", name),
		InstanceType:     in.OrchInstanceType,
		SubnetID:         subnetID,
		SecurityGroupIDs: []string{orchSGID},
		RootVolumeSizeGB: rootGiB,
		// Always EIP-backed when public (an EIP is allocated above), so the
		// instance never needs an auto-assigned public IP.
		AssignPublicIP:     false,
		UserData:           script,
		IAMInstanceProfile: cpProfile,
		Tags:               map[string]string{"sbxadm/cluster": name, "sbxadm/role": "orch"},
	})
	if err != nil {
		return fmt.Errorf("launch control plane instance: %w", err)
	}

	rollback.add(fmt.Sprintf("terminate instance %s", instanceID), func(ctx context.Context) error {
		return awsx.TerminateInstance(ctx, ec2c, instanceID)
	})
	s.Done("instance %s running (private_ip=%s public_ip=%s)", instanceID, privateIP, publicIP)

	if eipAllocID != "" {
		s.Step("associating Elastic IP %s", eipAllocID)
		publicIP, err = awsx.AssociateEIPByAllocationID(ctx, ec2c, instanceID, eipAllocID)
		if err != nil {
			return err
		}

		if !eipManaged {
			// Managed EIPs are already covered by the release rollback
			// added at allocation time; user-supplied EIPs should only
			// ever be disassociated, never released, on rollback.
			rollback.add(fmt.Sprintf("disassociate EIP %s", eipAllocID), func(ctx context.Context) error {
				return awsx.DisassociateEIP(ctx, ec2c, eipAllocID)
			})
		}

		s.Done("eip associated public_ip=%s", publicIP)
	}

	s.Step("waiting for SSM agent to come online on %s", instanceID)
	if err := awsx.WaitForSSMOnline(ctx, ssmc, instanceID, ssmOnlineTimeout); err != nil {
		return fmt.Errorf("sbxorch instance never reached SSM: %w", err)
	}
	s.Done("SSM agent online")

	s.Step("waiting for sbxorch.service health check (up to %s)", orchHealthTimeout)
	if err := awsx.WaitForLocalHealthz(ctx, ssmc, instanceID, orchAPIPort, orchHealthTimeout); err != nil {
		return fmt.Errorf("sbxorch health check failed: %w (tail /var/log/sbxadm-userdata.log on the instance via SSM for details)", err)
	}
	s.Done("sbxorch is healthy")

	now := time.Now().UTC()
	cluster := store.Cluster{
		Name:                        name,
		Version:                     in.Version,
		Region:                      in.Region,
		VPCID:                       in.VPCID,
		PublicSubnets:               in.PublicSubnetIDs,
		PrivateSubnet:               in.PrivateSubnetIDs,
		SharedSecret:                sharedSecret,
		SecurityGroup:               orchSGID,
		WorkerSecurityGroup:         workerSGID,
		ControlPlaneInstanceProfile: cpProfile,
		WorkerInstanceProfile:       workerProfile,
		CreatedAt:                   now,
		UpdatedAt:                   now,
		ControlPlane: store.ControlPlane{
			InstanceID:       instanceID,
			InstanceType:     in.OrchInstanceType,
			SubnetID:         subnetID,
			PublicEndpoint:   in.OrchPublicEndpoint,
			PublicEIPAllocID: eipAllocID,
			PublicEIPManaged: eipManaged,
			PublicIP:         publicIP,
			PrivateIP:        privateIP,
			RootVolumeSizeGB: rootGiB,
			ConfigJSON:       configJSON,
		},
	}

	s.Step("persisting cluster state to DynamoDB")
	if err := st.PutNewCluster(ctx, cluster); err != nil {
		return fmt.Errorf("persist cluster: %w", err)
	}

	s.Done("cluster %q persisted", name)
	if cluster.ControlPlane.PublicEndpoint && strings.TrimSpace(cluster.ControlPlane.PublicIP) != "" {
		s.Info("control plane public endpoint: http://%s:%d", cluster.ControlPlane.PublicIP, orchAPIPort)
	}

	s.Info("cluster shared secret: %s", maskSecret(sharedSecret))

	rollback.clear()
	return nil
}

func DeleteCluster(ctx context.Context, ec2c *ec2.Client, iamc *iam.Client, st *store.Store, name string, s *stepper.Stepper) error {
	s.Step("loading cluster %q", name)
	cluster, err := st.GetCluster(ctx, name)
	if err != nil {
		return err
	}
	s.Done("found cluster with %d worker(s)", len(cluster.Workers))

	for _, w := range cluster.Workers {
		s.Step("deleting worker %s (instance=%s)", w.Name, w.InstanceID)
		if err := teardownWorker(ctx, ec2c, w, s); err != nil {
			return fmt.Errorf("delete worker %q: %w", w.Name, err)
		}
		s.Done("worker %s removed", w.Name)
	}

	s.Step("terminating control plane instance %s", cluster.ControlPlane.InstanceID)
	if cluster.ControlPlane.PublicEIPAllocID != "" {
		if cluster.ControlPlane.PublicEIPManaged {
			if err := awsx.ReleaseEIP(ctx, ec2c, cluster.ControlPlane.PublicEIPAllocID); err != nil {
				s.Warn("release control plane EIP: %v", err)
			}
		} else if err := awsx.DisassociateEIP(ctx, ec2c, cluster.ControlPlane.PublicEIPAllocID); err != nil {
			s.Warn("disassociate control plane EIP: %v", err)
		}
	}
	if err := awsx.TerminateInstance(ctx, ec2c, cluster.ControlPlane.InstanceID); err != nil {
		return fmt.Errorf("terminate control plane instance: %w", err)
	}
	s.Done("control plane instance terminated")

	s.Step("deleting security groups")
	// Worker SG has an ingress rule referencing the orch SG as its source,
	// so it must go first or the orch SG delete fails with
	// DependencyViolation.
	if err := awsx.DeleteSecurityGroup(ctx, ec2c, cluster.WorkerSecurityGroup); err != nil {
		s.Warn("delete worker security group: %v", err)
	}

	if err := awsx.DeleteSecurityGroup(ctx, ec2c, cluster.SecurityGroup); err != nil {
		s.Warn("delete orch security group: %v", err)
	}
	s.Done("security groups deleted")

	s.Step("deleting IAM instance profiles")
	if err := awsx.DeleteInstanceProfile(ctx, iamc, fmt.Sprintf("sbxcluster-%s-control-plane", name), nil); err != nil {
		s.Warn("delete control plane instance profile: %v", err)
	}

	if err := awsx.DeleteInstanceProfile(ctx, iamc, fmt.Sprintf("sbxcluster-%s-worker", name), nil); err != nil {
		s.Warn("delete worker instance profile: %v", err)
	}
	s.Done("IAM instance profiles deleted")

	s.Step("removing cluster record from DynamoDB")
	if err := st.DeleteCluster(ctx, name); err != nil {
		return fmt.Errorf("delete cluster record: %w", err)
	}
	s.Done("cluster %q deleted", name)

	return nil
}

func pickSubnet(ids []string) string {
	if len(ids) == 0 {
		return ""
	}
	return ids[0]
}

func randomSecret(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func resolveClusterSharedSecret(raw string) (secret string, userProvided bool, err error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		secret, err := randomSecret(32)
		return secret, false, err
	}

	if len(trimmed) < 8 {
		return "", false, fmt.Errorf("explicit shared secret must be at least 8 characters")
	}

	return trimmed, true, nil
}

func maskSecret(secret string) string {
	if secret == "" {
		return ""
	}

	runes := []rune(secret)
	if len(runes) == 1 {
		return string(runes[0])
	}

	if len(runes) == 2 {
		return string(runes[0]) + "*"
	}

	return string(runes[0]) + strings.Repeat("*", len(runes)-2) + string(runes[len(runes)-1])
}
