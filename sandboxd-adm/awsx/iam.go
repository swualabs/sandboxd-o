package awsx

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	iamtypes "github.com/aws/aws-sdk-go-v2/service/iam/types"
)

const ec2AssumeRolePolicy = `{
  "Version": "2012-10-17",
  "Statement": [{
    "Effect": "Allow",
    "Principal": {"Service": "ec2.amazonaws.com"},
    "Action": "sts:AssumeRole"
  }]
}`

func EnsureInstanceProfile(ctx context.Context, c *iam.Client, name string, extraPolicyARNs []string) (profileName string, err error) {
	roleName := name + "-role"
	profileName = name + "-profile"

	_, err = c.GetRole(ctx, &iam.GetRoleInput{RoleName: aws.String(roleName)})
	if err != nil {
		if !isIAMNotFound(err) {
			return "", fmt.Errorf("get iam role %q: %w", roleName, err)
		}

		if _, err := c.CreateRole(ctx, &iam.CreateRoleInput{
			RoleName:                 aws.String(roleName),
			AssumeRolePolicyDocument: aws.String(ec2AssumeRolePolicy),
			Tags: []iamtypes.Tag{
				{Key: aws.String("ManagedBy"), Value: aws.String("sbxadm")},
			},
		}); err != nil {
			return "", fmt.Errorf("create iam role %q: %w", roleName, err)
		}
	}

	policies := append([]string{"arn:aws:iam::aws:policy/AmazonSSMManagedInstanceCore"}, extraPolicyARNs...)
	for _, p := range policies {
		if _, err := c.AttachRolePolicy(ctx, &iam.AttachRolePolicyInput{
			RoleName:  aws.String(roleName),
			PolicyArn: aws.String(p),
		}); err != nil {
			return "", fmt.Errorf("attach policy %q to role %q: %w", p, roleName, err)
		}
	}

	_, err = c.GetInstanceProfile(ctx, &iam.GetInstanceProfileInput{InstanceProfileName: aws.String(profileName)})
	if err != nil {
		if !isIAMNotFound(err) {
			return "", fmt.Errorf("get instance profile %q: %w", profileName, err)
		}

		if _, err := c.CreateInstanceProfile(ctx, &iam.CreateInstanceProfileInput{
			InstanceProfileName: aws.String(profileName),
		}); err != nil {
			return "", fmt.Errorf("create instance profile %q: %w", profileName, err)
		}

		if _, err := c.AddRoleToInstanceProfile(ctx, &iam.AddRoleToInstanceProfileInput{
			InstanceProfileName: aws.String(profileName),
			RoleName:            aws.String(roleName),
		}); err != nil {
			return "", fmt.Errorf("add role %q to instance profile %q: %w", roleName, profileName, err)
		}

		// Instance profiles take a few seconds to propagate to EC2's
		// control plane; RunInstances can otherwise fail with
		// "Invalid IAM Instance Profile".
		time.Sleep(8 * time.Second)
	}

	return profileName, nil
}

func DeleteInstanceProfile(ctx context.Context, c *iam.Client, name string, extraPolicyARNs []string) error {
	roleName := name + "-role"
	profileName := name + "-profile"

	if _, err := c.RemoveRoleFromInstanceProfile(ctx, &iam.RemoveRoleFromInstanceProfileInput{
		InstanceProfileName: aws.String(profileName),
		RoleName:            aws.String(roleName),
	}); err != nil && !isIAMNotFound(err) {
		return fmt.Errorf("remove role %q from instance profile %q: %w", roleName, profileName, err)
	}

	if _, err := c.DeleteInstanceProfile(ctx, &iam.DeleteInstanceProfileInput{InstanceProfileName: aws.String(profileName)}); err != nil && !isIAMNotFound(err) {
		return fmt.Errorf("delete instance profile %q: %w", profileName, err)
	}

	policies := append([]string{"arn:aws:iam::aws:policy/AmazonSSMManagedInstanceCore"}, extraPolicyARNs...)
	for _, p := range policies {
		if _, err := c.DetachRolePolicy(ctx, &iam.DetachRolePolicyInput{RoleName: aws.String(roleName), PolicyArn: aws.String(p)}); err != nil && !isIAMNotFound(err) {
			return fmt.Errorf("detach policy %q from role %q: %w", p, roleName, err)
		}
	}

	if _, err := c.DeleteRole(ctx, &iam.DeleteRoleInput{RoleName: aws.String(roleName)}); err != nil && !isIAMNotFound(err) {
		return fmt.Errorf("delete role %q: %w", roleName, err)
	}

	return nil
}

func isIAMNotFound(err error) bool {
	var nfe *iamtypes.NoSuchEntityException
	return errors.As(err, &nfe)
}
