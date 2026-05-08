package sandbox

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type LogsPage struct {
	Lines      []string `json:"lines"`
	NextCursor string   `json:"next_cursor,omitempty"`
	HasMore    bool     `json:"has_more"`
}

func (s *Service) containerLogPath(sandboxID, containerName string) string {
	return filepath.Join(s.cfg.StateBaseDir, sandboxID, "logs", containerName+".log")
}

func (s *Service) GetContainerLogs(_ context.Context, sandboxID, containerName, cursor string, limit int) (*LogsPage, error) {
	if sandboxID == "" || containerName == "" {
		return nil, fmt.Errorf("sandbox id and container name are required")
	}

	if limit <= 0 {
		limit = 100
	}

	if limit > 1000 {
		limit = 1000
	}

	offset := int64(0)
	if strings.TrimSpace(cursor) != "" {
		v, err := strconv.ParseInt(cursor, 10, 64)
		if err != nil || v < 0 {
			return nil, fmt.Errorf("invalid cursor")
		}
		offset = v
	}

	path := s.containerLogPath(sandboxID, containerName)
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return nil, err
	}

	r := bufio.NewReader(f)
	lines := make([]string, 0, limit)
	next := offset
	for len(lines) < limit {
		line, err := r.ReadString('\n')
		if len(line) > 0 {
			next += int64(len(line))
			lines = append(lines, strings.TrimRight(line, "\r\n"))
		}

		if err == io.EOF {
			return &LogsPage{Lines: lines, NextCursor: strconv.FormatInt(next, 10), HasMore: false}, nil
		}

		if err != nil {
			return nil, err
		}
	}

	_, err = r.Peek(1)
	hasMore := err == nil
	return &LogsPage{Lines: lines, NextCursor: strconv.FormatInt(next, 10), HasMore: hasMore}, nil
}
