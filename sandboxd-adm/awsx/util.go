package awsx

import (
	"encoding/base64"
	"fmt"
	"strings"
)

func base64Encode(s string) string {
	return base64.StdEncoding.EncodeToString([]byte(s))
}

// AllocationIDFromARNOrID accepts either a raw EIP allocation id
// (eipalloc-xxxx) or a full EIP ARN
// (arn:aws:ec2:<region>:<account>:elastic-ip/eipalloc-xxxx) and returns the
// bare allocation id.
func AllocationIDFromARNOrID(v string) (string, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return "", fmt.Errorf("empty EIP ARN/allocation id")
	}

	if !strings.HasPrefix(v, "arn:") {
		if !strings.HasPrefix(v, "eipalloc-") {
			return "", fmt.Errorf("invalid EIP allocation id %q (expected eipalloc-xxxx or an EIP ARN)", v)
		}

		return v, nil
	}

	parts := strings.Split(v, "/")
	if len(parts) != 2 || !strings.HasPrefix(parts[1], "eipalloc-") {
		return "", fmt.Errorf("invalid EIP ARN %q (expected arn:aws:ec2:<region>:<account>:elastic-ip/eipalloc-xxxx)", v)
	}

	return parts[1], nil
}
