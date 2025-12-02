package engine

import (
	"context"
	"fmt"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
)

type Manager struct {
	cli *client.Client
}

func NewManager() (*Manager, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, err
	}
	return &Manager{cli: cli}, nil
}

func (m *Manager) SpinUp(branchID string, storagePath string) (string, error) {
	ctx := context.Background()
	containerName := fmt.Sprintf("prism-%s", branchID)

	containers, err := m.cli.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		return "", fmt.Errorf("failed to list containers: %w", err)
	}

	for _, c := range containers {
		for _, name := range c.Names {
			if name == "/"+containerName {
				if c.State != "running" {
					if err := m.cli.ContainerStart(ctx, c.ID, container.StartOptions{}); err != nil {
						return "", fmt.Errorf("failed to restart container: %w", err)
					}
				}
				return m.getHostAddress(ctx, c.ID)
			}
		}
	}

	config := &container.Config{
		Image: "postgres:15-alpine",
		Env: []string{
			"POSTGRES_PASSWORD=password",
			"POSTGRES_USER=postgres",
			"POSTGRES_DB=postgres",
		},
		ExposedPorts: nat.PortSet{
			"5432/tcp": struct{}{},
		},
	}

	hostConfig := &container.HostConfig{
		Mounts: []mount.Mount{
			{
				Type:   mount.TypeBind,
				Source: storagePath,
				Target: "/var/lib/postgresql/data",
			},
		},
		NetworkMode: "prism-net",
		PortBindings: nat.PortMap{
			"5432/tcp": []nat.PortBinding{
				{
					HostIP:   "127.0.0.1",
					HostPort: "0",
				},
			},
		},
	}

	resp, err := m.cli.ContainerCreate(ctx, config, hostConfig, nil, nil, containerName)
	if err != nil {
		return "", fmt.Errorf("failed to create container: %w", err)
	}

	if err := m.cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		return "", fmt.Errorf("failed to start container: %w", err)
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
		return "", fmt.Errorf("no port binding found for container")
	}

	return "127.0.0.1:" + bindings[0].HostPort, nil
}
