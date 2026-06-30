package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

func newCordonCommand(opts *Options) *cobra.Command {
	return newNodeSchedulingCommand(opts, "cordon", true)
}

func newUncordonCommand(opts *Options) *cobra.Command {
	return newNodeSchedulingCommand(opts, "uncordon", false)
}

func newNodeSchedulingCommand(opts *Options, use string, unschedulable bool) *cobra.Command {
	cmd := &cobra.Command{
		Use:   use + " <node/name>",
		Short: map[bool]string{true: "Mark node unschedulable", false: "Mark node schedulable"}[unschedulable],
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(opts.Node) != "" {
				return fmt.Errorf("--node is not applicable for node object update")
			}

			ref, err := parseObjectRef(args[0])
			if err != nil {
				return err
			}
			if ref.Resource != "node" {
				return fmt.Errorf("%s supports only node resources", use)
			}

			c := mustClient(opts)
			ctx, cancel := withCtx(opts)
			defer cancel()

			out, err := c.PatchNode(ctx, ref.Name, map[string]any{
				"spec": map[string]any{
					"unschedulable": unschedulable,
				},
			})
			if err != nil {
				return err
			}

			return printAny(cmd.OutOrStdout(), out, opts.Output)
		},
	}

	return cmd
}
