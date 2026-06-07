package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.ContainerdAddress != DefaultContainerdAddr {
		t.Fatalf("addr=%q", cfg.ContainerdAddress)
	}

	if cfg.MaxAllocPercent != 90 {
		t.Fatalf("max alloc=%d", cfg.MaxAllocPercent)
	}
}

func TestLoad_FileAndEnvOverride(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sbxlet.json")
	raw := `{"http_addr":":18080","containerd_address":"/tmp/from-file.sock","forward_hook_chains":["FORWARD"],"max_alloc_percent":70}`
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("SANDBOX_CONTAINERD_ADDRESS", "/tmp/from-env.sock")
	t.Setenv("SANDBOX_MAX_ALLOC_PERCENT", "80")
	t.Setenv("SANDBOX_FORWARD_HOOK_CHAINS", "DOCKER-USER,FORWARD")
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}

	if cfg.HTTPAddr != ":18080" {
		t.Fatalf("http addr=%q", cfg.HTTPAddr)
	}

	if cfg.ContainerdAddress != "/tmp/from-env.sock" {
		t.Fatalf("addr=%q", cfg.ContainerdAddress)
	}

	if cfg.MaxAllocPercent != 80 {
		t.Fatalf("max alloc=%d", cfg.MaxAllocPercent)
	}

	if len(cfg.ForwardHookChains) != 2 || cfg.ForwardHookChains[0] != "DOCKER-USER" {
		t.Fatalf("forward chains=%v", cfg.ForwardHookChains)
	}
}

func TestWithConfigDefaults(t *testing.T) {
	cfg := WithConfigDefaults(Config{})
	if cfg.ContainerdAddress == "" || cfg.CNIConfPath == "" {
		t.Fatal("expected defaults")
	}

	cfg = WithConfigDefaults(Config{MaxAllocPercent: 200, ProvisionTimeout: -1 * time.Second})
	if cfg.MaxAllocPercent != 90 || cfg.ProvisionTimeout != DefaultProvisionTimeout {
		t.Fatalf("unexpected normalized config: %+v", cfg)
	}
}

func TestWithConfigDefaults_EphemeralSplitNormalization(t *testing.T) {
	cfg := WithConfigDefaults(Config{
		DefaultEphemeralBytes: -1,
		RootfsRatioPercent:    70,
		TmpfsRatioPercent:     10,
	})

	if cfg.DefaultEphemeralBytes <= 0 {
		t.Fatalf("default ephemeral bytes not normalized: %d", cfg.DefaultEphemeralBytes)
	}

	if cfg.RootfsRatioPercent != 80 || cfg.TmpfsRatioPercent != 20 {
		t.Fatalf("unexpected split: root=%d tmp=%d", cfg.RootfsRatioPercent, cfg.TmpfsRatioPercent)
	}
}
