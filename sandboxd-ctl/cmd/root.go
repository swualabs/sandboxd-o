package cmd

import (
	"context"
	"fmt"
	"strings"
	"time"

	"sandboxd-o/sandboxd-ctl/client"
	cfgfile "sandboxd-o/sandboxd-ctl/config"

	"github.com/spf13/cobra"
)

type Options struct {
	ConfigPath string
	Server     string
	Node       string
	Timeout    time.Duration
	Output     string
	Limit      int
}

func NewRoot() *cobra.Command {
	opts := &Options{}

	cmd := &cobra.Command{
		Use:           "sbxctl",
		Short:         "sandboxd control client",
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := cfgfile.Load(opts.ConfigPath)
			if err != nil {
				return err
			}

			if !cmd.Flags().Changed("server") && strings.TrimSpace(opts.Server) == "" {
				opts.Server = cfg.Server
			}

			if !cmd.Flags().Changed("timeout") && opts.Timeout == 10*time.Second {
				opts.Timeout = cfg.Timeout
			}

			if !cmd.Flags().Changed("output") && strings.TrimSpace(opts.Output) == "" {
				opts.Output = cfg.Output
			}

			if !cmd.Flags().Changed("limit") && opts.Limit == 100 {
				opts.Limit = cfg.Limit
			}

			if strings.TrimSpace(opts.Server) == "" {
				opts.Server = cfgfile.DefaultConfig().Server
			}

			if opts.Timeout <= 0 {
				opts.Timeout = cfgfile.DefaultConfig().Timeout
			}

			if opts.Limit <= 0 {
				opts.Limit = cfgfile.DefaultConfig().Limit
			}

			return nil
		},
	}

	cmd.PersistentFlags().StringVarP(&opts.ConfigPath, "config", "c", cfgfile.DefaultConfigPath, "path to sbxctl config json")
	cmd.PersistentFlags().StringVar(&opts.Server, "server", "", "orchestrator base url (config file or SBXCTL_SERVER)")
	cmd.PersistentFlags().StringVar(&opts.Node, "node", "", "node id for proxy APIs")
	cmd.PersistentFlags().DurationVar(&opts.Timeout, "timeout", 10*time.Second, "request timeout")
	cmd.PersistentFlags().StringVarP(&opts.Output, "output", "o", "", "output format: json|yaml|wide")
	cmd.PersistentFlags().IntVar(&opts.Limit, "limit", 100, "list limit")

	cmd.AddCommand(newGetCommand(opts))
	cmd.AddCommand(newSpecCommand(opts))
	cmd.AddCommand(newCreateCommand(opts))
	cmd.AddCommand(newDeleteCommand(opts))
	cmd.AddCommand(newLogsCommand(opts))

	return cmd
}

func mustClient(opts *Options) *client.Client {
	return client.New(opts.Server, opts.Timeout)
}

func withCtx(opts *Options) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), opts.Timeout)
}

func ensureSandboxResource(name string) error {
	if normalizeResource(name) != "sandbox" {
		return fmt.Errorf("unsupported resource %q (only sandbox is supported now)", name)
	}

	return nil
}
