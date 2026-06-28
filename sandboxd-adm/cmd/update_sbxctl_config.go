package cmd

import (
	"sandboxd-o/sandboxd-adm/orchestrate"
	"sandboxd-o/sandboxd-adm/stepper"

	"github.com/spf13/cobra"
)

func newUpdateSbxctlConfigCommand(opts *Options) *cobra.Command {
	return &cobra.Command{
		Use:   "update-sbxctl-config <cluster>",
		Short: "Refresh /var/lib/sandboxd/sbxctl_config.json on a cluster's public control plane",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			c, st, err := opts.clients(ctx)
			if err != nil {
				return err
			}

			s := stepper.New()
			s.Step("updating sbxctl_config.json for cluster %q", args[0])
			if err := orchestrate.UpdateSbxctlConfig(ctx, c.SSM, st, args[0], s); err != nil {
				s.Fail("update-sbxctl-config %q: %v", args[0], err)
				return err
			}

			s.Done("sbxctl_config.json updated for cluster %q", args[0])
			return nil
		},
	}
}
