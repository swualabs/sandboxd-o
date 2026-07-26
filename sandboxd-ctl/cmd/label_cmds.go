package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

func newLabelCommand(opts *Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "label <node/name> <key=value|key->",
		Short: "Add, update, or remove a node label",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(opts.Node) != "" {
				return fmt.Errorf("--node is not applicable for node object update")
			}

			ref, err := parseObjectRef(args[0])
			if err != nil {
				return err
			}
			if ref.Resource != "node" {
				return fmt.Errorf("label supports only node resources")
			}

			key, value, remove, err := parseLabelMutation(args[1])
			if err != nil {
				return err
			}

			labels := map[string]any{}
			if remove {
				labels[key] = nil
			} else {
				labels[key] = value
			}

			c := mustClient(opts)
			ctx, cancel := withCtx(opts)
			defer cancel()

			out, err := c.PatchNode(ctx, ref.Name, map[string]any{
				"metadata": map[string]any{
					"labels": labels,
				},
				"spec": map[string]any{},
			})
			if err != nil {
				return err
			}

			return printAny(cmd.OutOrStdout(), out, opts.Output)
		},
	}

	return cmd
}

func parseLabelMutation(arg string) (key, value string, remove bool, err error) {
	arg = strings.TrimSpace(arg)
	if arg == "" {
		return "", "", false, fmt.Errorf("label mutation is required")
	}

	if strings.Contains(arg, "=") {
		parts := strings.SplitN(arg, "=", 2)
		key = strings.TrimSpace(parts[0])
		if key == "" {
			return "", "", false, fmt.Errorf("label key is required")
		}

		return key, parts[1], false, nil
	}

	if strings.HasSuffix(arg, "-") {
		key = strings.TrimSpace(strings.TrimSuffix(arg, "-"))
		if key == "" {
			return "", "", false, fmt.Errorf("label key is required")
		}

		return key, "", true, nil
	}

	return "", "", false, fmt.Errorf("expected label mutation in the form key=value or key-")
}
