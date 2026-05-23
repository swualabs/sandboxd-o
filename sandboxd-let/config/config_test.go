package config

import (
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	t.Setenv("SANDBOX_CONTAINERD_ADDRESS", "/tmp/containerd.sock")
	t.Setenv("SANDBOX_MAX_ALLOC_PERCENT", "80")
	cfg := DefaultConfig()
	if cfg.ContainerdAddress != "/tmp/containerd.sock" {
		t.Fatalf("addr=%q", cfg.ContainerdAddress)
	}

	if cfg.MaxAllocPercent != 80 {
		t.Fatalf("max alloc=%d", cfg.MaxAllocPercent)
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
