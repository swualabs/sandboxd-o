package sandbox

import (
	"context"
	"errors"
	"os"
	"sort"
	"strings"
	"time"

	"example.com/sandbox-demo/internal/model"
	"example.com/sandbox-demo/internal/network"
	"github.com/containerd/containerd/v2/pkg/namespaces"
)

// ReconcileOnce aligns state with runtime/network.
func (s *Service) ReconcileOnce(ctx context.Context) error {
	s.dbg("reconcile start")
	ctx = namespaces.WithNamespace(ctx, s.namespace)
	stateList, err := s.store.List()
	if err != nil {
		return err
	}

	stateMap := map[string]*model.Sandbox{}
	for _, sbx := range stateList {
		stateMap[sbx.ID] = sbx
	}

	runtimeIDs, err := s.ListRuntimeSandboxIDs(ctx)
	if err != nil {
		return err
	}

	runtimeSet := map[string]struct{}{}
	for _, id := range runtimeIDs {
		runtimeSet[id] = struct{}{}
	}

	for _, sbx := range stateList {
		if s.isSandboxLockHeld(sbx.ID) {
			continue
		}

		if sbx.Phase == SandboxPhaseDeleting {
			continue
		}

		if sbx.Phase == SandboxPhaseCreating && time.Since(sbx.CreatedAt) < 2*time.Minute {
			// During initial provisioning window, avoid treating transient
			// not-yet-created runtime/container state as unhealthy.
			continue
		}

		if sbx.Phase == SandboxPhaseCreating && time.Since(sbx.CreatedAt) >= 2*time.Minute {
			// Prevent indefinite creating state if provisioning goroutine is stuck.
			setSandboxPhase(sbx, SandboxPhaseError, "provisioning timeout")
			_ = s.store.Save(sbx)
			s.dbg("reconcile timeout -> error sandbox=%s", sbx.ID)
			continue
		}

		if _, ok := runtimeSet[sbx.ID]; !ok {
			if s.shouldFinalizeUnhealthy(sbx.ID) {
				s.dbg("reconcile finalize missing runtime sandbox=%s", sbx.ID)
				_ = s.deleteSandboxFromState(ctx, sbx)
				s.clearUnhealthy(sbx.ID)
			}
			continue
		}

		healthy := true
		for _, c := range sbx.Containers {
			if !s.isContainerRunning(ctx, c.ID) {
				healthy = false
				break
			}
		}

		if !healthy {
			if s.shouldFinalizeUnhealthy(sbx.ID) {
				s.dbg("reconcile finalize unhealthy sandbox=%s", sbx.ID)
				_ = s.deleteSandboxFromState(ctx, sbx)
				s.clearUnhealthy(sbx.ID)
			}
			continue
		}
		s.clearUnhealthy(sbx.ID)
	}

	for _, runtimeID := range runtimeIDs {
		if _, ok := stateMap[runtimeID]; ok {
			continue
		}
		// Create/Delete holds an exclusive sandbox lock. If locked, skip orphan
		// cleanup this round to avoid racing in-flight lifecycle operations.
		if s.isSandboxLockHeld(runtimeID) {
			continue
		}

		s.dbg("reconcile cleanup orphan runtime sandbox=%s", runtimeID)
		_ = s.cleanupOrphanRuntimeSandbox(ctx, runtimeID)
	}

	keep := map[string]struct{}{}
	for _, sbx := range stateList {
		keep[sbx.ID] = struct{}{}
	}

	for _, id := range runtimeIDs {
		keep[id] = struct{}{}
	}

	network.DeleteOrphanHostPortDNAT(s.ipt, keep)
	s.dbg("reconcile done state=%d runtime=%d", len(stateList), len(runtimeIDs))

	return nil
}

// StartReconcileLoop runs reconcile asynchronously until ctx is canceled.
func (s *Service) StartReconcileLoop(ctx context.Context) {
	go func() {
		t := time.NewTicker(s.cfg.ReconcileInterval)
		defer t.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				_ = s.ReconcileOnce(ctx)
			}
		}
	}()
}

func (s *Service) ListRuntimeSandboxIDs(ctx context.Context) ([]string, error) {
	ctx = namespaces.WithNamespace(ctx, s.namespace)
	containers, err := s.client.Containers(ctx)
	if err != nil {
		return nil, err
	}

	ids := map[string]struct{}{}
	for _, c := range containers {
		id := c.ID()
		if !strings.HasPrefix(id, "sbx-") {
			continue
		}
		sep := strings.LastIndex(id, "-")
		if sep <= 0 {
			continue
		}
		ids[id[:sep]] = struct{}{}
	}

	out := make([]string, 0, len(ids))
	for id := range ids {
		out = append(out, id)
	}

	sort.Strings(out)
	return out, nil
}

func (s *Service) IsSandboxHealthy(ctx context.Context, sandboxID string) (bool, error) {
	ctx = namespaces.WithNamespace(ctx, s.namespace)
	sbx, err := s.store.Load(sandboxID)
	if err != nil {
		return false, err
	}

	for _, st := range sbx.Containers {
		if !s.isContainerRunning(ctx, st.ID) {
			return false, nil
		}
	}

	return true, nil
}

func (s *Service) CleanupSandboxResources(ctx context.Context, sandboxID string) error {
	ctx = namespaces.WithNamespace(ctx, s.namespace)
	sbx, err := s.store.Load(sandboxID)
	if err == nil {
		return s.deleteSandboxFromState(ctx, sbx)
	}

	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}

	return s.cleanupOrphanRuntimeSandbox(ctx, sandboxID)
}

func (s *Service) cleanupOrphanRuntimeSandbox(ctx context.Context, sandboxID string) error {
	containers, err := s.client.Containers(ctx)
	if err != nil {
		return err
	}

	tmp := &model.Sandbox{ID: sandboxID, Namespace: s.namespace, IP: s.sandboxIPFromCNICache(sandboxID), BridgeName: s.bridgeIF, Containers: map[string]model.ContainerState{}}
	for _, c := range containers {
		id := c.ID()
		if strings.HasPrefix(id, sandboxID+"-") {
			tmp.Containers[id] = model.ContainerState{ID: id}
		}
	}

	if len(tmp.Containers) == 0 && tmp.IP == "" {
		// Even without runtime/cni artifacts, DNAT rules may remain after partial failure.
		s.cleanupHostPortPublish(tmp)
		return nil
	}

	return s.deleteSandboxFromState(ctx, tmp)
}

func (s *Service) sandboxIPFromCNICache(sandboxID string) string {
	ip, err := network.LookupSandboxIPv4FromResultCache(sandboxID)
	if err != nil {
		return ""
	}

	return ip
}

func (s *Service) isContainerRunning(ctx context.Context, id string) bool {
	ctr, err := s.client.LoadContainer(ctx, id)
	if err != nil {
		return false
	}

	task, err := ctr.Task(ctx, nil)
	if err != nil {
		return false
	}

	status, err := task.Status(ctx)
	if err != nil {
		return false
	}

	return string(status.Status) == "running"
}

func (s *Service) shouldFinalizeUnhealthy(sandboxID string) bool {
	s.reconcileMu.Lock()
	defer s.reconcileMu.Unlock()

	now := time.Now()
	if _, ok := s.unhealthySince[sandboxID]; !ok {
		s.unhealthySince[sandboxID] = now
		s.unhealthyHits[sandboxID] = 1
		return false
	}

	s.unhealthyHits[sandboxID]++
	if now.Sub(s.unhealthySince[sandboxID]) < s.cfg.ReconcileGrace {
		return false
	}

	return s.unhealthyHits[sandboxID] >= s.cfg.ReconcileHits
}

func (s *Service) clearUnhealthy(sandboxID string) {
	s.reconcileMu.Lock()
	delete(s.unhealthySince, sandboxID)
	delete(s.unhealthyHits, sandboxID)
	s.reconcileMu.Unlock()
}
