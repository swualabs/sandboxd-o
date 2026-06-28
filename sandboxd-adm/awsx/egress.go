package awsx

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

type PrivateEgressCheck struct {
	HasNATGateway      bool
	HasSSMVPCEndpoints bool
}

// Sufficient is true only for HasNATGateway: SSM VPC endpoints alone cover
// the SSM control channel but not the UserData bootstrap's need to reach
// api.github.com/github.com to download the release artifact, so they
// can't substitute for a NAT gateway.
func (c PrivateEgressCheck) Sufficient() bool {
	return c.HasNATGateway
}

func CheckPrivateEgress(ctx context.Context, c *ec2.Client, vpcID, subnetID string) (PrivateEgressCheck, error) {
	var result PrivateEgressCheck

	hasNAT, err := subnetHasNATRoute(ctx, c, vpcID, subnetID)
	if err != nil {
		return result, err
	}
	result.HasNATGateway = hasNAT

	hasSSM, err := vpcHasSSMEndpoints(ctx, c, vpcID)
	if err != nil {
		return result, err
	}
	result.HasSSMVPCEndpoints = hasSSM

	return result, nil
}

// SubnetHasIGWRoute reports whether the subnet's effective route table has a
// 0.0.0.0/0 route to an internet gateway, i.e. it's a real public subnet.
func SubnetHasIGWRoute(ctx context.Context, c *ec2.Client, vpcID, subnetID string) (bool, error) {
	tables, err := effectiveRouteTables(ctx, c, vpcID, subnetID)
	if err != nil {
		return false, err
	}

	for _, rt := range tables {
		for _, r := range rt.Routes {
			if r.GatewayId != nil && strings.HasPrefix(*r.GatewayId, "igw-") && r.State == ec2types.RouteStateActive {
				return true, nil
			}
		}
	}

	return false, nil
}

func subnetHasNATRoute(ctx context.Context, c *ec2.Client, vpcID, subnetID string) (bool, error) {
	tables, err := effectiveRouteTables(ctx, c, vpcID, subnetID)
	if err != nil {
		return false, err
	}

	for _, rt := range tables {
		for _, r := range rt.Routes {
			if r.NatGatewayId != nil && *r.NatGatewayId != "" && r.State == ec2types.RouteStateActive {
				return true, nil
			}
		}
	}

	return false, nil
}

// effectiveRouteTables returns the route table(s) that apply to a subnet:
// its explicitly associated table, or the VPC main table when none is
// explicitly associated.
func effectiveRouteTables(ctx context.Context, c *ec2.Client, vpcID, subnetID string) ([]ec2types.RouteTable, error) {
	out, err := c.DescribeRouteTables(ctx, &ec2.DescribeRouteTablesInput{
		Filters: []ec2types.Filter{
			{Name: aws.String("vpc-id"), Values: []string{vpcID}},
			{Name: aws.String("association.subnet-id"), Values: []string{subnetID}},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("describe route tables for subnet %q: %w", subnetID, err)
	}

	if len(out.RouteTables) > 0 {
		return out.RouteTables, nil
	}

	mainOut, err := c.DescribeRouteTables(ctx, &ec2.DescribeRouteTablesInput{
		Filters: []ec2types.Filter{
			{Name: aws.String("vpc-id"), Values: []string{vpcID}},
			{Name: aws.String("association.main"), Values: []string{"true"}},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("describe main route table for vpc %q: %w", vpcID, err)
	}

	return mainOut.RouteTables, nil
}

func vpcHasSSMEndpoints(ctx context.Context, c *ec2.Client, vpcID string) (bool, error) {
	out, err := c.DescribeVpcEndpoints(ctx, &ec2.DescribeVpcEndpointsInput{
		Filters: []ec2types.Filter{
			{Name: aws.String("vpc-id"), Values: []string{vpcID}},
			{Name: aws.String("vpc-endpoint-type"), Values: []string{"Interface"}},
		},
	})
	if err != nil {
		return false, fmt.Errorf("describe vpc endpoints for vpc %q: %w", vpcID, err)
	}

	needed := map[string]bool{"ssm": false, "ssmmessages": false, "ec2messages": false}
	for _, ep := range out.VpcEndpoints {
		if ep.State != ec2types.StateAvailable || ep.ServiceName == nil {
			continue
		}

		for suffix := range needed {
			if strings.HasSuffix(*ep.ServiceName, "."+suffix) {
				needed[suffix] = true
			}
		}
	}

	for _, ok := range needed {
		if !ok {
			return false, nil
		}
	}

	return true, nil
}
