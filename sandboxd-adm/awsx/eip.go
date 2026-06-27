package awsx

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

func AllocateEIP(ctx context.Context, c *ec2.Client, name string) (allocationID, publicIP string, err error) {
	out, err := c.AllocateAddress(ctx, &ec2.AllocateAddressInput{
		Domain: ec2types.DomainTypeVpc,
		TagSpecifications: []ec2types.TagSpecification{
			{ResourceType: ec2types.ResourceTypeElasticIp, Tags: nameTags(name)},
		},
	})
	if err != nil {
		return "", "", fmt.Errorf("allocate elastic ip %q: %w", name, err)
	}

	allocationID = aws.ToString(out.AllocationId)
	publicIP = aws.ToString(out.PublicIp)
	return allocationID, publicIP, nil
}

func ReleaseEIP(ctx context.Context, c *ec2.Client, allocationID string) error {
	if allocationID == "" {
		return nil
	}

	if err := DisassociateEIP(ctx, c, allocationID); err != nil {
		return err
	}

	_, err := c.ReleaseAddress(ctx, &ec2.ReleaseAddressInput{AllocationId: aws.String(allocationID)})
	if err != nil && !isNotFound(err) {
		return fmt.Errorf("release elastic ip %q: %w", allocationID, err)
	}

	return nil
}
