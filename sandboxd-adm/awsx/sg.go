package awsx

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

type IngressRule struct {
	// Port is used as both from/to when ToPort is zero (single-port rule).
	Port        int32
	ToPort      int32
	CIDR        string
	Description string
	// SourceSGID, when set, scopes the rule to traffic from another
	// security group instead of a CIDR (used to lock down orch<->let
	// traffic to only the cluster's own nodes).
	SourceSGID string
}

func EnsureSecurityGroup(ctx context.Context, c *ec2.Client, vpcID, name, description string, rules []IngressRule) (string, error) {
	existing, managed, err := findSecurityGroupByName(ctx, c, vpcID, name)
	if err != nil {
		return "", err
	}

	var sgID string
	if existing != "" {
		if !managed {
			return "", fmt.Errorf("security group %q already exists in vpc %q but is not managed by sbxadm (missing ManagedBy=sbxadm tag); refusing to take it over", name, vpcID)
		}
		sgID = existing
	} else {
		out, err := c.CreateSecurityGroup(ctx, &ec2.CreateSecurityGroupInput{
			GroupName:   aws.String(name),
			Description: aws.String(description),
			VpcId:       aws.String(vpcID),
			TagSpecifications: []ec2types.TagSpecification{
				{
					ResourceType: ec2types.ResourceTypeSecurityGroup,
					Tags:         nameTags(name),
				},
			},
		})
		if err != nil {
			return "", fmt.Errorf("create security group %q: %w", name, err)
		}

		sgID = *out.GroupId
	}

	if err := reconcileIngress(ctx, c, sgID, rules); err != nil {
		return sgID, err
	}

	return sgID, nil
}

func findSecurityGroupByName(ctx context.Context, c *ec2.Client, vpcID, name string) (id string, managed bool, err error) {
	out, err := c.DescribeSecurityGroups(ctx, &ec2.DescribeSecurityGroupsInput{
		Filters: []ec2types.Filter{
			{Name: aws.String("vpc-id"), Values: []string{vpcID}},
			{Name: aws.String("group-name"), Values: []string{name}},
		},
	})
	if err != nil {
		return "", false, fmt.Errorf("describe security groups %q: %w", name, err)
	}

	if len(out.SecurityGroups) == 0 {
		return "", false, nil
	}

	sg := out.SecurityGroups[0]
	for _, t := range sg.Tags {
		if aws.ToString(t.Key) == "ManagedBy" && aws.ToString(t.Value) == "sbxadm" {
			managed = true
			break
		}
	}

	return *sg.GroupId, managed, nil
}

func reconcileIngress(ctx context.Context, c *ec2.Client, sgID string, rules []IngressRule) error {
	desc, err := c.DescribeSecurityGroups(ctx, &ec2.DescribeSecurityGroupsInput{GroupIds: []string{sgID}})
	if err != nil {
		return fmt.Errorf("describe security group %q: %w", sgID, err)
	}

	if len(desc.SecurityGroups) == 0 {
		return fmt.Errorf("security group %q disappeared", sgID)
	}

	if perms := desc.SecurityGroups[0].IpPermissions; len(perms) > 0 {
		if _, err := c.RevokeSecurityGroupIngress(ctx, &ec2.RevokeSecurityGroupIngressInput{
			GroupId:       aws.String(sgID),
			IpPermissions: perms,
		}); err != nil {
			return fmt.Errorf("revoke existing ingress on %q: %w", sgID, err)
		}
	}

	if len(rules) == 0 {
		return nil
	}

	perms := make([]ec2types.IpPermission, 0, len(rules))
	for _, r := range rules {
		toPort := r.ToPort
		if toPort == 0 {
			toPort = r.Port
		}

		perm := ec2types.IpPermission{
			IpProtocol: aws.String("tcp"),
			FromPort:   aws.Int32(r.Port),
			ToPort:     aws.Int32(toPort),
		}
		if r.SourceSGID != "" {
			perm.UserIdGroupPairs = []ec2types.UserIdGroupPair{{GroupId: aws.String(r.SourceSGID), Description: aws.String(r.Description)}}
		} else {
			perm.IpRanges = []ec2types.IpRange{{CidrIp: aws.String(r.CIDR), Description: aws.String(r.Description)}}
		}

		perms = append(perms, perm)
	}

	if _, err := c.AuthorizeSecurityGroupIngress(ctx, &ec2.AuthorizeSecurityGroupIngressInput{
		GroupId:       aws.String(sgID),
		IpPermissions: perms,
	}); err != nil {
		return fmt.Errorf("authorize ingress on %q: %w", sgID, err)
	}

	return nil
}

func DeleteSecurityGroup(ctx context.Context, c *ec2.Client, sgID string) error {
	if sgID == "" {
		return nil
	}

	_, err := c.DeleteSecurityGroup(ctx, &ec2.DeleteSecurityGroupInput{GroupId: aws.String(sgID)})
	if err != nil {
		if isNotFound(err) {
			return nil
		}

		return fmt.Errorf("delete security group %q: %w", sgID, err)
	}

	return nil
}

func nameTags(name string) []ec2types.Tag {
	return []ec2types.Tag{
		{Key: aws.String("Name"), Value: aws.String(name)},
		{Key: aws.String("ManagedBy"), Value: aws.String("sbxadm")},
	}
}
