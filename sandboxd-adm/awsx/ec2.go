package awsx

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

type LaunchSpec struct {
	Name               string
	InstanceType       string
	SubnetID           string
	SecurityGroupIDs   []string
	RootVolumeSizeGB   int32
	AssignPublicIP     bool
	UserData           string // plain text, base64-encoded internally
	Tags               map[string]string
	IAMInstanceProfile string // name, not ARN; grants SSM access (no SSH key needed)
}

// InstanceTypeArch returns the CPU architecture ("x86_64" or "arm64") the
// given EC2 instance type requires, so the matching AMI can be selected.
func InstanceTypeArch(ctx context.Context, c *ec2.Client, instanceType string) (string, error) {
	out, err := c.DescribeInstanceTypes(ctx, &ec2.DescribeInstanceTypesInput{
		InstanceTypes: []ec2types.InstanceType{ec2types.InstanceType(instanceType)},
	})
	if err != nil {
		return "", fmt.Errorf("describe instance type %q: %w", instanceType, err)
	}

	if len(out.InstanceTypes) == 0 || out.InstanceTypes[0].ProcessorInfo == nil {
		return "", fmt.Errorf("instance type %q not found in this region", instanceType)
	}

	archs := out.InstanceTypes[0].ProcessorInfo.SupportedArchitectures
	hasX86, hasArm := false, false
	for _, a := range archs {
		switch a {
		case ec2types.ArchitectureTypeX8664:
			hasX86 = true
		case ec2types.ArchitectureTypeArm64:
			hasArm = true
		}
	}
	switch {
	case hasX86:
		return "x86_64", nil
	case hasArm:
		return "arm64", nil
	default:
		return "", fmt.Errorf("instance type %q has no x86_64/arm64 architecture (got %v)", instanceType, archs)
	}
}

// LatestUbuntuAMIForInstanceType resolves the newest Ubuntu 22.04 AMI whose
// architecture matches the instance type, avoiding an opaque RunInstances
// failure when an ARM/Graviton type is paired with an x86_64 image.
func LatestUbuntuAMIForInstanceType(ctx context.Context, c *ec2.Client, instanceType string) (string, error) {
	arch, err := InstanceTypeArch(ctx, c, instanceType)
	if err != nil {
		return "", err
	}

	nameArch := "amd64"
	if arch == "arm64" {
		nameArch = "arm64"
	}

	out, err := c.DescribeImages(ctx, &ec2.DescribeImagesInput{
		Owners: []string{"099720109477"}, // Canonical
		Filters: []ec2types.Filter{
			{Name: aws.String("name"), Values: []string{fmt.Sprintf("ubuntu/images/hvm-ssd/ubuntu-jammy-22.04-%s-server-*", nameArch)}},
			{Name: aws.String("state"), Values: []string{"available"}},
			{Name: aws.String("architecture"), Values: []string{arch}},
			{Name: aws.String("virtualization-type"), Values: []string{"hvm"}},
		},
	})
	if err != nil {
		return "", fmt.Errorf("describe images (ubuntu 22.04 %s): %w", arch, err)
	}

	if len(out.Images) == 0 {
		return "", fmt.Errorf("no ubuntu 22.04 %s AMI found in this region", arch)
	}

	latest := out.Images[0]
	for _, img := range out.Images[1:] {
		if img.CreationDate != nil && latest.CreationDate != nil && *img.CreationDate > *latest.CreationDate {
			latest = img
		}
	}

	return *latest.ImageId, nil
}

func LaunchInstance(ctx context.Context, c *ec2.Client, amiID string, spec LaunchSpec) (instanceID, privateIP, publicIP string, err error) {
	tags := []ec2types.Tag{
		{Key: aws.String("Name"), Value: aws.String(spec.Name)},
		{Key: aws.String("ManagedBy"), Value: aws.String("sbxadm")},
	}
	for k, v := range spec.Tags {
		tags = append(tags, ec2types.Tag{Key: aws.String(k), Value: aws.String(v)})
	}

	input := &ec2.RunInstancesInput{
		ImageId:      aws.String(amiID),
		InstanceType: ec2types.InstanceType(spec.InstanceType),
		MinCount:     aws.Int32(1),
		MaxCount:     aws.Int32(1),
		UserData:     aws.String(base64Encode(spec.UserData)),
		BlockDeviceMappings: []ec2types.BlockDeviceMapping{
			{
				DeviceName: aws.String("/dev/sda1"),
				Ebs: &ec2types.EbsBlockDevice{
					VolumeSize:          aws.Int32(spec.RootVolumeSizeGB),
					VolumeType:          ec2types.VolumeTypeGp3,
					DeleteOnTermination: aws.Bool(true),
					Encrypted:           aws.Bool(true),
				},
			},
		},
		NetworkInterfaces: []ec2types.InstanceNetworkInterfaceSpecification{
			{
				DeviceIndex:              aws.Int32(0),
				SubnetId:                 aws.String(spec.SubnetID),
				Groups:                   spec.SecurityGroupIDs,
				AssociatePublicIpAddress: aws.Bool(spec.AssignPublicIP),
			},
		},
		TagSpecifications: []ec2types.TagSpecification{
			{ResourceType: ec2types.ResourceTypeInstance, Tags: tags},
			{ResourceType: ec2types.ResourceTypeVolume, Tags: tags},
		},
	}

	if spec.IAMInstanceProfile != "" {
		input.IamInstanceProfile = &ec2types.IamInstanceProfileSpecification{Name: aws.String(spec.IAMInstanceProfile)}
	}

	out, err := c.RunInstances(ctx, input)
	if err != nil {
		return "", "", "", fmt.Errorf("run instance %q: %w", spec.Name, err)
	}

	if len(out.Instances) == 0 {
		return "", "", "", fmt.Errorf("run instance %q: no instance returned", spec.Name)
	}
	instanceID = *out.Instances[0].InstanceId

	waiter := ec2.NewInstanceRunningWaiter(c)
	if err := waiter.Wait(ctx, &ec2.DescribeInstancesInput{InstanceIds: []string{instanceID}}, 5*time.Minute); err != nil {
		return instanceID, "", "", fmt.Errorf("wait for instance %q running: %w", instanceID, err)
	}

	desc, err := c.DescribeInstances(ctx, &ec2.DescribeInstancesInput{InstanceIds: []string{instanceID}})
	if err != nil || len(desc.Reservations) == 0 || len(desc.Reservations[0].Instances) == 0 {
		return instanceID, "", "", fmt.Errorf("describe instance %q after launch: %w", instanceID, err)
	}

	inst := desc.Reservations[0].Instances[0]
	if inst.PrivateIpAddress != nil {
		privateIP = *inst.PrivateIpAddress
	}

	if inst.PublicIpAddress != nil {
		publicIP = *inst.PublicIpAddress
	}

	return instanceID, privateIP, publicIP, nil
}

func TerminateInstance(ctx context.Context, c *ec2.Client, instanceID string) error {
	if instanceID == "" {
		return nil
	}

	_, err := c.TerminateInstances(ctx, &ec2.TerminateInstancesInput{InstanceIds: []string{instanceID}})
	if err != nil {
		if isNotFound(err) {
			return nil
		}
		return fmt.Errorf("terminate instance %q: %w", instanceID, err)
	}

	waiter := ec2.NewInstanceTerminatedWaiter(c)
	if err := waiter.Wait(ctx, &ec2.DescribeInstancesInput{InstanceIds: []string{instanceID}}, 5*time.Minute); err != nil {
		return fmt.Errorf("wait for instance %q terminated: %w", instanceID, err)
	}

	return nil
}

func ResizeInstanceType(ctx context.Context, c *ec2.Client, instanceID, instanceType string) error {
	if instanceID == "" {
		return fmt.Errorf("instance id is required")
	}

	if instanceType == "" {
		return fmt.Errorf("instance type is required")
	}

	if _, err := c.StopInstances(ctx, &ec2.StopInstancesInput{InstanceIds: []string{instanceID}}); err != nil {
		if !isIncorrectInstanceStateForAlreadyStopped(err) {
			return fmt.Errorf("stop instance %q: %w", instanceID, err)
		}
	}

	stoppedWaiter := ec2.NewInstanceStoppedWaiter(c)
	if err := stoppedWaiter.Wait(ctx, &ec2.DescribeInstancesInput{InstanceIds: []string{instanceID}}, 10*time.Minute); err != nil {
		return fmt.Errorf("wait for instance %q stopped: %w", instanceID, err)
	}

	if _, err := c.ModifyInstanceAttribute(ctx, &ec2.ModifyInstanceAttributeInput{
		InstanceId: aws.String(instanceID),
		InstanceType: &ec2types.AttributeValue{
			Value: aws.String(instanceType),
		},
	}); err != nil {
		return fmt.Errorf("modify instance %q type to %q: %w", instanceID, instanceType, err)
	}

	if _, err := c.StartInstances(ctx, &ec2.StartInstancesInput{InstanceIds: []string{instanceID}}); err != nil {
		return fmt.Errorf("start instance %q: %w", instanceID, err)
	}

	runningWaiter := ec2.NewInstanceRunningWaiter(c)
	if err := runningWaiter.Wait(ctx, &ec2.DescribeInstancesInput{InstanceIds: []string{instanceID}}, 10*time.Minute); err != nil {
		return fmt.Errorf("wait for instance %q running: %w", instanceID, err)
	}

	return nil
}

func DescribeInstanceNetwork(ctx context.Context, c *ec2.Client, instanceID string) (privateIP, publicIP string, err error) {
	out, err := c.DescribeInstances(ctx, &ec2.DescribeInstancesInput{InstanceIds: []string{instanceID}})
	if err != nil {
		return "", "", fmt.Errorf("describe instance %q: %w", instanceID, err)
	}

	if len(out.Reservations) == 0 || len(out.Reservations[0].Instances) == 0 {
		return "", "", fmt.Errorf("instance %q not found", instanceID)
	}

	inst := out.Reservations[0].Instances[0]
	if inst.PrivateIpAddress != nil {
		privateIP = *inst.PrivateIpAddress
	}

	if inst.PublicIpAddress != nil {
		publicIP = *inst.PublicIpAddress
	}

	return privateIP, publicIP, nil
}

func AssociateEIPByAllocationID(ctx context.Context, c *ec2.Client, instanceID, allocationID string) (string, error) {
	_, err := c.AssociateAddress(ctx, &ec2.AssociateAddressInput{
		InstanceId:   aws.String(instanceID),
		AllocationId: aws.String(allocationID),
	})
	if err != nil {
		return "", fmt.Errorf("associate eip %q to instance %q: %w", allocationID, instanceID, err)
	}

	out, err := c.DescribeAddresses(ctx, &ec2.DescribeAddressesInput{AllocationIds: []string{allocationID}})
	if err != nil || len(out.Addresses) == 0 {
		return "", fmt.Errorf("describe eip %q after association: %w", allocationID, err)
	}

	ip := ""
	if out.Addresses[0].PublicIp != nil {
		ip = *out.Addresses[0].PublicIp
	}

	return ip, nil
}

func DisassociateEIP(ctx context.Context, c *ec2.Client, allocationID string) error {
	if allocationID == "" {
		return nil
	}

	out, err := c.DescribeAddresses(ctx, &ec2.DescribeAddressesInput{AllocationIds: []string{allocationID}})
	if err != nil {
		if isNotFound(err) {
			return nil
		}

		return fmt.Errorf("describe eip %q: %w", allocationID, err)
	}

	if len(out.Addresses) == 0 || out.Addresses[0].AssociationId == nil {
		return nil
	}

	_, err = c.DisassociateAddress(ctx, &ec2.DisassociateAddressInput{AssociationId: out.Addresses[0].AssociationId})
	if err != nil && !isNotFound(err) {
		return fmt.Errorf("disassociate eip %q: %w", allocationID, err)
	}

	return nil
}
