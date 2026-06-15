package model

import (
	"fmt"
	"path"
	"regexp"
	"strings"

	k8sresource "k8s.io/apimachinery/pkg/api/resource"
)

func ValidateSandboxID(id string) error {
	v := strings.TrimSpace(id)
	if v == "" {
		return fmt.Errorf("id is required")
	}

	if v != id {
		return fmt.Errorf("id must not contain leading or trailing whitespace")
	}

	if strings.Contains(v, "/") || strings.Contains(v, "\\") || strings.Contains(v, "..") {
		return fmt.Errorf("id contains invalid path characters")
	}

	if !safeSandboxIDRe.MatchString(v) {
		return fmt.Errorf("id contains unsupported characters")
	}

	return nil
}

var safeSandboxIDRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,127}$`)
var safeVolumeNameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$`)

func (r CreateSandboxRequest) Validate() error {
	if err := ValidateSandboxID(r.ID); err != nil {
		return err
	}

	if len(r.Containers) < 1 {
		return fmt.Errorf("at least one container is required")
	}

	seenNames := map[string]struct{}{}
	knownVolumes := map[string]struct{}{}
	for _, v := range r.Volumes {
		name := strings.TrimSpace(v.Name)
		if name == "" {
			return fmt.Errorf("volume name is required")
		}

		if !safeVolumeNameRe.MatchString(name) {
			return fmt.Errorf("volume %s: unsupported name", v.Name)
		}

		if strings.TrimSpace(v.EphemeralStorage) == "" {
			return fmt.Errorf("volume %s: ephemeralStorage is required", name)
		}

		q, err := k8sresource.ParseQuantity(strings.TrimSpace(v.EphemeralStorage))
		if err != nil || q.Value() <= 0 {
			return fmt.Errorf("volume %s: invalid ephemeralStorage", name)
		}

		if _, ok := knownVolumes[name]; ok {
			return fmt.Errorf("duplicate volume name: %s", name)
		}

		knownVolumes[name] = struct{}{}
	}

	for _, c := range r.Containers {
		if c.Name == "" || c.Image == "" {
			return fmt.Errorf("container name and image are required")
		}

		if strings.TrimSpace(c.Resource.CPU) == "" {
			return fmt.Errorf("container %s: cpu is required", c.Name)
		}

		if strings.TrimSpace(c.Resource.Memory) == "" {
			return fmt.Errorf("container %s: memory is required", c.Name)
		}

		if _, ok := seenNames[c.Name]; ok {
			return fmt.Errorf("duplicate container name: %s", c.Name)
		}

		seenNames[c.Name] = struct{}{}

		seenMountPaths := map[string]struct{}{}
		for _, vm := range c.VolumeMounts {
			volName := strings.TrimSpace(vm.Name)
			if volName == "" {
				return fmt.Errorf("container %s: volume mount name is required", c.Name)
			}

			if _, ok := knownVolumes[volName]; !ok {
				return fmt.Errorf("container %s: unknown volume %s", c.Name, volName)
			}

			mountPath := strings.TrimSpace(vm.MountPath)
			if mountPath == "" {
				return fmt.Errorf("container %s: mountPath is required for volume %s", c.Name, volName)
			}

			if !strings.HasPrefix(mountPath, "/") {
				return fmt.Errorf("container %s: mountPath must be absolute for volume %s", c.Name, volName)
			}

			if path.Clean(mountPath) != mountPath {
				return fmt.Errorf("container %s: mountPath must be clean for volume %s", c.Name, volName)
			}

			if mountPath == "/" {
				return fmt.Errorf("container %s: mountPath '/' is not allowed for volume %s", c.Name, volName)
			}

			if mountPath == "/tmp" || strings.HasPrefix(mountPath, "/tmp/") {
				return fmt.Errorf("container %s: mountPath /tmp is reserved", c.Name)
			}

			if _, ok := seenMountPaths[mountPath]; ok {
				return fmt.Errorf("container %s: duplicate mountPath %s", c.Name, mountPath)
			}

			seenMountPaths[mountPath] = struct{}{}
		}
	}

	seenHostPorts := map[int]struct{}{}
	for _, p := range r.Ports {
		if p.HostPort < 1 || p.HostPort > 65535 || p.ContainerPort < 1 || p.ContainerPort > 65535 {
			return fmt.Errorf("invalid port mapping: %d:%d", p.HostPort, p.ContainerPort)
		}

		if p.HostPort < 1024 {
			return fmt.Errorf("privileged host ports are not allowed: %d", p.HostPort)
		}

		proto := strings.ToLower(strings.TrimSpace(p.Protocol))
		if proto == "" {
			proto = "tcp"
		}

		if proto != "tcp" && proto != "udp" {
			return fmt.Errorf("unsupported protocol: %s", p.Protocol)
		}

		if _, ok := seenHostPorts[p.HostPort]; ok {
			return fmt.Errorf("duplicate host port: %d", p.HostPort)
		}

		seenHostPorts[p.HostPort] = struct{}{}
	}

	if r.Readiness != nil {
		p := r.Readiness
		proto := strings.ToLower(strings.TrimSpace(p.Protocol))
		if proto == "" {
			return fmt.Errorf("readinessProbe.protocol is required")
		}

		if proto != "tcp" && proto != "http" {
			return fmt.Errorf("readinessProbe.protocol must be tcp or http")
		}

		if p.Port < 1 || p.Port > 65535 {
			return fmt.Errorf("readinessProbe.port must be between 1 and 65535")
		}

		if proto == "http" {
			path := strings.TrimSpace(p.Path)
			if path == "" {
				return fmt.Errorf("readinessProbe.path is required when protocol is http")
			}

			if !strings.HasPrefix(path, "/") {
				return fmt.Errorf("readinessProbe.path must start with '/' when protocol is http")
			}
		}

		if p.InitialDelaySeconds < 1 {
			return fmt.Errorf("readinessProbe.initialDelaySeconds must be >= 1")
		}

		if p.PeriodSeconds < 1 {
			return fmt.Errorf("readinessProbe.periodSeconds must be >= 1")
		}

		if p.TimeoutSeconds < 1 {
			return fmt.Errorf("readinessProbe.timeoutSeconds must be >= 1")
		}

		if p.SuccessThreshold < 1 {
			return fmt.Errorf("readinessProbe.successThreshold must be >= 1")
		}

		if p.FailureThreshold < 1 {
			return fmt.Errorf("readinessProbe.failureThreshold must be >= 1")
		}
	}

	return nil
}
