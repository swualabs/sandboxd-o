package awsx

import (
	"fmt"
	"strconv"
	"strings"
)

// ParseSizeGiB accepts Kubernetes-style quantities ("16Gi", "64Gi", "512Mi")
// or a bare number (assumed GiB).
func ParseSizeGiB(raw string) (int32, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return 0, fmt.Errorf("empty size")
	}

	upper := strings.ToUpper(s)
	switch {
	case strings.HasSuffix(upper, "GI"):
		return parseIntPrefix(s[:len(s)-2])
	case strings.HasSuffix(upper, "G"):
		return parseIntPrefix(s[:len(s)-1])
	case strings.HasSuffix(upper, "MI"):
		v, err := parseIntPrefix(s[:len(s)-2])
		if err != nil {
			return 0, err
		}

		gib := max(v/1024, 1)

		return gib, nil
	default:
		return parseIntPrefix(s)
	}
}

func parseIntPrefix(s string) (int32, error) {
	s = strings.TrimSpace(s)
	n, err := strconv.ParseInt(s, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("invalid size value %q: %w", s, err)
	}

	if n <= 0 {
		return 0, fmt.Errorf("size must be positive, got %q", s)
	}

	return int32(n), nil
}
