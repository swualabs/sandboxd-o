package sandbox

import (
	"time"

	"sandboxd-o/sandboxd-let/model"
)

const (
	SandboxPhaseCreating = "creating"
	SandboxPhaseRunning  = "running"
	SandboxPhaseDeleting = "deleting"
	SandboxPhaseError    = "error"

	ContainerPhaseCreating = "creating"
	ContainerPhaseRunning  = "running"
	ContainerPhaseStopped  = "stopped"
	ContainerPhaseError    = "error"
	ContainerPhaseUnknown  = "unknown"
)

func setSandboxPhase(sbx *model.Sandbox, phase, errMsg string) {
	sbx.Phase = phase
	if errMsg != "" {
		sbx.Error = errMsg
	} else if phase != SandboxPhaseError {
		sbx.Error = ""
	}

	sbx.UpdatedAt = time.Now().UTC()
}

func (s *Service) newSandboxState(req model.CreateSandboxRequest) *model.Sandbox {
	now := time.Now().UTC()
	sbx := &model.Sandbox{
		ID:          req.ID,
		Phase:       SandboxPhaseCreating,
		Namespace:   s.namespace,
		Egress:      req.Egress,
		BridgeName:  s.bridgeIF,
		SubnetCIDR:  s.cidr,
		CNIConfPath: s.cniConfPath,
		Containers:  map[string]model.ContainerState{},
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	sbx.Ports = append(sbx.Ports, req.Ports...)
	sbx.Volumes = append(sbx.Volumes, req.Volumes...)
	for _, c := range req.Containers {
		sbx.Containers[c.Name] = model.ContainerState{
			ID:           sbx.ID + "-" + c.Name,
			Name:         c.Name,
			Phase:        ContainerPhaseCreating,
			Image:        c.Image,
			Args:         append([]string(nil), c.Args...),
			Env:          append([]string(nil), c.Env...),
			CapAdd:       append([]string(nil), c.CapAdd...),
			CapDrop:      append([]string(nil), c.CapDrop...),
			SecurityOpt:  append([]string(nil), c.SecurityOpt...),
			ReadOnly:     c.ReadOnly,
			Tmpfs:        append([]model.TmpfsMount(nil), c.Tmpfs...),
			VolumeMounts: append([]model.VolumeMount(nil), c.VolumeMounts...),
			Resource:     c.Resource,
			Runtime:      s.runtimeBinary,
		}
	}

	return sbx
}
