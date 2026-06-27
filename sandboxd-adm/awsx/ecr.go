package awsx

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

var ecrRepoPatternRE = regexp.MustCompile(`^[a-z0-9._/*-]+$`)

func ParseECRRepoPatterns(raw string) ([]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}

	seen := make(map[string]bool)
	var out []string
	for p := range strings.SplitSeq(raw, ",") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}

		if !ecrRepoPatternRE.MatchString(p) {
			return nil, fmt.Errorf("invalid ECR repository pattern %q (allowed: lowercase letters, digits, '.', '_', '/', '-', '*')", p)
		}

		if seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}

	return out, nil
}

func MergeECRRepoPatterns(existing, additional []string) []string {
	seen := make(map[string]bool, len(existing)+len(additional))
	var out []string
	for _, p := range existing {
		if p != "" && !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}

	for _, p := range additional {
		if p != "" && !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out
}
