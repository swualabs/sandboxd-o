package model

import (
	"fmt"
	"path"
	"regexp"
	"sort"
	"strconv"
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
var safeContainerNameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$`)
var safeCapabilityNameRe = regexp.MustCompile(`^[A-Z0-9_]+$`)

// ValidateContainerName enforces a strict allowlist for container names.
//
// The container name is used to build host-side paths (for example the per-container
// tmpfs mount path StateBaseDir/<sandboxID>/tmpfs/<containerName>/tmp), so it must not
// be allowed to contain path separators or traversal sequences. See issue #21.
func ValidateContainerName(name string) error {
	v := strings.TrimSpace(name)
	if v == "" {
		return fmt.Errorf("container name is required")
	}

	if v != name {
		return fmt.Errorf("container name must not contain leading or trailing whitespace")
	}

	if strings.Contains(v, "/") || strings.Contains(v, "\\") || strings.Contains(v, "..") {
		return fmt.Errorf("container name contains invalid path characters")
	}

	if !safeContainerNameRe.MatchString(v) {
		return fmt.Errorf("container name contains unsupported characters")
	}

	return nil
}

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
		if err := ValidateContainerName(c.Name); err != nil {
			return err
		}

		if c.Image == "" {
			return fmt.Errorf("container %s: image is required", c.Name)
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

		for _, mount := range c.Tmpfs {
			mountPath := strings.TrimSpace(mount.MountPath)
			if mountPath == "" {
				return fmt.Errorf("container %s: tmpfs mountPath is required", c.Name)
			}

			if !strings.HasPrefix(mountPath, "/") {
				return fmt.Errorf("container %s: tmpfs mountPath must be absolute", c.Name)
			}

			if path.Clean(mountPath) != mountPath {
				return fmt.Errorf("container %s: tmpfs mountPath must be clean", c.Name)
			}

			if mountPath == "/" {
				return fmt.Errorf("container %s: tmpfs mountPath '/' is not allowed", c.Name)
			}

			if _, ok := seenMountPaths[mountPath]; ok {
				return fmt.Errorf("container %s: duplicate mountPath %s", c.Name, mountPath)
			}

			seenMountPaths[mountPath] = struct{}{}
			if err := validateTmpfsOptions(mount.Options); err != nil {
				return fmt.Errorf("container %s: tmpfs %s: %w", c.Name, mountPath, err)
			}
		}

		for _, capName := range c.CapAdd {
			if err := validateCapabilityName(capName); err != nil {
				return fmt.Errorf("container %s: invalid capAdd entry: %w", c.Name, err)
			}
		}

		for _, capName := range c.CapDrop {
			if err := validateCapabilityName(capName); err != nil {
				return fmt.Errorf("container %s: invalid capDrop entry: %w", c.Name, err)
			}
		}

		for _, raw := range c.SecurityOpt {
			if err := validateSecurityOpt(raw); err != nil {
				return fmt.Errorf("container %s: %w", c.Name, err)
			}
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

func validateCapabilityName(name string) error {
	v := strings.TrimSpace(name)
	if v == "" {
		return fmt.Errorf("capability is empty")
	}
	if !safeCapabilityNameRe.MatchString(v) {
		return fmt.Errorf("unsupported capability %q", name)
	}
	return nil
}

func validateSecurityOpt(raw string) error {
	v := strings.TrimSpace(raw)
	if v == "" {
		return fmt.Errorf("securityOpt entry is empty")
	}

	key, value, ok := splitOption(v)
	if !ok {
		return fmt.Errorf("securityOpt %q must use key=value or key:value", raw)
	}

	switch strings.ToLower(key) {
	case "no-new-privileges":
		if _, err := strconv.ParseBool(strings.TrimSpace(value)); err != nil {
			return fmt.Errorf("securityOpt %q has invalid boolean", raw)
		}
	case "seccomp", "apparmor":
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("securityOpt %q requires a profile value", raw)
		}
	default:
		return fmt.Errorf("unsupported securityOpt %q", raw)
	}

	return nil
}

func validateTmpfsOptions(raw string) error {
	if strings.TrimSpace(raw) == "" {
		return nil
	}

	for _, item := range strings.Split(raw, ",") {
		part := strings.TrimSpace(item)
		if part == "" {
			return fmt.Errorf("empty option")
		}

		switch part {
		case "ro", "rw", "nosuid", "suid", "nodev", "dev", "noexec", "exec":
			continue
		}

		key, value, ok := splitOption(part)
		if !ok {
			return fmt.Errorf("unsupported option %q", part)
		}

		switch strings.ToLower(key) {
		case "mode":
			if value == "" {
				return fmt.Errorf("mode requires a value")
			}
			if _, err := strconv.ParseUint(value, 8, 32); err != nil {
				return fmt.Errorf("invalid mode %q", value)
			}
		case "size":
			if strings.TrimSpace(value) == "" {
				return fmt.Errorf("size requires a value")
			}
			if _, err := parseByteSize(strings.TrimSpace(value)); err != nil {
				return fmt.Errorf("invalid size %q", value)
			}
		default:
			return fmt.Errorf("unsupported option %q", part)
		}
	}

	return nil
}

func splitOption(raw string) (string, string, bool) {
	if key, value, ok := strings.Cut(raw, "="); ok {
		return strings.TrimSpace(key), strings.TrimSpace(value), true
	}
	if key, value, ok := strings.Cut(raw, ":"); ok {
		return strings.TrimSpace(key), strings.TrimSpace(value), true
	}
	return "", "", false
}

func parseByteSize(raw string) (int64, error) {
	v := strings.TrimSpace(raw)
	if v == "" {
		return 0, fmt.Errorf("size is required")
	}

	type suffixDef struct {
		suffix string
		scale  int64
	}
	suffixes := []suffixDef{
		{"kib", 1024},
		{"mib", 1024 * 1024},
		{"gib", 1024 * 1024 * 1024},
		{"tib", 1024 * 1024 * 1024 * 1024},
		{"ki", 1024},
		{"mi", 1024 * 1024},
		{"gi", 1024 * 1024 * 1024},
		{"ti", 1024 * 1024 * 1024 * 1024},
		{"kb", 1000},
		{"mb", 1000 * 1000},
		{"gb", 1000 * 1000 * 1000},
		{"tb", 1000 * 1000 * 1000 * 1000},
		{"k", 1000},
		{"m", 1000 * 1000},
		{"g", 1000 * 1000 * 1000},
		{"t", 1000 * 1000 * 1000 * 1000},
		{"b", 1},
	}
	sort.Slice(suffixes, func(i, j int) bool { return len(suffixes[i].suffix) > len(suffixes[j].suffix) })

	lower := strings.ToLower(v)
	for _, def := range suffixes {
		if !strings.HasSuffix(lower, def.suffix) {
			continue
		}
		num := strings.TrimSpace(v[:len(v)-len(def.suffix)])
		if num == "" {
			return 0, fmt.Errorf("size is required")
		}
		n, err := strconv.ParseInt(num, 10, 64)
		if err != nil || n <= 0 {
			return 0, fmt.Errorf("invalid size %q", raw)
		}
		return n * def.scale, nil
	}

	if q, err := k8sresource.ParseQuantity(v); err == nil && q.Value() > 0 {
		return q.Value(), nil
	}

	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("invalid size %q", raw)
	}
	return n, nil
}
