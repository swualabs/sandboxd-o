package sandbox

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"example.com/sandbox-demo/internal/model"
	"golang.org/x/sys/unix"
)

func withDefaultLimits(in model.ResourceLimits) model.ResourceLimits {
	out := in
	if out.MemoryBytes <= 0 {
		out.MemoryBytes = 128 * 1024 * 1024
	}

	if out.CPUQuota <= 0 {
		out.CPUQuota = 50000
	}

	if out.CPUPeriod == 0 {
		out.CPUPeriod = 100000
	}

	if out.PidsLimit <= 0 {
		out.PidsLimit = 128
	}

	return out
}

func (s *Service) acquireSandboxLock(sandboxID string) (func(), error) {
	if sandboxID == "" {
		return func() {}, fmt.Errorf("sandbox id is required")
	}

	lockPath := filepath.Join(s.lockDir, sandboxID+".lock")
	fd, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return func() {}, err
	}

	deadline := time.Now().Add(DefaultLockWaitTimeout)
	for {
		if err := unix.Flock(int(fd.Fd()), unix.LOCK_EX|unix.LOCK_NB); err == nil {
			return func() {
				_ = unix.Flock(int(fd.Fd()), unix.LOCK_UN)
				_ = fd.Close()
			}, nil
		}

		if time.Now().After(deadline) {
			_ = fd.Close()
			return func() {}, fmt.Errorf("lock timeout for %s", sandboxID)
		}

		time.Sleep(100 * time.Millisecond)
	}
}

func (s *Service) isSandboxLockHeld(sandboxID string) bool {
	if sandboxID == "" {
		return false
	}

	lockPath := filepath.Join(s.lockDir, sandboxID+".lock")
	fd, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return false
	}
	defer fd.Close()

	if err := unix.Flock(int(fd.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		return true
	}

	_ = unix.Flock(int(fd.Fd()), unix.LOCK_UN)
	return false
}
