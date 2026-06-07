package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoad_FileAndEnvOverride(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sbxctl.json")
	if err := os.WriteFile(path, []byte(`{"server":"http://file:8082","timeout":"15s","output":"json","limit":55}`), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("SBXCTL_SERVER", "http://env:8082")
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Server != "http://env:8082" {
		t.Fatalf("server=%q", cfg.Server)
	}
	if cfg.Timeout != 15*time.Second {
		t.Fatalf("timeout=%s", cfg.Timeout)
	}
	if cfg.Output != "json" {
		t.Fatalf("output=%q", cfg.Output)
	}
	if cfg.Limit != 55 {
		t.Fatalf("limit=%d", cfg.Limit)
	}
}
