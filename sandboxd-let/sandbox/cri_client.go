package sandbox

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	runtimeapi "k8s.io/cri-api/pkg/apis/runtime/v1"

	"golang.org/x/sync/singleflight"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// defaultPullTimeout bounds a deduplicated pull when the caller's context
// carries no deadline.
const defaultPullTimeout = 8 * time.Minute

type criClient struct {
	conn    *grpc.ClientConn
	runtime runtimeapi.RuntimeServiceClient
	image   runtimeapi.ImageServiceClient

	// pullGroup collapses concurrent pulls of the same image reference on
	// this node into a single in-flight PullImage, so launching many
	// sandboxes that share an uncached image doesn't fan out into N
	// identical registry pulls.
	pullGroup singleflight.Group
}

type criContainerDetails struct {
	ID    string
	State runtimeapi.ContainerState
	PID   uint32
}

func newCRIClient(ctx context.Context, endpoint string) (*criClient, error) {
	conn, err := grpc.NewClient(
		"unix://"+normalizeCRIEndpoint(endpoint),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, fmt.Errorf("dial cri endpoint %q: %w", endpoint, err)
	}

	return &criClient{
		conn:    conn,
		runtime: runtimeapi.NewRuntimeServiceClient(conn),
		image:   runtimeapi.NewImageServiceClient(conn),
	}, nil
}

func (c *criClient) Close() error {
	if c == nil || c.conn == nil {
		return nil
	}

	return c.conn.Close()
}

func (c *criClient) pullImage(ctx context.Context, image string) error {
	// Fast-path: if the image already exists in local CRI image store,
	// skip remote pull to avoid unnecessary registry/auth dependencies.
	if st, err := c.image.ImageStatus(ctx, &runtimeapi.ImageStatusRequest{
		Image: &runtimeapi.ImageSpec{Image: image},
	}); err == nil && st.GetImage() != nil {
		return nil
	}

	// Deduplicate concurrent pulls of the same reference: only one
	// PullImage runs, the rest wait for and share its result.
	ch := c.pullGroup.DoChan(image, func() (any, error) {
		// Detach from the triggering caller's context so that caller
		// cancelling/timing out doesn't abort the shared pull for the
		// other waiters; re-bound it with the caller's remaining deadline.
		timeout := defaultPullTimeout
		if dl, ok := ctx.Deadline(); ok {
			if remaining := time.Until(dl); remaining > 0 {
				timeout = remaining
			}
		}

		pullCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), timeout)
		defer cancel()

		// Logged once per real (deduplicated) pull, so concurrent sandboxes
		// sharing an image produce a single "pulling image" line.
		slog.Info("pulling image", slog.String("image", image))
		start := time.Now()
		_, err := c.image.PullImage(pullCtx, &runtimeapi.PullImageRequest{
			Image: &runtimeapi.ImageSpec{Image: image},
		})
		if err != nil {
			slog.Warn("image pull failed", slog.String("image", image), slog.String("error", err.Error()))
			return nil, err
		}
		slog.Info("image pulled", slog.String("image", image), slog.String("duration", time.Since(start).String()))
		return nil, nil
	})

	select {
	case <-ctx.Done():
		// This caller gives up, but the shared pull keeps running for the
		// remaining waiters.
		return ctx.Err()
	case res := <-ch:
		return res.Err
	}
}

func (c *criClient) runPodSandbox(ctx context.Context, cfg *runtimeapi.PodSandboxConfig, runtimeHandler string) (string, error) {
	resp, err := c.runtime.RunPodSandbox(ctx, &runtimeapi.RunPodSandboxRequest{
		Config:         cfg,
		RuntimeHandler: runtimeHandler,
	})
	if err != nil {
		return "", err
	}

	return resp.GetPodSandboxId(), nil
}

func (c *criClient) podSandboxStatus(ctx context.Context, podID string) (*runtimeapi.PodSandboxStatus, error) {
	resp, err := c.runtime.PodSandboxStatus(ctx, &runtimeapi.PodSandboxStatusRequest{PodSandboxId: podID})
	if err != nil {
		return nil, err
	}

	return resp.GetStatus(), nil
}

func (c *criClient) createContainer(ctx context.Context, podID string, container *runtimeapi.ContainerConfig, sbxCfg *runtimeapi.PodSandboxConfig) (string, error) {
	resp, err := c.runtime.CreateContainer(ctx, &runtimeapi.CreateContainerRequest{
		PodSandboxId:  podID,
		Config:        container,
		SandboxConfig: sbxCfg,
	})
	if err != nil {
		return "", err
	}

	return resp.GetContainerId(), nil
}

func (c *criClient) startContainer(ctx context.Context, containerID string) error {
	_, err := c.runtime.StartContainer(ctx, &runtimeapi.StartContainerRequest{
		ContainerId: containerID,
	})
	return err
}

func (c *criClient) containerStatus(ctx context.Context, containerID string) (*criContainerDetails, error) {
	resp, err := c.runtime.ContainerStatus(ctx, &runtimeapi.ContainerStatusRequest{
		ContainerId: containerID,
		Verbose:     true,
	})
	if err != nil {
		return nil, err
	}

	st := resp.GetStatus()
	d := &criContainerDetails{
		ID:    st.GetId(),
		State: st.GetState(),
		PID:   parseContainerPID(resp.GetInfo()),
	}
	return d, nil
}

func (c *criClient) stopAndRemovePodSandbox(ctx context.Context, podID string) {
	if podID == "" {
		return
	}

	_, _ = c.runtime.StopPodSandbox(ctx, &runtimeapi.StopPodSandboxRequest{PodSandboxId: podID})
	_, _ = c.runtime.RemovePodSandbox(ctx, &runtimeapi.RemovePodSandboxRequest{PodSandboxId: podID})
}

func (c *criClient) listPodSandboxes(ctx context.Context) ([]*runtimeapi.PodSandbox, error) {
	resp, err := c.runtime.ListPodSandbox(ctx, &runtimeapi.ListPodSandboxRequest{})
	if err != nil {
		return nil, err
	}

	return resp.GetItems(), nil
}

func normalizeCRIEndpoint(addr string) string {
	s := strings.TrimSpace(addr)
	s = strings.TrimPrefix(s, "unix://")
	s = strings.TrimPrefix(s, "unix:")
	return s
}

func parseContainerPID(info map[string]string) uint32 {
	for _, raw := range info {
		var payload struct {
			Pid int `json:"pid"`
		}

		if err := json.Unmarshal([]byte(raw), &payload); err != nil {
			continue
		}

		if payload.Pid > 0 {
			return uint32(payload.Pid)
		}
	}

	return 0
}
