package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"sandboxd-o/sandboxd-orch/config"
	"sandboxd-o/sandboxd-orch/types"
)

func newTestService(t *testing.T, baseURL string) *Service {
	t.Helper()
	cfg := config.Config{
		HTTPAddr:                 ":0",
		SQLitePath:               filepath.Join(t.TempDir(), "orch.db"),
		ProbeTimeout:             time.Second,
		ReadySuccessThreshold:    2,
		NotReadyFailureThreshold: 2,
		HeartbeatInterval:        50 * time.Millisecond,
		ResourceSyncInterval:     50 * time.Millisecond,
		ResourcePersistMinInt:    10 * time.Millisecond,
		ResourcePersistMaxInt:    30 * time.Millisecond,
		ShutdownTimeout:          time.Second,
	}
	s, err := New(cfg)
	if err != nil {
		t.Fatalf("New err=%v", err)
	}

	if baseURL != "" {
		_ = s.repo.UpsertNode(context.Background(), "n1", baseURL[7:len(baseURL)-5], 18080, "api")
	}

	return s
}

func TestService_RegisterListDelete(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/healthz":
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
		case "/v1/node/status":
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "resources": map[string]any{"capacity_cpu_milli": 4000, "allocatable_cpu_milli": 3600}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	s := newTestService(t, "")
	defer s.Close()

	req := types.RegisterNodeRequest{ID: "n1", IP: "127.0.0.1", Port: 18080}
	n, err := s.RegisterNode(context.Background(), req, "api")
	if err != nil {
		t.Fatalf("RegisterNode err=%v", err)
	}

	if n.ID != "n1" {
		t.Fatalf("id=%s", n.ID)
	}

	list, err := s.ListNodes(context.Background())
	if err != nil || len(list) != 1 {
		t.Fatalf("ListNodes err=%v len=%d", err, len(list))
	}

	if err := s.DeleteNodeForce(context.Background(), "n1", true); err != nil {
		t.Fatalf("DeleteNode err=%v", err)
	}
}

func TestValidateNodeInput(t *testing.T) {
	if err := validateNodeInput("n", "127.0.0.1", 8080); err != nil {
		t.Fatalf("valid input err=%v", err)
	}

	if err := validateNodeInput("", "127.0.0.1", 8080); err == nil {
		t.Fatal("expected error")
	}
}

func TestValidateNodeInput_RejectsUnsafeAndInvalid(t *testing.T) {
	rejected := []struct {
		name string
		ip   string
		port int
	}{
		{name: "malformed ip", ip: "not-an-ip", port: 8080},
		{name: "unspecified ipv4", ip: "0.0.0.0", port: 8080},
		{name: "unspecified ipv6", ip: "::", port: 8080},
		{name: "cloud metadata link-local", ip: "169.254.169.254", port: 80},
		{name: "ipv6 link-local", ip: "fe80::1", port: 8080},
		{name: "multicast", ip: "224.0.0.1", port: 8080},
		{name: "port too low", ip: "10.0.0.5", port: 0},
		{name: "port too high", ip: "10.0.0.5", port: 70000},
	}

	for _, tc := range rejected {
		t.Run(tc.name, func(t *testing.T) {
			if err := validateNodeInput("n", tc.ip, tc.port); err == nil {
				t.Fatalf("expected rejection for %s (%s:%d)", tc.name, tc.ip, tc.port)
			}
		})
	}

	allowed := []string{"127.0.0.1", "10.89.1.10", "192.168.1.20", "198.51.100.2"}
	for _, ip := range allowed {
		if err := validateNodeInput("n", ip, 8080); err != nil {
			t.Fatalf("expected %s to be allowed, got err=%v", ip, err)
		}
	}
}
