package awsx

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

type SubnetInfo struct {
	ID                  string
	AZ                  string
	MapPublicIPOnLaunch bool
}

func ValidateVPC(ctx context.Context, c *ec2.Client, vpcID string) error {
	_, err := ValidateVPCAndGetCIDR(ctx, c, vpcID)
	return err
}

func ValidateVPCAndGetCIDR(ctx context.Context, c *ec2.Client, vpcID string) (string, error) {
	out, err := c.DescribeVpcs(ctx, &ec2.DescribeVpcsInput{VpcIds: []string{vpcID}})
	if err != nil {
		return "", fmt.Errorf("describe vpc %q: %w", vpcID, err)
	}

	if len(out.Vpcs) == 0 {
		return "", fmt.Errorf("vpc %q not found in this account/region", vpcID)
	}

	vpc := out.Vpcs[0]
	if vpc.State != ec2types.VpcStateAvailable {
		return "", fmt.Errorf("vpc %q is not available (state=%s)", vpcID, vpc.State)
	}

	if vpc.CidrBlock == nil || *vpc.CidrBlock == "" {
		return "", fmt.Errorf("vpc %q has no primary CIDR block", vpcID)
	}

	return *vpc.CidrBlock, nil
}

func ValidateSubnets(ctx context.Context, c *ec2.Client, vpcID string, subnetIDs []string) ([]SubnetInfo, error) {
	if len(subnetIDs) == 0 {
		return nil, fmt.Errorf("at least one subnet id is required")
	}

	out, err := c.DescribeSubnets(ctx, &ec2.DescribeSubnetsInput{SubnetIds: subnetIDs})
	if err != nil {
		return nil, fmt.Errorf("describe subnets %v: %w", subnetIDs, err)
	}

	byID := make(map[string]ec2types.Subnet, len(out.Subnets))
	for _, s := range out.Subnets {
		byID[*s.SubnetId] = s
	}

	infos := make([]SubnetInfo, 0, len(subnetIDs))
	for _, id := range subnetIDs {
		s, ok := byID[id]
		if !ok {
			return nil, fmt.Errorf("subnet %q not found in this account/region", id)
		}

		if s.VpcId == nil || *s.VpcId != vpcID {
			return nil, fmt.Errorf("subnet %q does not belong to vpc %q", id, vpcID)
		}

		if s.State != ec2types.SubnetStateAvailable {
			return nil, fmt.Errorf("subnet %q is not available (state=%s)", id, s.State)
		}

		az := ""
		if s.AvailabilityZone != nil {
			az = *s.AvailabilityZone
		}
		infos = append(infos, SubnetInfo{
			ID:                  id,
			AZ:                  az,
			MapPublicIPOnLaunch: s.MapPublicIpOnLaunch != nil && *s.MapPublicIpOnLaunch,
		})
	}

	return infos, nil
}
