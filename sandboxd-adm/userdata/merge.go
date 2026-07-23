package userdata

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
)

// Layering order:
// shipped default -> dynamicDefaults -> overridePath -> finalOverrides ->
// sharedSecret (always wins last).
func MergeConfig(component, overridePath, sharedSecret string, dynamicDefaults, finalOverrides map[string]any) (string, error) {
	def, ok := defaultConfigs[component]
	if !ok {
		return "", fmt.Errorf("no default config for component %q", component)
	}

	var base map[string]any
	if err := json.Unmarshal(def, &base); err != nil {
		return "", fmt.Errorf("parse default %s config: %w", component, err)
	}

	maps.Copy(base, dynamicDefaults)

	if overridePath != "" {
		raw, err := os.ReadFile(overridePath)
		if err != nil {
			return "", fmt.Errorf("read override config %q: %w", overridePath, err)
		}
		var override map[string]any
		if err := json.Unmarshal(raw, &override); err != nil {
			return "", fmt.Errorf("parse override config %q: %w", overridePath, err)
		}
		maps.Copy(base, override)
	}

	maps.Copy(base, finalOverrides)

	if sharedSecret != "" {
		base["shared_secret"] = sharedSecret
	}

	out, err := json.MarshalIndent(base, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal merged %s config: %w", component, err)
	}

	return string(out), nil
}
