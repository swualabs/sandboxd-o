package cmd

import (
	"fmt"
	"strings"

	"sandboxd-o/sandboxd-adm/color"
	"sandboxd-o/sandboxd-adm/store"

	"github.com/spf13/cobra"
)

func newInfoCommand(opts *Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "info",
		Short: "Show detailed cluster/worker information",
	}

	cmd.AddCommand(newInfoClusterCommand(opts))
	cmd.AddCommand(newInfoWorkerCommand(opts))

	return cmd
}

func newInfoWorkerCommand(opts *Options) *cobra.Command {
	var clusterName string

	cmd := &cobra.Command{
		Use:   "worker <name>",
		Short: "Show detailed information about a single worker node",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireFlags(requiredFlag{"--cluster", clusterName}); err != nil {
				return err
			}

			ctx := cmd.Context()
			_, st, err := opts.clients(ctx)
			if err != nil {
				return err
			}

			cluster, err := st.GetCluster(ctx, clusterName)
			if err != nil {
				return err
			}

			for _, w := range cluster.Workers {
				if w.Name == args[0] {
					printWorkerInfo(cmd, w)
					return nil
				}
			}

			return fmt.Errorf("worker %q not found in cluster %q", args[0], clusterName)
		},
	}

	cmd.Flags().StringVar(&clusterName, "cluster", "", "cluster name (required)")

	return cmd
}

func printWorkerInfo(cmd *cobra.Command, w store.Worker) {
	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "Worker Node Name:               %s\n", w.Name)
	fmt.Fprintf(out, "Worker Node Instance ID:        %s\n", w.InstanceID)
	fmt.Fprintf(out, "Worker Node Instance Type:      %s\n", w.InstanceType)
	fmt.Fprintf(out, "Worker Node Subnet:             %s\n", w.SubnetID)
	fmt.Fprintf(out, "Worker Node Public IP:          %s\n", valueOrNone(w.PublicIP))
	fmt.Fprintf(out, "Worker Node Private IP:         %s\n", w.PrivateIP)
	fmt.Fprintf(out, "Worker Node Root Volume Size:   %dGi\n", w.RootVolumeSizeGB)
	fmt.Fprintf(out, "Worker Node Security Group:     %s\n", w.SecurityGroup)
	if w.External != "" {
		fmt.Fprintf(out, "Worker Node External:           %s\n", w.External)
	}
	fmt.Fprintf(out, "Worker Node Created At:         %s\n", w.CreatedAt.Format("2006-01-02T15:04:05Z07:00"))
	fmt.Fprintf(out, "Worker Node Config (JSON):\n%s\n", indent(w.ConfigJSON))
}

func newInfoClusterCommand(opts *Options) *cobra.Command {
	return &cobra.Command{
		Use:   "cluster <name>",
		Short: "Show detailed information about a cluster and its workers",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			_, st, err := opts.clients(ctx)
			if err != nil {
				return err
			}

			cluster, err := st.GetCluster(ctx, args[0])
			if err != nil {
				return err
			}

			printClusterInfo(cmd, cluster)
			return nil
		},
	}
}

func printClusterInfo(cmd *cobra.Command, c *store.Cluster) {
	out := cmd.OutOrStdout()

	fmt.Fprintln(out, color.Bold(fmt.Sprintf("Cluster: %s", c.Name)))
	fmt.Fprintf(out, "Cluster Name:                  %s\n", c.Name)
	fmt.Fprintf(out, "Cluster Version:                %s\n", c.Version)
	fmt.Fprintf(out, "Cluster Region:                 %s\n", c.Region)
	fmt.Fprintf(out, "Cluster VPC ID:                 %s\n", c.VPCID)
	fmt.Fprintf(out, "Cluster Public Subnet:          %v\n", c.PublicSubnets)
	fmt.Fprintf(out, "Cluster Private Subnet:         %v\n", c.PrivateSubnet)
	fmt.Fprintf(out, "Cluster Created At:             %s\n", c.CreatedAt.Format("2006-01-02T15:04:05Z07:00"))
	fmt.Fprintf(out, "Cluster Updated At:             %s\n", c.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"))
	fmt.Fprintln(out)

	cp := c.ControlPlane
	fmt.Fprintln(out, color.Bold("Cluster Control Plane"))
	fmt.Fprintf(out, "Cluster Control Plane / Instance ID:          %s\n", cp.InstanceID)
	fmt.Fprintf(out, "Cluster Control Plane / Instance Type:        %s\n", cp.InstanceType)
	fmt.Fprintf(out, "Cluster Control Plane / Subnet:                %s\n", cp.SubnetID)
	fmt.Fprintf(out, "Cluster Control Plane / Public Access:         %s\n", boolColor(cp.PublicEndpoint))
	if cp.PublicEIPAllocID != "" {
		fmt.Fprintf(out, "Cluster Control Plane / Public EIP Alloc ID:   %s%s\n", cp.PublicEIPAllocID, managedSuffix(cp.PublicEIPManaged))
	}
	fmt.Fprintf(out, "Cluster Control Plane / Public IP:             %s\n", valueOrNone(cp.PublicIP))
	fmt.Fprintf(out, "Cluster Control Plane / Private IP:            %s\n", cp.PrivateIP)
	fmt.Fprintf(out, "Cluster Control Plane / Root Volume Size:      %dGi\n", cp.RootVolumeSizeGB)
	fmt.Fprintf(out, "Cluster Control Plane / Security Group:        %s\n", c.SecurityGroup)
	fmt.Fprintf(out, "Cluster Control Plane / Instance Profile:      %s\n", c.ControlPlaneInstanceProfile)
	fmt.Fprintf(out, "Cluster Control Plane / Config (JSON):\n%s\n", indent(cp.ConfigJSON))
	fmt.Fprintln(out)

	if len(c.WorkerECRRepoPatterns) > 0 {
		fmt.Fprintf(out, "Cluster Worker ECR Pull Allowlist:      %v\n", c.WorkerECRRepoPatterns)
		fmt.Fprintln(out)
	}

	if len(c.Workers) == 0 {
		fmt.Fprintln(out, "Cluster Worker Nodes: (none)")
		return
	}

	for _, w := range c.Workers {
		fmt.Fprintln(out, color.Bold(fmt.Sprintf("Cluster Worker Node - %s", w.Name)))
		fmt.Fprintf(out, "Cluster Worker Node - %s / Instance ID:          %s\n", w.Name, w.InstanceID)
		fmt.Fprintf(out, "Cluster Worker Node - %s / Instance Type:        %s\n", w.Name, w.InstanceType)
		fmt.Fprintf(out, "Cluster Worker Node - %s / Subnet:               %s\n", w.Name, w.SubnetID)
		fmt.Fprintf(out, "Cluster Worker Node - %s / Public IP:            %s\n", w.Name, valueOrNone(w.PublicIP))
		if w.PublicEIPAllocID != "" {
			fmt.Fprintf(out, "Cluster Worker Node - %s / Public EIP Alloc ID:  %s%s\n", w.Name, w.PublicEIPAllocID, managedSuffix(w.PublicEIPManaged))
		}
		fmt.Fprintf(out, "Cluster Worker Node - %s / Private IP:           %s\n", w.Name, w.PrivateIP)
		fmt.Fprintf(out, "Cluster Worker Node - %s / Root Volume Size:     %dGi\n", w.Name, w.RootVolumeSizeGB)
		fmt.Fprintf(out, "Cluster Worker Node - %s / Security Group:       %s\n", w.Name, w.SecurityGroup)
		if w.External != "" {
			fmt.Fprintf(out, "Cluster Worker Node - %s / External:              %s\n", w.Name, w.External)
		}
		fmt.Fprintf(out, "Cluster Worker Node - %s / Config (JSON):\n%s\n", w.Name, indent(w.ConfigJSON))
		fmt.Fprintln(out)
	}
}

func boolColor(b bool) string {
	if b {
		return color.Green("true")
	}
	return color.Gray("false")
}

func managedSuffix(managed bool) string {
	if managed {
		return color.Gray(" (managed by sbxadm)")
	}
	return color.Gray(" (user-supplied)")
}

func valueOrNone(v string) string {
	if v == "" {
		return "(none)"
	}
	return v
}

func indent(jsonStr string) string {
	if jsonStr == "" {
		return "  (default)"
	}
	var out strings.Builder
	out.WriteString("  ")
	for _, r := range jsonStr {
		out.WriteString(string(r))
		if r == '\n' {
			out.WriteString("  ")
		}
	}
	return out.String()
}
