package awsx

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awscfg "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
)

type Clients struct {
	EC2      *ec2.Client
	DynamoDB *dynamodb.Client
	IAM      *iam.Client
	SSM      *ssm.Client
	Region   string
}

func ResolveDefaultRegion(ctx context.Context, profile string) (string, error) {
	opts := []func(*awscfg.LoadOptions) error{}
	if profile != "" {
		opts = append(opts, awscfg.WithSharedConfigProfile(profile))
	}

	cfg, err := awscfg.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return "", fmt.Errorf("load aws config (profile=%q): %w", profile, err)
	}

	region := strings.TrimSpace(cfg.Region)
	if region == "" {
		return "", fmt.Errorf("no region configured for profile %q; pass --region or set AWS_REGION", orDefaultProfile(profile))
	}

	return region, nil
}

func orDefaultProfile(p string) string {
	if p == "" {
		return "default"
	}
	return p
}

func NewClients(ctx context.Context, profile, region string) (*Clients, error) {
	if region == "" {
		return nil, fmt.Errorf("region is required")
	}

	opts := []func(*awscfg.LoadOptions) error{
		awscfg.WithRegion(region),
	}
	if profile != "" {
		opts = append(opts, awscfg.WithSharedConfigProfile(profile))
	}

	cfg, err := awscfg.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("load aws config (profile=%q region=%q): %w", profile, region, err)
	}

	if _, err := cfg.Credentials.Retrieve(ctx); err != nil {
		return nil, fmt.Errorf("resolve aws credentials (profile=%q): %w", profile, err)
	}

	return &Clients{
		EC2:      ec2.NewFromConfig(cfg),
		DynamoDB: dynamodb.NewFromConfig(cfg),
		IAM:      iam.NewFromConfig(cfg),
		SSM:      ssm.NewFromConfig(cfg),
		Region:   region,
	}, nil
}

func strPtr(v string) *string {
	if v == "" {
		return nil
	}
	return aws.String(v)
}
