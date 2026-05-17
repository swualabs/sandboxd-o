package manifest

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

type SandboxManifest struct {
	APIVersion string         `yaml:"apiVersion"`
	Kind       string         `yaml:"kind"`
	ID         string         `yaml:"id"`
	Spec       map[string]any `yaml:"spec"`
}

func ParseSandboxManifest(raw []byte) (map[string]any, error) {
	var m SandboxManifest
	if err := yaml.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("parse yaml: %w", err)
	}

	if strings.TrimSpace(m.Kind) == "" {
		return nil, fmt.Errorf("kind is required")
	}

	if !strings.EqualFold(strings.TrimSpace(m.Kind), "Sandbox") {
		return nil, fmt.Errorf("unsupported kind %q (expected Sandbox)", m.Kind)
	}

	if strings.TrimSpace(m.ID) == "" {
		return nil, fmt.Errorf("id is required")
	}

	if m.Spec == nil {
		return nil, fmt.Errorf("spec is required")
	}

	if _, ok := m.Spec["containers"]; !ok {
		return nil, fmt.Errorf("spec.containers is required")
	}

	payload := map[string]any{
		"id":   strings.TrimSpace(m.ID),
		"spec": m.Spec,
	}

	return payload, nil
}
