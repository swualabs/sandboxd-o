package sandbox

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"sandboxd-o/sandboxd-let/config"
	"sandboxd-o/sandboxd-let/model"
	"sandboxd-o/sandboxd-let/store"
)

func testLogService(t *testing.T) (*Service, string) {
	t.Helper()

	dir := t.TempDir()
	fs, err := store.NewFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}

	s := &Service{
		store: fs,
		cfg:   config.Config{StateBaseDir: dir},
	}

	return s, dir
}

func withLogLimits(t *testing.T, fileBytes, sandboxBytes int64) {
	t.Helper()

	oldFileBytes := maxLogFileBytes
	oldSandboxBytes := maxSandboxLogBytes
	maxLogFileBytes = fileBytes
	maxSandboxLogBytes = sandboxBytes
	t.Cleanup(func() {
		maxLogFileBytes = oldFileBytes
		maxSandboxLogBytes = oldSandboxBytes
	})
}

func TestGetContainerLogsReturnsFullFile(t *testing.T) {
	s, dir := testLogService(t)

	logDir := filepath.Join(dir, "s1", "logs")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(logDir, "app.log"), []byte("line1\nline2\r\npartial"), 0o644); err != nil {
		t.Fatal(err)
	}

	logs, err := s.GetContainerLogs(context.Background(), "s1", "app")
	if err != nil {
		t.Fatal(err)
	}

	want := []string{"line1", "line2", "partial"}
	if len(logs.Lines) != len(want) {
		t.Fatalf("lines=%v want=%v", logs.Lines, want)
	}
	for i := range want {
		if logs.Lines[i] != want[i] {
			t.Fatalf("lines=%v want=%v", logs.Lines, want)
		}
	}
}

func TestGetContainerLogsRejectsInvalidContainerName(t *testing.T) {
	s, _ := testLogService(t)
	if _, err := s.GetContainerLogs(context.Background(), "s1", "../app"); err == nil {
		t.Fatal("expected invalid container name error")
	}
}

func TestGetContainerLogsRejectsLargeFile(t *testing.T) {
	withLogLimits(t, 8, 1024)
	s, dir := testLogService(t)

	logDir := filepath.Join(dir, "s1", "logs")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(logDir, "app.log"), []byte("012345678\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := s.GetContainerLogs(context.Background(), "s1", "app")
	if !errors.Is(err, ErrLogTooLarge) {
		t.Fatalf("err=%v want ErrLogTooLarge", err)
	}
}

func TestGetSandboxLogsPrefixesAcrossContainers(t *testing.T) {
	s, dir := testLogService(t)

	sb := &model.Sandbox{
		ID: "s1",
		Containers: map[string]model.ContainerState{
			"app": {Name: "app"},
			"db":  {Name: "db"},
		},
	}
	if err := s.store.Save(sb); err != nil {
		t.Fatal(err)
	}

	logDir := filepath.Join(dir, "s1", "logs")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(logDir, "app.log"), []byte("a1\na2\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(logDir, "db.log"), []byte("d1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	logs, err := s.GetSandboxLogs(context.Background(), "s1")
	if err != nil {
		t.Fatal(err)
	}

	want := []string{"[app] a1", "[app] a2", "[db] d1"}
	if len(logs.Lines) != len(want) {
		t.Fatalf("lines=%v want=%v", logs.Lines, want)
	}

	for i := range want {
		if logs.Lines[i] != want[i] {
			t.Fatalf("lines=%v want=%v", logs.Lines, want)
		}
	}
}

func TestGetSandboxLogsRejectsLargeAggregate(t *testing.T) {
	withLogLimits(t, 1024, 16)
	s, dir := testLogService(t)

	sb := &model.Sandbox{
		ID: "s1",
		Containers: map[string]model.ContainerState{
			"app": {Name: "app"},
			"db":  {Name: "db"},
		},
	}
	if err := s.store.Save(sb); err != nil {
		t.Fatal(err)
	}

	logDir := filepath.Join(dir, "s1", "logs")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(logDir, "app.log"), []byte("12345678\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(logDir, "db.log"), []byte("abcdefghi\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := s.GetSandboxLogs(context.Background(), "s1")
	if !errors.Is(err, ErrLogTooLarge) {
		t.Fatalf("err=%v want ErrLogTooLarge", err)
	}
}

func TestGetSandboxLogsSortsCRILogsByTimestamp(t *testing.T) {
	s, dir := testLogService(t)

	sb := &model.Sandbox{
		ID: "s1",
		Containers: map[string]model.ContainerState{
			"app": {Name: "app"},
			"db":  {Name: "db"},
		},
	}
	if err := s.store.Save(sb); err != nil {
		t.Fatal(err)
	}

	logDir := filepath.Join(dir, "s1", "logs")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(logDir, "app.log"), []byte(
		"2026-06-05T10:47:08.712889483+09:00 stdout F app first\n"+
			"2026-06-05T10:47:13.28096791+09:00 stderr F app second\n",
	), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(logDir, "db.log"), []byte(
		"2026-06-05T10:47:09.179057567+09:00 stdout F db between\n",
	), 0o644); err != nil {
		t.Fatal(err)
	}

	logs, err := s.GetSandboxLogs(context.Background(), "s1")
	if err != nil {
		t.Fatal(err)
	}

	want := []string{
		"[app] 2026-06-05T10:47:08.712889483+09:00 stdout F app first",
		"[db] 2026-06-05T10:47:09.179057567+09:00 stdout F db between",
		"[app] 2026-06-05T10:47:13.28096791+09:00 stderr F app second",
	}
	if len(logs.Lines) != len(want) {
		t.Fatalf("lines=%v want=%v", logs.Lines, want)
	}

	for i := range want {
		if logs.Lines[i] != want[i] {
			t.Fatalf("lines=%v want=%v", logs.Lines, want)
		}
	}
}

func TestGetSandboxLogsSortsInterleavedCRIHeartbeatLogs(t *testing.T) {
	s, dir := testLogService(t)

	sb := &model.Sandbox{
		ID: "s1",
		Containers: map[string]model.ContainerState{
			"app-a": {Name: "app-a"},
			"app-b": {Name: "app-b"},
		},
	}
	if err := s.store.Save(sb); err != nil {
		t.Fatal(err)
	}

	logDir := filepath.Join(dir, "s1", "logs")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(logDir, "app-a.log"), []byte(
		"2026-06-05T10:56:32.096769188+09:00 stdout F Hello from app-a\n"+
			"2026-06-05T10:56:33.115683523+09:00 stdout F Hello from app-a\n"+
			"2026-06-05T10:56:34.156754114+09:00 stdout F Hello from app-a\n",
	), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(logDir, "app-b.log"), []byte(
		"2026-06-05T10:56:32.44259615+09:00 stdout F Hello from app-b\n"+
			"2026-06-05T10:56:33.488941699+09:00 stdout F Hello from app-b\n",
	), 0o644); err != nil {
		t.Fatal(err)
	}

	logs, err := s.GetSandboxLogs(context.Background(), "s1")
	if err != nil {
		t.Fatal(err)
	}

	want := []string{
		"[app-a] 2026-06-05T10:56:32.096769188+09:00 stdout F Hello from app-a",
		"[app-b] 2026-06-05T10:56:32.44259615+09:00 stdout F Hello from app-b",
		"[app-a] 2026-06-05T10:56:33.115683523+09:00 stdout F Hello from app-a",
		"[app-b] 2026-06-05T10:56:33.488941699+09:00 stdout F Hello from app-b",
		"[app-a] 2026-06-05T10:56:34.156754114+09:00 stdout F Hello from app-a",
	}
	if len(logs.Lines) != len(want) {
		t.Fatalf("lines=%v want=%v", logs.Lines, want)
	}

	for i := range want {
		if logs.Lines[i] != want[i] {
			t.Fatalf("lines=%v want=%v", logs.Lines, want)
		}
	}
}

func TestGetSandboxLogsSkipsMissingContainerLogs(t *testing.T) {
	s, dir := testLogService(t)

	sb := &model.Sandbox{
		ID: "s1",
		Containers: map[string]model.ContainerState{
			"app": {Name: "app"},
			"db":  {Name: "db"},
		},
	}
	if err := s.store.Save(sb); err != nil {
		t.Fatal(err)
	}

	logDir := filepath.Join(dir, "s1", "logs")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(logDir, "db.log"), []byte("ready\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	page, err := s.GetSandboxLogs(context.Background(), "s1")
	if err != nil {
		t.Fatal(err)
	}

	if len(page.Lines) != 1 || page.Lines[0] != "[db] ready" {
		t.Fatalf("unexpected logs: %+v", page)
	}
}
