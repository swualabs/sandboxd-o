package service

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"strings"
	"time"

	"sandboxd-o/orchestrator/client"
	"sandboxd-o/orchestrator/config"
	"sandboxd-o/orchestrator/repo"
	"sandboxd-o/orchestrator/types"
)

type Service struct {
	cfg  config.Config
	repo repo.NodeRepo
}

func New(cfg config.Config) (*Service, error) {
	st, err := repo.NewSQLite(cfg.SQLitePath)
	if err != nil {
		return nil, err
	}

	s := &Service{cfg: cfg, repo: st}
	return s, nil
}

func (s *Service) Close() error                   { return s.repo.Close() }
func (s *Service) HTTPAddr() string               { return s.cfg.HTTPAddr }
func (s *Service) ShutdownTimeout() time.Duration { return s.cfg.ShutdownTimeout }

func (s *Service) BootstrapNodes(ctx context.Context) error {
	seen := make(map[string]int)
	for _, n := range s.cfg.Bootstrap.Nodes {
		if err := validateNodeInput(n.Name, n.IP, n.Port); err != nil {
			slog.Warn("skip invalid bootstrap node",
				slog.String("name", n.Name),
				slog.String("ip", n.IP),
				slog.Int("port", n.Port),
				slog.Any("error", err),
			)
			continue
		}

		seen[n.Name]++
		if seen[n.Name] > 1 {
			slog.Warn("duplicate bootstrap node name detected; last entry wins",
				slog.String("name", n.Name),
				slog.String("ip", n.IP),
				slog.Int("port", n.Port),
			)
		}

		if err := s.repo.UpsertNode(ctx, n.Name, n.IP, n.Port, "config"); err != nil {
			return err
		}
	}

	return nil
}

func (s *Service) StartHeartbeatLoop(ctx context.Context) {
	go func() {
		t := time.NewTicker(s.cfg.HeartbeatInterval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				s.runHeartbeatOnce(ctx)
			}
		}
	}()
}

func (s *Service) runHeartbeatOnce(ctx context.Context) {
	nodes, err := s.repo.ListNodes(ctx)
	if err != nil {
		slog.Error("list nodes failed", slog.Any("error", err))
		return
	}

	if !s.cfg.HeartbeatParallel || len(nodes) <= 1 {
		for _, n := range nodes {
			s.probeNode(ctx, n)
		}
		return
	}

	parallel := s.cfg.HeartbeatMaxParallel
	if parallel > len(nodes) {
		parallel = len(nodes)
	}

	sem := make(chan struct{}, parallel)
	var wg sync.WaitGroup
	for _, n := range nodes {
		wg.Add(1)
		sem <- struct{}{}
		go func(node types.Node) {
			defer wg.Done()
			defer func() { <-sem }()
			s.probeNode(ctx, node)
		}(n)
	}
	wg.Wait()
}

func (s *Service) probeNode(ctx context.Context, n types.Node) {
	probeCtx, cancel := context.WithTimeout(ctx, s.cfg.ProbeTimeout)
	defer cancel()

	c := client.New(n.SandboxdBaseURL, s.cfg.ProbeTimeout)
	err := c.Healthz(probeCtx)

	now := time.Now().UTC()
	next := n
	next.LastHeartbeat = &now
	if err == nil {
		next.SuccessStreak++
		if next.SuccessStreak > s.cfg.ReadySuccessThreshold {
			next.SuccessStreak = s.cfg.ReadySuccessThreshold
		}
		next.FailureStreak = 0
		next.LastError = ""
		if next.State != types.NodeStateReady && next.SuccessStreak >= s.cfg.ReadySuccessThreshold {
			next.State = types.NodeStateReady
		}
	} else {
		next.FailureStreak++
		next.SuccessStreak = 0
		next.LastError = err.Error()
		if next.FailureStreak >= s.cfg.NotReadyFailureThreshold {
			next.State = types.NodeStateNotReady
		}
	}

	if next.State == "" {
		next.State = types.NodeStateUnknown
	}

	_ = s.repo.UpdateHeartbeat(ctx, n.Name, next.State, next.SuccessStreak, next.FailureStreak, next.LastError, next.LastHeartbeat)
}

func (s *Service) RegisterNode(ctx context.Context, req types.RegisterNodeRequest, source string) (*types.Node, error) {
	if err := validateNodeInput(req.Name, req.IP, req.Port); err != nil {
		return nil, err
	}

	if source == "" {
		source = "api"
	}

	if err := s.repo.UpsertNode(ctx, req.Name, req.IP, req.Port, source); err != nil {
		return nil, err
	}

	n, err := s.repo.GetNode(ctx, req.Name)
	if err != nil {
		return nil, err
	}

	s.probeNode(ctx, *n)
	return s.repo.GetNode(ctx, req.Name)
}

func (s *Service) DeleteNode(ctx context.Context, name string) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("name is required")
	}

	return s.repo.DeleteNode(ctx, name)
}

func (s *Service) ListNodes(ctx context.Context) ([]types.Node, error) {
	return s.repo.ListNodes(ctx)
}

func (s *Service) GetNode(ctx context.Context, name string) (*types.Node, error) {
	return s.repo.GetNode(ctx, name)
}

func (s *Service) SandboxClientForNode(ctx context.Context, name string) (*client.Client, *types.Node, error) {
	n, err := s.repo.GetNode(ctx, name)
	if err != nil {
		return nil, nil, err
	}

	client := client.New(n.SandboxdBaseURL, s.cfg.ProbeTimeout)
	return client, n, nil
}

func validateNodeInput(name, ip string, port int) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("name is required")
	}

	if net.ParseIP(strings.TrimSpace(ip)) == nil {
		return fmt.Errorf("invalid ip")
	}

	if port < 1 || port > 65535 {
		return fmt.Errorf("invalid port")
	}

	return nil
}
