package sandbox

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"sandboxd-o/sandboxd-let/model"
)

type Logs struct {
	Lines []string `json:"lines"`
}

type logEntry struct {
	line      string
	ts        time.Time
	hasTime   bool
	container int
	seq       int
}

func validatePathToken(v string) error {
	if v == "" {
		return fmt.Errorf("empty path token")
	}

	if strings.Contains(v, "/") || strings.Contains(v, "\\") || strings.Contains(v, "..") {
		return fmt.Errorf("invalid path token")
	}

	return nil
}

func (s *Service) containerLogPath(sandboxID, containerName string) (string, error) {
	if err := model.ValidateSandboxID(sandboxID); err != nil {
		return "", fmt.Errorf("invalid sandbox id: %w", err)
	}

	if err := validatePathToken(containerName); err != nil {
		return "", fmt.Errorf("invalid container name")
	}

	logDir := filepath.Join(s.cfg.StateBaseDir, sandboxID, "logs")
	path := filepath.Clean(filepath.Join(logDir, containerName+".log"))
	logDirClean := filepath.Clean(logDir)
	if path != logDirClean && !strings.HasPrefix(path, logDirClean+string(filepath.Separator)) {
		return "", fmt.Errorf("invalid log path")
	}

	return path, nil
}

func (s *Service) GetContainerLogs(_ context.Context, sandboxID, containerName string) (*Logs, error) {
	if sandboxID == "" || containerName == "" {
		return nil, fmt.Errorf("sandbox id and container name are required")
	}

	path, err := s.containerLogPath(sandboxID, containerName)
	if err != nil {
		return nil, err
	}

	lines, err := readLogLines(path, "")
	if err != nil {
		return nil, err
	}

	return &Logs{Lines: lines}, nil
}

func (s *Service) GetSandboxLogs(_ context.Context, sandboxID string) (*Logs, error) {
	if sandboxID == "" {
		return nil, fmt.Errorf("sandbox id is required")
	}

	if err := model.ValidateSandboxID(sandboxID); err != nil {
		return nil, fmt.Errorf("invalid sandbox id: %w", err)
	}

	sbx, err := s.store.Load(sandboxID)
	if err != nil {
		return nil, err
	}

	names := make([]string, 0, len(sbx.Containers))
	for name := range sbx.Containers {
		if err := validatePathToken(name); err != nil {
			return nil, fmt.Errorf("invalid container name")
		}
		names = append(names, name)
	}
	sort.Strings(names)

	entries := []logEntry{}
	seq := 0
	for containerIndex, name := range names {
		path, err := s.containerLogPath(sandboxID, name)
		if err != nil {
			return nil, err
		}

		containerEntries, err := readLogEntries(path, "["+name+"] ", containerIndex, &seq)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}

			return nil, err
		}

		entries = append(entries, containerEntries...)
	}

	sort.SliceStable(entries, func(i, j int) bool {
		left, right := entries[i], entries[j]
		if left.hasTime && right.hasTime {
			if left.ts.Equal(right.ts) {
				return left.seq < right.seq
			}

			return left.ts.Before(right.ts)
		}

		if left.hasTime != right.hasTime {
			return left.hasTime
		}

		if left.container == right.container {
			return left.seq < right.seq
		}

		return left.container < right.container
	})

	lines := make([]string, 0, len(entries))
	for _, entry := range entries {
		lines = append(lines, entry.line)
	}

	return &Logs{Lines: lines}, nil
}

func readLogLines(path, prefix string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	r := bufio.NewReader(f)
	lines := []string{}
	for {
		line, err := r.ReadString('\n')
		if len(line) > 0 {
			lines = append(lines, prefix+strings.TrimRight(line, "\r\n"))
		}

		if err == io.EOF {
			return lines, nil
		}

		if err != nil {
			return nil, err
		}
	}
}

func readLogEntries(path, prefix string, containerIndex int, seq *int) ([]logEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	r := bufio.NewReader(f)
	entries := []logEntry{}
	for {
		line, err := r.ReadString('\n')
		if len(line) > 0 {
			line = strings.TrimRight(line, "\r\n")
			ts, ok := parseCRILogTimestamp(line)
			entries = append(entries, logEntry{
				line:      prefix + line,
				ts:        ts,
				hasTime:   ok,
				container: containerIndex,
				seq:       *seq,
			})
			*seq = *seq + 1
		}

		if err == io.EOF {
			return entries, nil
		}

		if err != nil {
			return nil, err
		}
	}
}

func parseCRILogTimestamp(line string) (time.Time, bool) {
	tsRaw, _, ok := strings.Cut(line, " ")
	if !ok || tsRaw == "" {
		return time.Time{}, false
	}

	ts, err := time.Parse(time.RFC3339Nano, tsRaw)
	if err != nil {
		return time.Time{}, false
	}

	return ts, true
}
