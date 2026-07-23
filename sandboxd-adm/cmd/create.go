package cmd

import (
	"fmt"
	"strings"

	"sandboxd-o/sandboxd-adm/orchestrate"
	"sandboxd-o/sandboxd-adm/stepper"

	"github.com/spf13/cobra"
)

func newCreateCommand(opts *Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a cluster or worker node",
	}

	cmd.AddCommand(newCreateClusterCommand(opts))
	cmd.AddCommand(newCreateWorkerCommand(opts))

	return cmd
}

func newCreateClusterCommand(opts *Options) *cobra.Command {
	in := orchestrate.CreateClusterInput{}
	var publicSubnets, privateSubnets string

	cmd := &cobra.Command{
		Use:   "cluster <name>",
		Short: "Create a cluster and its single control plane (sbxorch)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			in.Name = args[0]
			in.PublicSubnetIDs = splitCSV(publicSubnets)
			in.PrivateSubnetIDs = splitCSV(privateSubnets)

			if err := requireFlags(
				requiredFlag{"--version", in.Version},
				requiredFlag{"--vpc-id", in.VPCID},
				requiredFlag{"--public-subnet", publicSubnets},
				requiredFlag{"--private-subnet", privateSubnets},
				requiredFlag{"--orch-instance", in.OrchInstanceType},
			); err != nil {
				return err
			}

			ctx := cmd.Context()
			c, st, err := opts.clients(ctx)
			if err != nil {
				return err
			}
			in.Region = opts.Region

			s := stepper.New()
			s.Step("creating cluster %q (version=%s region=%s)", in.Name, in.Version, in.Region)
			if err := orchestrate.CreateCluster(ctx, c.EC2, c.IAM, c.SSM, st, in, s); err != nil {
				s.Fail("create cluster %q: %v", in.Name, err)
				return err
			}

			s.Done("cluster %q created", in.Name)
			return nil
		},
	}

	cmd.Flags().StringVar(&in.Version, "version", "", "sandboxd-o release version to install, e.g. 0.3.0 (required)")
	cmd.Flags().StringVar(&in.VPCID, "vpc-id", "", "VPC id (required)")
	cmd.Flags().StringVar(&publicSubnets, "public-subnet", "", "comma-separated public subnet ids (at least 1, required)")
	cmd.Flags().StringVar(&privateSubnets, "private-subnet", "", "comma-separated private subnet ids (at least 1, required)")
	cmd.Flags().StringVar(&in.OrchInstanceType, "orch-instance", "", "control plane EC2 instance type (required)")
	cmd.Flags().BoolVar(&in.OrchPublicEndpoint, "orch-public-endpoint", false, "place the control plane in a public subnet with a public IP")
	cmd.Flags().StringVar(&in.OrchPublicEIP, "orch-public-eip", "", "existing Elastic IP ARN or allocation id to associate (requires --orch-public-endpoint); without it, sbxadm allocates and manages its own EIP automatically")
	cmd.Flags().StringVar(&in.OrchRootVolume, "orch-root-volume-size", "16Gi", "control plane root EBS volume size")
	cmd.Flags().StringVar(&in.OrchConfigPath, "orch-config", "", "JSON file overriding sbxorch_config.json defaults (only changed keys need to be present)")
	cmd.Flags().StringVar(&in.SharedSecret, "shared-secret", "", "explicit cluster shared secret (min 8 chars); omitted by default so sbxadm generates a random secret")

	return cmd
}

func newCreateWorkerCommand(opts *Options) *cobra.Command {
	in := orchestrate.CreateWorkerInput{RuntimeBinary: "runsc"}

	cmd := &cobra.Command{
		Use:   "worker <name>",
		Short: "Create a worker node (sbxlet) in an existing cluster",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			in.Name = args[0]
			in.OrchServer = opts.OrchServer
			in.OrchTimeout = opts.Timeout

			if err := requireFlags(
				requiredFlag{"--version", in.Version},
				requiredFlag{"--cluster", in.ClusterName},
				requiredFlag{"--instance", in.InstanceType},
			); err != nil {
				return err
			}

			ctx := cmd.Context()
			c, st, err := opts.clients(ctx)
			if err != nil {
				return err
			}

			accountID, err := c.AccountID(ctx)
			if err != nil {
				return err
			}

			s := stepper.New()
			s.Step("creating worker %q in cluster %q", in.Name, in.ClusterName)
			if err := orchestrate.CreateWorker(ctx, c.EC2, c.IAM, c.SSM, st, accountID, in, s); err != nil {
				s.Fail("create worker %q: %v", in.Name, err)
				return err
			}

			s.Done("worker %q created", in.Name)
			return nil
		},
	}

	cmd.Flags().StringVar(&in.Version, "version", "", "sandboxd-o release version to install, e.g. 0.3.0 (required)")
	cmd.Flags().StringVar(&in.ClusterName, "cluster", "", "cluster name (required)")
	cmd.Flags().StringVar(&in.InstanceType, "instance", "", "worker EC2 instance type (required)")
	cmd.Flags().StringVar(&in.RuntimeBinary, "runtime-binary", "runsc", "worker container runtime handler to use: runsc (default) or runc")
	cmd.Flags().StringVar(&in.RootVolume, "root-volume-size", "64Gi", "worker root EBS volume size")
	cmd.Flags().StringVar(&in.External, "external", "", "external hostname; defaults to the worker's own Elastic IP when left empty")
	cmd.Flags().StringVar(&in.PublicEIP, "public-eip", "", "existing Elastic IP ARN or allocation id; without it, sbxadm allocates and manages its own EIP automatically")
	cmd.Flags().StringVar(&in.ConfigPath, "config", "", "JSON file overriding sbxlet_config.json defaults (only changed keys need to be present)")
	cmd.Flags().StringVar(&in.ECRRepos, "ecr-repos", "", "comma-separated private ECR repository name patterns to grant pull access to, e.g. \"my-repo-1,ctf-*\"; wildcards allowed, applies to every worker in the cluster")

	return cmd
}

func splitCSV(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}

	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}

	return out
}

type requiredFlag struct {
	name  string
	value string
}

func requireFlags(flags ...requiredFlag) error {
	for _, f := range flags {
		if strings.TrimSpace(f.value) == "" {
			return fmt.Errorf("%s is required", f.name)
		}
	}

	return nil
}
