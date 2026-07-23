package cmd

import (
	"context"
	"fmt"
	"strings"
	"time"

	"sandboxd-o/sandboxd-adm/awsx"
	"sandboxd-o/sandboxd-adm/color"
	cfgfile "sandboxd-o/sandboxd-adm/config"
	"sandboxd-o/sandboxd-adm/store"

	"github.com/spf13/cobra"
)

type Options struct {
	EnvFile    string
	Region     string
	Profile    string
	StoreTable string
	OrchServer string
	Timeout    time.Duration
	NoColor    bool
}

func NewRoot() *cobra.Command {
	opts := &Options{}

	root := &cobra.Command{
		Use:           "sbxadm",
		Short:         "Sandboxd-O Admin: provisions sbxorch/sbxlet clusters on AWS",
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			color.Init(opts.NoColor)

			cfg, err := cfgfile.Load(opts.EnvFile)
			if err != nil {
				return err
			}

			if strings.TrimSpace(opts.StoreTable) == "" {
				opts.StoreTable = cfg.StoreDynamoDBTable
			}
			if strings.TrimSpace(opts.OrchServer) == "" {
				opts.OrchServer = cfg.OrchServer
			}
			if strings.TrimSpace(opts.Profile) == "" {
				opts.Profile = cfg.AWSProfile
			}
			if strings.TrimSpace(opts.Region) == "" {
				opts.Region = cfg.AWSRegion
			}

			return nil
		},
	}

	root.PersistentFlags().StringVar(&opts.EnvFile, "env-file", "", "path to a KEY=VALUE env file (e.g. SBXADM_STORE_DYNAMODB, SBXADM_ORCH_SERVER)")
	root.PersistentFlags().StringVar(&opts.Region, "region", "", "AWS region (default: the region configured for --profile, env: AWS_REGION)")
	root.PersistentFlags().StringVar(&opts.Profile, "profile", "", "AWS profile (env: AWS_PROFILE, default: default)")
	root.PersistentFlags().StringVar(&opts.StoreTable, "store-table", "", "DynamoDB table for sbxadm state (env: SBXADM_STORE_DYNAMODB)")
	root.PersistentFlags().StringVar(&opts.OrchServer, "orch-server", "", "override orchestrator base URL (env: SBXADM_ORCH_SERVER; default: derived from the cluster's control plane IP)")
	root.PersistentFlags().DurationVar(&opts.Timeout, "timeout", 20*time.Second, "orchestrator API request timeout")
	root.PersistentFlags().BoolVar(&opts.NoColor, "no-color", false, "disable colored output (also honors NO_COLOR env var)")

	root.AddCommand(newCreateCommand(opts))
	root.AddCommand(newInfoCommand(opts))
	root.AddCommand(newResizeCommand(opts))
	root.AddCommand(newDeleteCommand(opts))
	root.AddCommand(newUpdateSbxctlConfigCommand(opts))

	return root
}

func (o *Options) resolveRegion(ctx context.Context) error {
	if strings.TrimSpace(o.Region) != "" {
		return nil
	}

	region, err := awsx.ResolveDefaultRegion(ctx, o.Profile)
	if err != nil {
		return fmt.Errorf("--region not given and no default could be resolved: %w", err)
	}
	o.Region = region
	fmt.Println(color.Gray(fmt.Sprintf("(no --region given; using %q from AWS profile %q)", region, orDefaultProfileName(o.Profile))))
	return nil
}

func orDefaultProfileName(p string) string {
	if strings.TrimSpace(p) == "" {
		return "default"
	}
	return p
}

func (o *Options) clients(ctx context.Context) (*awsx.Clients, *store.Store, error) {
	if err := o.resolveRegion(ctx); err != nil {
		return nil, nil, err
	}

	c, err := awsx.NewClients(ctx, o.Profile, o.Region)
	if err != nil {
		return nil, nil, err
	}

	st := store.New(c.DynamoDB, o.StoreTable)
	created, err := st.EnsureTable(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("ensure state table %q: %w", o.StoreTable, err)
	}
	if created {
		fmt.Println(color.Yellow(fmt.Sprintf("! DynamoDB table %q did not exist and was created automatically (pay-per-request billing)", o.StoreTable)))
	}

	return c, st, nil
}
