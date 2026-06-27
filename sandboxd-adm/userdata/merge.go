package userdata

import (
	"encoding/json"
	"fmt"
	"os"
)

// Layering order: shipped default -> dynamicDefaults -> overridePath ->
// sharedSecret (always wins last).
func MergeConfig(component, overridePath, sharedSecret string, dynamicDefaults map[string]any) (string, error) {
	def, ok := defaultConfigs[component]
	if !ok {
		return "", fmt.Errorf("no default config for component %q", component)
	}

	var base map[string]any
	if err := json.Unmarshal(def, &base); err != nil {
		return "", fmt.Errorf("parse default %s config: %w", component, err)
	}

	for k, v := range dynamicDefaults {
		base[k] = v
	}

	if overridePath != "" {
		raw, err := os.ReadFile(overridePath)
		if err != nil {
			return "", fmt.Errorf("read override config %q: %w", overridePath, err)
		}
		var override map[string]any
		if err := json.Unmarshal(raw, &override); err != nil {
			return "", fmt.Errorf("parse override config %q: %w", overridePath, err)
		}
		for k, v := range override {
			base[k] = v
		}
	}

	if sharedSecret != "" {
		base["shared_secret"] = sharedSecret
	}

	out, err := json.MarshalIndent(base, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal merged %s config: %w", component, err)
	}

	return string(out), nil
}
