package cmd

import (
	"fmt"
	"os"
	"strings"

	"sandboxd-o/sandboxd-ctl/manifest"

	"github.com/spf13/cobra"
)

func newGetCommand(opts *Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "get <resource[,resource...]|resource/name>",
		Aliases: []string{"g"},
		Short:   "Get resources",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c := mustClient(opts)
			ctx, cancel := withCtx(opts)
			defer cancel()

			arg := strings.TrimSpace(args[0])
			if strings.Contains(arg, "/") {
				ref, err := parseObjectRef(arg)
				if err != nil {
					return err
				}

				var out map[string]any
				var outErr error
				switch ref.Resource {
				case "sandbox":
					if strings.TrimSpace(opts.Node) != "" {
						out, outErr = c.NodeGetSandbox(ctx, opts.Node, ref.Name)
					} else {
						out, outErr = c.GetSandbox(ctx, ref.Name)
					}
				case "node":
					if strings.TrimSpace(opts.Node) != "" {
						return fmt.Errorf("--node is not applicable when getting node objects")
					}
					out, outErr = c.GetNode(ctx, ref.Name)
				default:
					return fmt.Errorf("unsupported resource %q", ref.Resource)
				}

				if outErr != nil {
					return outErr
				}

				return printAny(cmd.OutOrStdout(), out, opts.Output)
			}

			resources, err := parseResourceList(arg)
			if err != nil {
				return err
			}

			outMode := strings.ToLower(strings.TrimSpace(opts.Output))
			combined := map[string]any{}

			for i, res := range resources {
				var out map[string]any
				switch res {
				case "sandbox":
					if strings.TrimSpace(opts.Node) != "" {
						out, err = c.NodeListSandboxes(ctx, opts.Node, opts.Limit)
					} else {
						out, err = c.ListSandboxes(ctx)
					}
				case "node":
					if strings.TrimSpace(opts.Node) != "" {
						return fmt.Errorf("--node is not applicable when listing node objects")
					}
					out, err = c.ListNodes(ctx)
				}

				if err != nil {
					return err
				}

				if outMode == "" || outMode == "wide" {
					if i > 0 {
						fmt.Fprintln(cmd.OutOrStdout())
					}

					fmt.Fprintf(cmd.OutOrStdout(), "RESOURCE: %s\n", res)
					if res == "sandbox" {
						rows := extractSandboxRows(out["items"])
						if len(rows) == 0 {
							fmt.Fprintln(cmd.OutOrStdout(), "No resources found")
							continue
						}

						if outMode == "wide" {
							printSandboxTableWide(cmd.OutOrStdout(), rows)
						} else {
							printSandboxTable(cmd.OutOrStdout(), rows)
						}
					} else {
						rows := extractNodeRows(out["items"])
						if len(rows) == 0 {
							fmt.Fprintln(cmd.OutOrStdout(), "No resources found")
							continue
						}

						if outMode == "wide" {
							printNodeTableWide(cmd.OutOrStdout(), rows)
						} else {
							printNodeTable(cmd.OutOrStdout(), rows)
						}
					}

					continue
				}

				switch res {
				case "sandbox":
					combined["sandboxes"] = out["items"]
				case "node":
					combined["nodes"] = out["items"]
				}
			}

			if outMode == "" || outMode == "wide" {
				return nil
			}
			return printAny(cmd.OutOrStdout(), combined, outMode)
		},
	}

	return cmd
}

func newSpecCommand(opts *Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "spec <resource/name>",
		Short: "Print resource in YAML spec form",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ref, err := parseObjectRef(args[0])
			if err != nil {
				return err
			}

			if err := ensureSandboxResource(ref.Resource); err != nil {
				return err
			}

			c := mustClient(opts)
			ctx, cancel := withCtx(opts)
			defer cancel()

			var out map[string]any
			if opts.Node != "" {
				out, err = c.NodeGetSandbox(ctx, opts.Node, ref.Name)
			} else {
				out, err = c.GetSandbox(ctx, ref.Name)
			}
			if err != nil {
				return err
			}

			sbx, _ := out["sandbox"].(map[string]any)
			if sbx == nil {
				return printAny(cmd.OutOrStdout(), out, "yaml")
			}

			spec, _ := sbx["spec"].(map[string]any)
			manifest := map[string]any{
				"apiVersion": "sandboxd.o/v1",
				"kind":       "Sandbox",
				"id":         sbx["id"],
				"spec":       spec,
			}
			return printAny(cmd.OutOrStdout(), manifest, "yaml")
		},
	}

	return cmd
}

func newCreateCommand(opts *Options) *cobra.Command {
	var file string
	cmd := &cobra.Command{
		Use:     "create -f <file>",
		Aliases: []string{"c"},
		Short:   "Create resource from YAML file",
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(file) == "" {
				return fmt.Errorf("-f/--file is required")
			}

			raw, err := os.ReadFile(file)
			if err != nil {
				return err
			}

			payload, err := manifest.ParseSandboxManifest(raw)
			if err != nil {
				return err
			}

			c := mustClient(opts)
			ctx, cancel := withCtx(opts)
			defer cancel()

			var out map[string]any
			if strings.TrimSpace(opts.Node) != "" {
				out, err = c.NodeCreateSandbox(ctx, opts.Node, payload)
			} else {
				out, err = c.CreateSandbox(ctx, payload)
			}

			if err != nil {
				return err
			}

			return printAny(cmd.OutOrStdout(), out, opts.Output)
		},
	}

	cmd.Flags().StringVarP(&file, "file", "f", "", "YAML file path")
	return cmd
}

func newDeleteCommand(opts *Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "delete <resource/name>",
		Aliases: []string{"d"},
		Short:   "Delete resource",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ref, err := parseObjectRef(args[0])
			if err != nil {
				return err
			}

			if err := ensureSandboxResource(ref.Resource); err != nil {
				return err
			}

			c := mustClient(opts)
			ctx, cancel := withCtx(opts)
			defer cancel()

			var out map[string]any
			if strings.TrimSpace(opts.Node) != "" {
				out, err = c.NodeDeleteSandbox(ctx, opts.Node, ref.Name)
			} else {
				out, err = c.DeleteSandbox(ctx, ref.Name)
			}

			if err != nil {
				return err
			}

			return printAny(cmd.OutOrStdout(), out, opts.Output)
		},
	}

	return cmd
}

func newLogsCommand(opts *Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "logs <resource/name> <container>",
		Aliases: []string{"log", "l"},
		Short:   "Get container logs via orchestrator node proxy",
		Args:    cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			ref, err := parseObjectRef(args[0])
			if err != nil {
				return err
			}

			if err := ensureSandboxResource(ref.Resource); err != nil {
				return err
			}

			container := strings.TrimSpace(args[1])
			if container == "" {
				return fmt.Errorf("container name is required")
			}

			c := mustClient(opts)
			ctx, cancel := withCtx(opts)
			defer cancel()

			node := strings.TrimSpace(opts.Node)
			if node == "" {
				obj, err := c.GetSandbox(ctx, ref.Name)
				if err != nil {
					return fmt.Errorf("resolve node from sandbox status: %w", err)
				}

				sbx, _ := obj["sandbox"].(map[string]any)
				status, _ := sbx["status"].(map[string]any)
				node = strings.TrimSpace(toString(status["node_name"]))
				if node == "" {
					return fmt.Errorf("sandbox %s has no assigned node; provide --node", ref.Name)
				}
			}

			out, err := c.NodeContainerLogs(ctx, node, ref.Name, container, opts.Limit)
			if err != nil {
				return err
			}

			return printAny(cmd.OutOrStdout(), out, opts.Output)
		},
	}
	return cmd
}
