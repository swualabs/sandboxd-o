package model

import (
	"fmt"
	"strings"
)

func (r CreateSandboxRequest) Validate() error {
	if r.ID == "" {
		return fmt.Errorf("id is required")
	}

	if len(r.Containers) < 1 {
		return fmt.Errorf("at least one container is required")
	}

	seenNames := map[string]struct{}{}
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
