package engine

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
)

type Manager struct {
	cli        *client.Client
	lastActive map[string]time.Time
	mu         sync.Mutex
}

func NewManager() (*Manager, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, err
	}
	return &Manager{
		cli:        cli,
		lastActive: make(map[string]time.Time),
	}, nil
}

func (m *Manager) Touch(branchID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastActive[branchID] = time.Now()
}

func (m *Manager) StartReaper(interval time.Duration, idleThreshold time.Duration, logger *slog.Logger) {
	ticker := time.NewTicker(interval)
	go func() {
		for range ticker.C {
			m.reap(idleThreshold, logger)
		}
	}()
}

func (m *Manager) reap(threshold time.Duration, logger *slog.Logger) {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	for branch, lastSeen := range m.lastActive {
		if now.Sub(lastSeen) > threshold {
			logger.Info("Reaper: Container idle, stopping...", "branch", branch)

			containerName := fmt.Sprintf("prism-%s", branch)

			timeout := 5
			err := m.cli.ContainerStop(context.Background(), containerName, container.StopOptions{Timeout: &timeout})
			if err != nil {
				logger.Error("Reaper: Failed to stop", "branch", branch, "error", err)
			} else {
				delete(m.lastActive, branch)
			}
		}
	}
}

func (m *Manager) SpinUp(branchID string, storagePath string) (string, error) {
	m.Touch(branchID)

	ctx := context.Background()
	containerName := fmt.Sprintf("prism-%s", branchID)

	opts := container.ListOptions{
		All:     true,
		Filters: filters.NewArgs(filters.Arg("name", "/"+containerName)),
	}
	containers, err := m.cli.ContainerList(ctx, opts)
	if err != nil {
		return "", fmt.Errorf("list failed: %w", err)
	}

	if len(containers) > 0 {
		c := containers[0]
		if c.State != "running" {
			if err := m.cli.ContainerStart(ctx, c.ID, container.StartOptions{}); err != nil {
				return "", fmt.Errorf("restart failed: %w", err)
			}
			time.Sleep(100 * time.Millisecond)
		}
		return m.getHostAddress(ctx, c.ID)
	}

	config := &container.Config{
		Image:        "postgres:15-alpine",
		Env:          []string{"POSTGRES_PASSWORD=password", "POSTGRES_USER=postgres", "POSTGRES_DB=postgres"},
		ExposedPorts: nat.PortSet{"5432/tcp": struct{}{}},
	}

	hostConfig := &container.HostConfig{
		Mounts:       []mount.Mount{{Type: mount.TypeBind, Source: storagePath, Target: "/var/lib/postgresql/data"}},
		NetworkMode:  "prism-net",
		PortBindings: nat.PortMap{"5432/tcp": []nat.PortBinding{{HostIP: "127.0.0.1", HostPort: "0"}}},
	}

	resp, err := m.cli.ContainerCreate(ctx, config, hostConfig, nil, nil, containerName)
	if err != nil {
		return "", fmt.Errorf("create failed: %w", err)
	}

	if err := m.cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		return "", fmt.Errorf("start failed: %w", err)
	}

	time.Sleep(100 * time.Millisecond)
	return m.getHostAddress(ctx, resp.ID)
}

func (m *Manager) getHostAddress(ctx context.Context, containerID string) (string, error) {
	inspect, err := m.cli.ContainerInspect(ctx, containerID)
	if err != nil {
		return "", err
	}
	bindings := inspect.NetworkSettings.Ports["5432/tcp"]
	if len(bindings) == 0 {
		return "", fmt.Errorf("no port binding")
	}
	return "127.0.0.1:" + bindings[0].HostPort, nil
}

func (m *Manager) ListBranches() map[string]time.Time {
	m.mu.Lock()
	defer m.mu.Unlock()

	copy := make(map[string]time.Time)
	for k, v := range m.lastActive {
		copy[k] = v
	}
	return copy
}
