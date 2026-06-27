package cmd

import (
	"sandboxd-o/sandboxd-adm/orchestrate"
	"sandboxd-o/sandboxd-adm/stepper"

	"github.com/spf13/cobra"
)

func newDeleteCommand(opts *Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete a worker node or an entire cluster",
	}

	cmd.AddCommand(newDeleteWorkerCommand(opts))
	cmd.AddCommand(newDeleteClusterCommand(opts))

	return cmd
}

func newDeleteWorkerCommand(opts *Options) *cobra.Command {
	var clusterName string

	cmd := &cobra.Command{
		Use:   "worker <name>",
		Short: "Delete a single worker node from a cluster",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireFlags(map[string]string{"--cluster": clusterName}); err != nil {
				return err
			}

			ctx := cmd.Context()
			c, st, err := opts.clients(ctx)
			if err != nil {
				return err
			}

			s := stepper.New()
			s.Step("deleting worker %q from cluster %q", args[0], clusterName)
			if err := orchestrate.DeleteWorker(ctx, c.EC2, st, clusterName, args[0], opts.OrchServer, opts.Timeout, s); err != nil {
				s.Fail("delete worker %q: %v", args[0], err)
				return err
			}

			s.Done("worker %q deleted", args[0])
			return nil
		},
	}

	cmd.Flags().StringVar(&clusterName, "cluster", "", "cluster name (required)")

	return cmd
}

func newDeleteClusterCommand(opts *Options) *cobra.Command {
	return &cobra.Command{
		Use:   "cluster <name>",
		Short: "Delete a cluster, its control plane, and all of its worker nodes",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			c, st, err := opts.clients(ctx)
			if err != nil {
				return err
			}

			s := stepper.New()
			s.Step("deleting cluster %q", args[0])
			if err := orchestrate.DeleteCluster(ctx, c.EC2, c.IAM, st, args[0], s); err != nil {
				s.Fail("delete cluster %q: %v", args[0], err)
				return err
			}

			s.Done("cluster %q deleted", args[0])
			return nil
		},
	}
}
