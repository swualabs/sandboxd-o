package sandbox

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"sandboxd-o/sandboxd-let/model"

	runtimeapi "k8s.io/cri-api/pkg/apis/runtime/v1"
)

type resolvedTmpfsMount struct {
	MountPath string
	ReadOnly  bool
	SizeBytes int64
	Mode      string
	NoSuid    bool
	NoDev     bool
	NoExec    bool
}

type resolvedContainerSecurity struct {
	Capabilities *runtimeapi.Capability
	NoNewPrivs   bool
	Seccomp      *runtimeapi.SecurityProfile
	AppArmor     *runtimeapi.SecurityProfile
}

func resolveContainerSecurityOptions(c model.CreateContainerRequest) (resolvedContainerSecurity, error) {
	out := resolvedContainerSecurity{
		NoNewPrivs: true,
		Seccomp: &runtimeapi.SecurityProfile{
			ProfileType: runtimeapi.SecurityProfile_RuntimeDefault,
		},
	}

	if len(c.CapAdd) > 0 || len(c.CapDrop) > 0 {
		out.Capabilities = &runtimeapi.Capability{
			AddCapabilities:  append([]string(nil), c.CapAdd...),
			DropCapabilities: append([]string(nil), c.CapDrop...),
		}
	}

	for _, raw := range c.SecurityOpt {
		key, value, _ := strings.Cut(raw, "=")
		if strings.TrimSpace(value) == "" {
			key, value, _ = strings.Cut(raw, ":")
		}
		key = strings.ToLower(strings.TrimSpace(key))
		value = strings.TrimSpace(value)

		switch key {
		case "no-new-privileges":
			v, err := strconv.ParseBool(value)
			if err != nil {
				return resolvedContainerSecurity{}, fmt.Errorf("invalid security_opt %q", raw)
			}
			out.NoNewPrivs = v
		case "seccomp":
			profile, err := resolveSecurityProfile(value, true)
			if err != nil {
				return resolvedContainerSecurity{}, err
			}
			out.Seccomp = profile
		case "apparmor":
			profile, err := resolveSecurityProfile(value, false)
			if err != nil {
				return resolvedContainerSecurity{}, err
			}
			out.AppArmor = profile
		default:
			return resolvedContainerSecurity{}, fmt.Errorf("unsupported security_opt %q", raw)
		}
	}

	return out, nil
}

func resolveSecurityProfile(value string, seccomp bool) (*runtimeapi.SecurityProfile, error) {
	v := strings.TrimSpace(value)
	switch strings.ToLower(v) {
	case "", "runtime/default", "runtimedefault", "default":
		return &runtimeapi.SecurityProfile{ProfileType: runtimeapi.SecurityProfile_RuntimeDefault}, nil
	case "unconfined":
		return &runtimeapi.SecurityProfile{ProfileType: runtimeapi.SecurityProfile_Unconfined}, nil
	}

	if strings.HasPrefix(v, "localhost/") {
		ref := strings.TrimPrefix(v, "localhost/")
		if seccomp && !strings.HasPrefix(ref, "/") {
			return nil, fmt.Errorf("localhost seccomp profile must use an absolute path")
		}
		if strings.TrimSpace(ref) == "" {
			return nil, fmt.Errorf("localhost profile reference is empty")
		}
		return &runtimeapi.SecurityProfile{
			ProfileType:  runtimeapi.SecurityProfile_Localhost,
			LocalhostRef: ref,
		}, nil
	}

	return nil, fmt.Errorf("unsupported profile value %q", value)
}

func resolveTmpfsMountSpec(in model.TmpfsMount) (resolvedTmpfsMount, error) {
	out := resolvedTmpfsMount{
		MountPath: strings.TrimSpace(in.MountPath),
		Mode:      "1777",
		NoSuid:    true,
		NoDev:     true,
	}
	if out.MountPath == "" {
		return resolvedTmpfsMount{}, fmt.Errorf("mountPath is required")
	}

	if strings.TrimSpace(in.Options) == "" {
		return out, nil
	}

	for _, token := range strings.Split(in.Options, ",") {
		part := strings.TrimSpace(token)
		switch part {
		case "", "rw":
			continue
		case "ro":
			out.ReadOnly = true
		case "nosuid":
			out.NoSuid = true
		case "suid":
			out.NoSuid = false
		case "nodev":
			out.NoDev = true
		case "dev":
			out.NoDev = false
		case "noexec":
			out.NoExec = true
		case "exec":
			out.NoExec = false
		default:
			key, value, ok := strings.Cut(part, "=")
			if !ok {
				key, value, ok = strings.Cut(part, ":")
			}
			if !ok {
				return resolvedTmpfsMount{}, fmt.Errorf("unsupported option %q", part)
			}

			switch strings.ToLower(strings.TrimSpace(key)) {
			case "mode":
				out.Mode = strings.TrimSpace(value)
			case "size":
				bytes, err := parseByteSize(strings.TrimSpace(value))
				if err != nil {
					return resolvedTmpfsMount{}, fmt.Errorf("invalid size %q", value)
				}
				out.SizeBytes = bytes
			default:
				return resolvedTmpfsMount{}, fmt.Errorf("unsupported option %q", part)
			}
		}
	}

	return out, nil
}

func (s *Service) ensureSandboxCustomTmpfsMount(sandboxID, containerName string, spec resolvedTmpfsMount) (string, error) {
	if err := model.ValidateSandboxID(sandboxID); err != nil {
		return "", err
	}
	if err := model.ValidateContainerName(containerName); err != nil {
		return "", err
	}

	sandboxTmpfsRoot := filepath.Join(s.cfg.StateBaseDir, sandboxID, "tmpfs")
	sum := sha256.Sum256([]byte(spec.MountPath))
	base := filepath.Join(sandboxTmpfsRoot, containerName, "mounts", hex.EncodeToString(sum[:16]))
	sourcePath := filepath.Join(base, "source")
	exportPath := filepath.Join(base, "export")
	if !isWithinBase(sandboxTmpfsRoot, sourcePath) || !isWithinBase(sandboxTmpfsRoot, exportPath) {
		return "", fmt.Errorf("tmpfs mount path %s escapes sandbox tmpfs root %s", base, sandboxTmpfsRoot)
	}
	if err := os.MkdirAll(sourcePath, 0o755); err != nil {
		return "", fmt.Errorf("mkdir tmpfs source path %s: %w", sourcePath, err)
	}
	if err := os.MkdirAll(exportPath, 0o755); err != nil {
		return "", fmt.Errorf("mkdir tmpfs export path %s: %w", exportPath, err)
	}

	resolvedSource := resolvePath(sourcePath)
	resolvedExport := resolvePath(exportPath)
	if isMountPoint(resolvedExport) {
		return resolvedExport, nil
	}

	options := []string{fmt.Sprintf("mode=%s", spec.Mode)}
	if spec.SizeBytes > 0 {
		options = append(options, fmt.Sprintf("size=%d", spec.SizeBytes))
	}
	if spec.NoSuid {
		options = append(options, "nosuid")
	}
	if spec.NoDev {
		options = append(options, "nodev")
	}
	if spec.NoExec {
		options = append(options, "noexec")
	}

	opt := strings.Join(options, ",")
	if !isMountPoint(resolvedSource) {
		cmd := exec.Command("mount", "-t", "tmpfs", "-o", opt, "tmpfs", resolvedSource)
		out, err := cmd.CombinedOutput()
		if err != nil {
			return "", fmt.Errorf("mount tmpfs %s (%s): %w: %s", resolvedSource, opt, err, strings.TrimSpace(string(out)))
		}
	}

	if err := bindRemountPath(resolvedSource, resolvedExport, spec.ReadOnly, spec.NoSuid, spec.NoDev, spec.NoExec); err != nil {
		return "", err
	}

	return resolvedExport, nil
}

func bindRemountPath(sourcePath, targetPath string, readOnly, noSuid, noDev, noExec bool) error {
	if !isMountPoint(targetPath) {
		cmd := exec.Command("mount", "--bind", sourcePath, targetPath)
		out, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("bind mount %s -> %s: %w: %s", sourcePath, targetPath, err, strings.TrimSpace(string(out)))
		}
	}

	remountOpts := []string{"remount", "bind"}
	if readOnly {
		remountOpts = append(remountOpts, "ro")
	} else {
		remountOpts = append(remountOpts, "rw")
	}
	if noSuid {
		remountOpts = append(remountOpts, "nosuid")
	}
	if noDev {
		remountOpts = append(remountOpts, "nodev")
	}
	if noExec {
		remountOpts = append(remountOpts, "noexec")
	}

	cmd := exec.Command("mount", "-o", strings.Join(remountOpts, ","), targetPath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("remount bind %s (%s): %w: %s", targetPath, strings.Join(remountOpts, ","), err, strings.TrimSpace(string(out)))
	}

	return nil
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

	if bytes, err := parseMemoryBytes(v); err == nil && bytes > 0 {
		return bytes, nil
	}

	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("invalid size %q", raw)
	}
	return n, nil
}
