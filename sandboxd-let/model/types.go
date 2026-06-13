package model

import "time"

type VolumeSpec struct {
	Name             string `json:"name"`
	EphemeralStorage string `json:"ephemeralStorage"`
}

type VolumeMount struct {
	Name      string `json:"name"`
	MountPath string `json:"mountPath"`
	ReadOnly  bool   `json:"readOnly,omitempty"`
}

type NodeResourceSnapshot struct {
	CapacityCPUMilli    int64     `json:"capacity_cpu_milli"`
	CapacityMemoryBytes int64     `json:"capacity_memory_bytes"`
	AllocatableCPUMilli int64     `json:"allocatable_cpu_milli"`
	AllocatableMemory   int64     `json:"allocatable_memory_bytes"`
	UsedCPUMilli        int64     `json:"used_cpu_milli"`
	UsedMemoryBytes     int64     `json:"used_memory_bytes"`
	AvailableCPUMilli   int64     `json:"available_cpu_milli"`
	AvailableMemory     int64     `json:"available_memory_bytes"`
	MaxAllocPercent     int       `json:"max_alloc_percent"`
	UpdatedAt           time.Time `json:"updated_at"`
}

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
	Volumes     []VolumeSpec              `json:"volumes,omitempty"`
	Containers  map[string]ContainerState `json:"containers"`
	PauseID     string                    `json:"pauseID,omitempty"`
	CNIConfPath string                    `json:"cniConfPath"`
	CreatedAt   time.Time                 `json:"createdAt"`
	UpdatedAt   time.Time                 `json:"updatedAt"`
}

type ContainerState struct {
	ID           string        `json:"id"`
	Name         string        `json:"name"`
	Phase        string        `json:"phase"`
	Error        string        `json:"error,omitempty"`
	Image        string        `json:"image"`
	Args         []string      `json:"args,omitempty"`
	Env          []string      `json:"env,omitempty"`
	VolumeMounts []VolumeMount `json:"volumeMounts,omitempty"`
	Resource     ResourceSpec  `json:"resource"`
	TaskPID      uint32        `json:"taskPID"`
	Runtime      string        `json:"runtime"`
	TaskStatus   string        `json:"taskStatus,omitempty"`
}

type CreateSandboxRequest struct {
	ID         string                   `json:"id"`
	Egress     bool                     `json:"egress"`
	Volumes    []VolumeSpec             `json:"volumes,omitempty"`
	Containers []CreateContainerRequest `json:"containers"`
	Ports      []PortMapping            `json:"ports"`
	Readiness  *ReadinessProbeSpec      `json:"readinessProbe,omitempty"`
}

type ReadinessProbeSpec struct {
	Protocol            string `json:"protocol"`
	Port                int    `json:"port"`
	Path                string `json:"path,omitempty"`
	InitialDelaySeconds int    `json:"initialDelaySeconds"`
	PeriodSeconds       int    `json:"periodSeconds"`
	TimeoutSeconds      int    `json:"timeoutSeconds"`
	SuccessThreshold    int    `json:"successThreshold"`
	FailureThreshold    int    `json:"failureThreshold"`
}

type CreateContainerRequest struct {
	Name         string        `json:"name"`
	Image        string        `json:"image"`
	Args         []string      `json:"args"`
	Env          []string      `json:"env"`
	WorkDir      string        `json:"workDir"`
	VolumeMounts []VolumeMount `json:"volumeMounts,omitempty"`
	Resource     ResourceSpec  `json:"resource"`
}

type ResourceLimits struct {
	MemoryBytes int64  `json:"memoryBytes"`
	CPUQuota    int64  `json:"cpuQuota"`
	CPUPeriod   uint64 `json:"cpuPeriod"`
	PidsLimit   int64  `json:"pidsLimit"`
	RootfsBytes int64  `json:"rootfsBytes,omitempty"`
	TmpfsBytes  int64  `json:"tmpfsBytes,omitempty"`
}

type ResourceSpec struct {
	CPU              string `json:"cpu"`
	Memory           string `json:"memory"`
	EphemeralStorage string `json:"ephemeralStorage,omitempty"`
}

type PortMapping struct {
	HostPort      int    `json:"hostPort"`
	ContainerPort int    `json:"containerPort"`
	Protocol      string `json:"protocol"`
}
