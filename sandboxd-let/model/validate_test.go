package model

import "testing"

func validReq() CreateSandboxRequest {
	return CreateSandboxRequest{
		ID:     "sbx-a",
		Egress: true,
		Containers: []CreateContainerRequest{{
			Name:  "c1",
			Image: "nginx",
			Resource: ResourceSpec{
				CPU:    "100m",
				Memory: "128Mi",
			},
		}},
		Ports: []PortMapping{{HostPort: 30080, ContainerPort: 80, Protocol: "tcp"}},
	}
}

func TestValidate_OK(t *testing.T) {
	if err := validReq().Validate(); err != nil {
		t.Fatalf("Validate err=%v", err)
	}
}

func TestValidate_DuplicateContainer(t *testing.T) {
	r := validReq()
	r.Containers = append(r.Containers, r.Containers[0])
	if err := r.Validate(); err == nil {
		t.Fatal("expected error")
	}
}

func TestValidate_InvalidSandboxID(t *testing.T) {
	cases := []string{
		"../../tmp/hello",
		"sbx/1",
		" sbx-1",
		"sbx-1 ",
		"",
	}

	for _, id := range cases {
		r := validReq()
		r.ID = id
		if err := r.Validate(); err == nil {
			t.Fatalf("expected invalid id error for %q", id)
		}
	}
}

func TestValidateContainerName(t *testing.T) {
	valid := []string{"c1", "app", "web-server", "a", "Cont_ainer.1"}
	for _, name := range valid {
		if err := ValidateContainerName(name); err != nil {
			t.Fatalf("expected %q to be valid, got err=%v", name, err)
		}
	}

	invalid := []string{
		"",
		" ",
		"../../tmp/escape",
		"../escape",
		"a/b",
		"a\\b",
		"with..dots",
		" leading",
		"trailing ",
		"-startsWithDash",
		".startsWithDot",
		"bad name",
		string(make([]byte, 65)), // exceeds 64 chars
	}
	for _, name := range invalid {
		if err := ValidateContainerName(name); err == nil {
			t.Fatalf("expected %q to be rejected", name)
		}
	}
}

func TestValidate_ContainerNameTraversal(t *testing.T) {
	cases := []string{
		"../../../../tmp/escape",
		"../escape",
		"a/b",
		"a\\b",
		"with..dots",
		" leading",
		"trailing ",
	}

	for _, name := range cases {
		r := validReq()
		r.Containers[0].Name = name
		if err := r.Validate(); err == nil {
			t.Fatalf("expected invalid container name error for %q", name)
		}
	}
}

func TestValidate_PortRules(t *testing.T) {
	r := validReq()
	r.Ports = []PortMapping{{HostPort: 80, ContainerPort: 80, Protocol: "tcp"}}
	if err := r.Validate(); err == nil {
		t.Fatal("expected privileged port error")
	}

	r = validReq()
	r.Ports = []PortMapping{{HostPort: 30080, ContainerPort: 80, Protocol: "sctp"}}
	if err := r.Validate(); err == nil {
		t.Fatal("expected protocol error")
	}
}

func TestValidate_ReadinessProbeRules(t *testing.T) {
	r := validReq()
	r.Readiness = &ReadinessProbeSpec{
		Protocol:            "http",
		Port:                8080,
		Path:                "/healthz",
		InitialDelaySeconds: 1,
		PeriodSeconds:       1,
		TimeoutSeconds:      1,
		SuccessThreshold:    1,
		FailureThreshold:    1,
	}
	if err := r.Validate(); err != nil {
		t.Fatalf("expected valid http readiness, got err=%v", err)
	}

	r = validReq()
	r.Readiness = &ReadinessProbeSpec{
		Protocol:            "http",
		Port:                8080,
		InitialDelaySeconds: 1,
		PeriodSeconds:       1,
		TimeoutSeconds:      1,
		SuccessThreshold:    1,
		FailureThreshold:    1,
	}
	if err := r.Validate(); err == nil {
		t.Fatal("expected http readiness path validation error")
	}
}

func TestValidate_ReadinessProbeInvalidCases(t *testing.T) {
	base := validReq()
	cases := []struct {
		name  string
		probe ReadinessProbeSpec
	}{
		{
			name: "unsupported protocol",
			probe: ReadinessProbeSpec{
				Protocol:            "udp",
				Port:                8080,
				InitialDelaySeconds: 1,
				PeriodSeconds:       1,
				TimeoutSeconds:      1,
				SuccessThreshold:    1,
				FailureThreshold:    1,
			},
		},
		{
			name: "invalid port",
			probe: ReadinessProbeSpec{
				Protocol:            "tcp",
				Port:                0,
				InitialDelaySeconds: 1,
				PeriodSeconds:       1,
				TimeoutSeconds:      1,
				SuccessThreshold:    1,
				FailureThreshold:    1,
			},
		},
		{
			name: "http path must start slash",
			probe: ReadinessProbeSpec{
				Protocol:            "http",
				Port:                8080,
				Path:                "healthz",
				InitialDelaySeconds: 1,
				PeriodSeconds:       1,
				TimeoutSeconds:      1,
				SuccessThreshold:    1,
				FailureThreshold:    1,
			},
		},
		{
			name: "initial delay invalid",
			probe: ReadinessProbeSpec{
				Protocol:            "tcp",
				Port:                8080,
				InitialDelaySeconds: 0,
				PeriodSeconds:       1,
				TimeoutSeconds:      1,
				SuccessThreshold:    1,
				FailureThreshold:    1,
			},
		},
		{
			name: "period invalid",
			probe: ReadinessProbeSpec{
				Protocol:            "tcp",
				Port:                8080,
				InitialDelaySeconds: 1,
				PeriodSeconds:       0,
				TimeoutSeconds:      1,
				SuccessThreshold:    1,
				FailureThreshold:    1,
			},
		},
		{
			name: "timeout invalid",
			probe: ReadinessProbeSpec{
				Protocol:            "tcp",
				Port:                8080,
				InitialDelaySeconds: 1,
				PeriodSeconds:       1,
				TimeoutSeconds:      0,
				SuccessThreshold:    1,
				FailureThreshold:    1,
			},
		},
		{
			name: "success threshold invalid",
			probe: ReadinessProbeSpec{
				Protocol:            "tcp",
				Port:                8080,
				InitialDelaySeconds: 1,
				PeriodSeconds:       1,
				TimeoutSeconds:      1,
				SuccessThreshold:    0,
				FailureThreshold:    1,
			},
		},
		{
			name: "failure threshold invalid",
			probe: ReadinessProbeSpec{
				Protocol:            "tcp",
				Port:                8080,
				InitialDelaySeconds: 1,
				PeriodSeconds:       1,
				TimeoutSeconds:      1,
				SuccessThreshold:    1,
				FailureThreshold:    0,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := base
			r.Readiness = &tc.probe
			if err := r.Validate(); err == nil {
				t.Fatalf("expected error for case %q", tc.name)
			}
		})
	}
}

func TestValidate_SharedVolumesOK(t *testing.T) {
	r := validReq()
	r.Volumes = []VolumeSpec{{
		Name:             "runtime-state",
		EphemeralStorage: "128Mi",
	}}
	r.Containers[0].VolumeMounts = []VolumeMount{{
		Name:      "runtime-state",
		MountPath: "/var/www/html",
	}}

	if err := r.Validate(); err != nil {
		t.Fatalf("expected valid shared volume config, got err=%v", err)
	}
}

func TestValidate_ContainerRuntimeOptionsOK(t *testing.T) {
	r := validReq()
	r.Containers[0].CapAdd = []string{"SYS_PTRACE"}
	r.Containers[0].CapDrop = []string{"ALL"}
	r.Containers[0].ReadOnly = true
	r.Containers[0].SecurityOpt = []string{
		"no-new-privileges:false",
		"seccomp=unconfined",
		"apparmor=runtime/default",
	}
	r.Containers[0].Tmpfs = []TmpfsMount{{
		MountPath: "/run",
		Options:   "rw,nosuid,nodev,noexec,mode=0755,size=64m",
	}}

	if err := r.Validate(); err != nil {
		t.Fatalf("expected valid runtime options, got err=%v", err)
	}
}

func TestValidate_SharedVolumeInvalidCases(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*CreateSandboxRequest)
	}{
		{
			name: "missing volume name",
			mutate: func(r *CreateSandboxRequest) {
				r.Volumes = []VolumeSpec{{EphemeralStorage: "64Mi"}}
			},
		},
		{
			name: "invalid volume name",
			mutate: func(r *CreateSandboxRequest) {
				r.Volumes = []VolumeSpec{{Name: "bad/name", EphemeralStorage: "64Mi"}}
			},
		},
		{
			name: "duplicate volume name",
			mutate: func(r *CreateSandboxRequest) {
				r.Volumes = []VolumeSpec{
					{Name: "shared", EphemeralStorage: "64Mi"},
					{Name: "shared", EphemeralStorage: "64Mi"},
				}
			},
		},
		{
			name: "missing volume size",
			mutate: func(r *CreateSandboxRequest) {
				r.Volumes = []VolumeSpec{{Name: "shared"}}
			},
		},
		{
			name: "zero volume size",
			mutate: func(r *CreateSandboxRequest) {
				r.Volumes = []VolumeSpec{{Name: "shared", EphemeralStorage: "0"}}
			},
		},
		{
			name: "unknown volume mount",
			mutate: func(r *CreateSandboxRequest) {
				r.Volumes = []VolumeSpec{{Name: "shared", EphemeralStorage: "64Mi"}}
				r.Containers[0].VolumeMounts = []VolumeMount{{Name: "missing", MountPath: "/data"}}
			},
		},
		{
			name: "missing container name",
			mutate: func(r *CreateSandboxRequest) {
				r.Containers[0].Name = ""
			},
		},
		{
			name: "missing container image",
			mutate: func(r *CreateSandboxRequest) {
				r.Containers[0].Image = ""
			},
		},
		{
			name: "missing mount volume name",
			mutate: func(r *CreateSandboxRequest) {
				r.Volumes = []VolumeSpec{{Name: "shared", EphemeralStorage: "64Mi"}}
				r.Containers[0].VolumeMounts = []VolumeMount{{MountPath: "/data"}}
			},
		},
		{
			name: "missing mount path",
			mutate: func(r *CreateSandboxRequest) {
				r.Volumes = []VolumeSpec{{Name: "shared", EphemeralStorage: "64Mi"}}
				r.Containers[0].VolumeMounts = []VolumeMount{{Name: "shared"}}
			},
		},
		{
			name: "relative mount path",
			mutate: func(r *CreateSandboxRequest) {
				r.Volumes = []VolumeSpec{{Name: "shared", EphemeralStorage: "64Mi"}}
				r.Containers[0].VolumeMounts = []VolumeMount{{Name: "shared", MountPath: "data"}}
			},
		},
		{
			name: "unclean mount path",
			mutate: func(r *CreateSandboxRequest) {
				r.Volumes = []VolumeSpec{{Name: "shared", EphemeralStorage: "64Mi"}}
				r.Containers[0].VolumeMounts = []VolumeMount{{Name: "shared", MountPath: "/data/../tmp"}}
			},
		},
		{
			name: "root mount path",
			mutate: func(r *CreateSandboxRequest) {
				r.Volumes = []VolumeSpec{{Name: "shared", EphemeralStorage: "64Mi"}}
				r.Containers[0].VolumeMounts = []VolumeMount{{Name: "shared", MountPath: "/"}}
			},
		},
		{
			name: "reserved tmp mount path",
			mutate: func(r *CreateSandboxRequest) {
				r.Volumes = []VolumeSpec{{Name: "shared", EphemeralStorage: "64Mi"}}
				r.Containers[0].VolumeMounts = []VolumeMount{{Name: "shared", MountPath: "/tmp"}}
			},
		},
		{
			name: "reserved tmp subpath mount",
			mutate: func(r *CreateSandboxRequest) {
				r.Volumes = []VolumeSpec{{Name: "shared", EphemeralStorage: "64Mi"}}
				r.Containers[0].VolumeMounts = []VolumeMount{{Name: "shared", MountPath: "/tmp/shared"}}
			},
		},
		{
			name: "duplicate mount path",
			mutate: func(r *CreateSandboxRequest) {
				r.Volumes = []VolumeSpec{
					{Name: "shared-a", EphemeralStorage: "64Mi"},
					{Name: "shared-b", EphemeralStorage: "64Mi"},
				}
				r.Containers[0].VolumeMounts = []VolumeMount{
					{Name: "shared-a", MountPath: "/data"},
					{Name: "shared-b", MountPath: "/data"},
				}
			},
		},
		{
			name: "invalid capability",
			mutate: func(r *CreateSandboxRequest) {
				r.Containers[0].CapAdd = []string{"sys_ptrace"}
			},
		},
		{
			name: "invalid security opt",
			mutate: func(r *CreateSandboxRequest) {
				r.Containers[0].SecurityOpt = []string{"label=disable"}
			},
		},
		{
			name: "invalid tmpfs path",
			mutate: func(r *CreateSandboxRequest) {
				r.Containers[0].Tmpfs = []TmpfsMount{{MountPath: "run"}}
			},
		},
		{
			name: "invalid tmpfs options",
			mutate: func(r *CreateSandboxRequest) {
				r.Containers[0].Tmpfs = []TmpfsMount{{MountPath: "/run", Options: "size=bad"}}
			},
		},
		{
			name: "tmpfs duplicate mount path with volume mount",
			mutate: func(r *CreateSandboxRequest) {
				r.Volumes = []VolumeSpec{{Name: "shared", EphemeralStorage: "64Mi"}}
				r.Containers[0].VolumeMounts = []VolumeMount{{Name: "shared", MountPath: "/run"}}
				r.Containers[0].Tmpfs = []TmpfsMount{{MountPath: "/run"}}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := validReq()
			tc.mutate(&r)
			if err := r.Validate(); err == nil {
				t.Fatalf("expected error for case %q", tc.name)
			}
		})
	}
}
