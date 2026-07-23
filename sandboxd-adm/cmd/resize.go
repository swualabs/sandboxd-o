package cmd

import (
	"sandboxd-o/sandboxd-adm/orchestrate"
	"sandboxd-o/sandboxd-adm/stepper"

	"github.com/spf13/cobra"
)

func newResizeCommand(opts *Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "resize",
		Short: "Resize a cluster control plane or worker EC2 instance",
	}

	cmd.AddCommand(newResizeControlPlaneCommand(opts))
	cmd.AddCommand(newResizeWorkerCommand(opts))

	return cmd
}

func newResizeControlPlaneCommand(opts *Options) *cobra.Command {
	in := orchestrate.ResizeControlPlaneInput{}

	cmd := &cobra.Command{
		Use:   "control-plane <cluster>",
		Short: "Resize a cluster control plane EC2 instance",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			in.ClusterName = args[0]
			if err := requireFlags(requiredFlag{"--instance", in.InstanceType}); err != nil {
				return err
			}

			ctx := cmd.Context()
			c, st, err := opts.clients(ctx)
			if err != nil {
				return err
			}

			s := stepper.New()
			s.Warn("control plane resize will stop and start the EC2 instance; downtime is expected")
			s.Step("resizing control plane for cluster %q", in.ClusterName)
			if err := orchestrate.ResizeControlPlane(ctx, c.EC2, c.SSM, st, in, s); err != nil {
				s.Fail("resize control plane: %v", err)
				return err
			}

			s.Done("control plane resize complete")
			return nil
		},
	}

	cmd.Flags().StringVar(&in.InstanceType, "instance", "", "target EC2 instance type (required)")

	return cmd
}

func newResizeWorkerCommand(opts *Options) *cobra.Command {
	in := orchestrate.ResizeWorkerInput{}

	cmd := &cobra.Command{
		Use:   "worker <name>",
		Short: "Resize a worker EC2 instance",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			in.WorkerName = args[0]
			if err := requireFlags(
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

			s := stepper.New()
			s.Warn("worker resize will stop and start the EC2 instance; sandbox downtime is expected")
			s.Step("resizing worker %q in cluster %q", in.WorkerName, in.ClusterName)
			in.OrchServer = opts.OrchServer
			in.OrchTimeout = opts.Timeout
			if err := orchestrate.ResizeWorker(ctx, c.EC2, c.SSM, st, in, s); err != nil {
				s.Fail("resize worker %q: %v", in.WorkerName, err)
				return err
			}

			s.Done("worker %q resize complete", in.WorkerName)
			return nil
		},
	}

	cmd.Flags().StringVar(&in.ClusterName, "cluster", "", "cluster name (required)")
	cmd.Flags().StringVar(&in.InstanceType, "instance", "", "target EC2 instance type (required)")

	return cmd
}
