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
