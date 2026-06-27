package store

import "time"

type Cluster struct {
	Name          string    `dynamodbav:"name"`
	Version       string    `dynamodbav:"version"`
	Region        string    `dynamodbav:"region"`
	VPCID         string    `dynamodbav:"vpc_id"`
	PublicSubnets []string  `dynamodbav:"public_subnets"`
	PrivateSubnet []string  `dynamodbav:"private_subnets"`
	SharedSecret  string    `dynamodbav:"shared_secret"`
	SecurityGroup string    `dynamodbav:"security_group_id"`
	CreatedAt     time.Time `dynamodbav:"created_at"`
	UpdatedAt     time.Time `dynamodbav:"updated_at"`

	// WorkerSecurityGroup is shared by every worker in the cluster.
	WorkerSecurityGroup         string `dynamodbav:"worker_security_group_id,omitempty"`
	ControlPlaneInstanceProfile string `dynamodbav:"control_plane_instance_profile,omitempty"`
	WorkerInstanceProfile       string `dynamodbav:"worker_instance_profile,omitempty"`

	// Union of every --ecr-repos pattern granted so far, so a new
	// create-worker call can extend the shared worker role's allowlist
	// instead of clobbering it.
	WorkerECRRepoPatterns []string `dynamodbav:"worker_ecr_repo_patterns,omitempty"`

	ControlPlane ControlPlane `dynamodbav:"control_plane"`
	Workers      []Worker     `dynamodbav:"workers"`
}

type ControlPlane struct {
	InstanceID       string `dynamodbav:"instance_id"`
	InstanceType     string `dynamodbav:"instance_type"`
	SubnetID         string `dynamodbav:"subnet_id"`
	PublicEndpoint   bool   `dynamodbav:"public_endpoint"`
	PublicEIPAllocID string `dynamodbav:"public_eip_alloc_id,omitempty"`
	// PublicEIPManaged: true if sbxadm allocated this EIP itself (release
	// it on delete); false if the user supplied it (only disassociate).
	PublicEIPManaged bool   `dynamodbav:"public_eip_managed,omitempty"`
	PublicIP         string `dynamodbav:"public_ip,omitempty"`
	PrivateIP        string `dynamodbav:"private_ip"`
	RootVolumeSizeGB int32  `dynamodbav:"root_volume_size_gb"`
	ConfigJSON       string `dynamodbav:"config_json,omitempty"`
}

type Worker struct {
	Name             string    `dynamodbav:"name"`
	InstanceID       string    `dynamodbav:"instance_id"`
	InstanceType     string    `dynamodbav:"instance_type"`
	SubnetID         string    `dynamodbav:"subnet_id"`
	SecurityGroup    string    `dynamodbav:"security_group_id"`
	PublicEIPAllocID string    `dynamodbav:"public_eip_alloc_id,omitempty"`
	PublicEIPManaged bool      `dynamodbav:"public_eip_managed,omitempty"`
	PublicIP         string    `dynamodbav:"public_ip,omitempty"`
	PrivateIP        string    `dynamodbav:"private_ip"`
	RootVolumeSizeGB int32     `dynamodbav:"root_volume_size_gb"`
	External         string    `dynamodbav:"external,omitempty"`
	ConfigJSON       string    `dynamodbav:"config_json,omitempty"`
	CreatedAt        time.Time `dynamodbav:"created_at"`
	UpdatedAt        time.Time `dynamodbav:"updated_at"`
}
