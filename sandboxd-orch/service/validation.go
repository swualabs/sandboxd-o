package service

import (
	"errors"
	"fmt"
	"net"
	"strings"
)

var ErrInvalidInput = errors.New("invalid input")

func validateNodeInput(id, ip string, port int) error {
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("%w: id is required", ErrInvalidInput)
	}

	parsed := net.ParseIP(strings.TrimSpace(ip))
	if parsed == nil {
		return fmt.Errorf("%w: invalid ip", ErrInvalidInput)
	}

	// Defense-in-depth against orchestrator-side SSRF (issue #23): the node IP is
	// used verbatim as the upstream for orchestrator-originated requests, so reject
	// address classes that should never identify a real sbxlet node. Notably this
	// blocks the cloud metadata endpoint (169.254.169.254), which is link-local.
	if err := rejectUnsafeNodeIP(parsed); err != nil {
		return err
	}

	if port < 1 || port > 65535 {
		return fmt.Errorf("%w: invalid port", ErrInvalidInput)
	}

	return nil
}

func rejectUnsafeNodeIP(ip net.IP) error {
	switch {
	case ip.IsUnspecified():
		return fmt.Errorf("%w: unspecified ip is not allowed", ErrInvalidInput)
	case ip.IsMulticast(), ip.IsInterfaceLocalMulticast(), ip.IsLinkLocalMulticast():
		return fmt.Errorf("%w: multicast ip is not allowed", ErrInvalidInput)
	case ip.IsLinkLocalUnicast():
		return fmt.Errorf("%w: link-local ip is not allowed", ErrInvalidInput)
	}

	return nil
}
