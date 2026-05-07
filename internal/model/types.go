package model

import "time"

type Sandbox struct {
	ID          string                    `json:"id"`
	Phase       string                    `json:"phase"`
	Error       string                    `json:"error,omitempty"`
	Namespace   string                    `json:"namespace"`
	IP          string                    `json:"ip"`
	SubnetCIDR  string                    `json:"subnetCIDR"`
	BridgeName  string                    `json:"bridgeName"`
	Egress      bool                      `json:"egress"`
	Ports       []PortMapping             `json:"ports,omitempty"`
	Containers  map[string]ContainerState `json:"containers"`
	CNIConfPath string                    `json:"cniConfPath"`
	CreatedAt   time.Time                 `json:"createdAt"`
	UpdatedAt   time.Time                 `json:"updatedAt"`
}

type ContainerState struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Phase       string   `json:"phase"`
	Error       string   `json:"error,omitempty"`
	Image       string   `json:"image"`
	Args        []string `json:"args,omitempty"`
	Env         []string `json:"env,omitempty"`
	SnapshotKey string   `json:"snapshotKey"`
	TaskPID     uint32   `json:"taskPID"`
	Runtime     string   `json:"runtime"`
	TaskStatus  string   `json:"taskStatus,omitempty"`
	ExitStatus  uint32   `json:"exitStatus,omitempty"`
	ExitTime    string   `json:"exitTime,omitempty"`
}

type CreateSandboxRequest struct {
	ID         string                   `json:"id"`
	Egress     bool                     `json:"egress"`
	Containers []CreateContainerRequest `json:"containers"`
	Ports      []PortMapping            `json:"ports"`
}

type CreateContainerRequest struct {
	Name    string         `json:"name"`
	Image   string         `json:"image"`
	Args    []string       `json:"args"`
	Env     []string       `json:"env"`
	WorkDir string         `json:"workDir"`
	Limits  ResourceLimits `json:"limits"`
}

type ResourceLimits struct {
	MemoryBytes int64  `json:"memoryBytes"`
	CPUQuota    int64  `json:"cpuQuota"`
	CPUPeriod   uint64 `json:"cpuPeriod"`
	PidsLimit   int64  `json:"pidsLimit"`
}

type PortMapping struct {
	HostPort      int    `json:"hostPort"`
	ContainerPort int    `json:"containerPort"`
	Protocol      string `json:"protocol"`
}
