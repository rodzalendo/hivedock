// Package docker wraps the Docker Engine API client with the narrow, read-only
// surface Hivedock needs in Phase 1. Mutations (Phase 3) shell out to
// `docker compose` and do not live here.
package docker

import (
	"context"
	"fmt"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/client"
)

// Compose label keys Hivedock reads to reconstruct stack membership.
const (
	LabelProject    = "com.docker.compose.project"
	LabelService    = "com.docker.compose.service"
	LabelWorkingDir = "com.docker.compose.project.working_dir"
	LabelConfigFile = "com.docker.compose.project.config_files"
	LabelConfigHash = "com.docker.compose.config-hash"
	LabelOneoff     = "com.docker.compose.oneoff"
)

// Client is a thin wrapper over the Docker SDK client.
type Client struct {
	cli *client.Client
}

// New creates a client from the ambient environment (DOCKER_HOST etc.) with API
// version negotiation so it works across daemon versions.
func New() (*Client, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("create docker client: %w", err)
	}
	return &Client{cli: cli}, nil
}

// Close releases the underlying client.
func (c *Client) Close() error { return c.cli.Close() }

// Ping verifies the daemon is reachable.
func (c *Client) Ping(ctx context.Context) error {
	_, err := c.cli.Ping(ctx)
	return err
}

// Port is a normalized published/exposed port.
type Port struct {
	IP      string `json:"ip,omitempty"`
	Public  uint16 `json:"public,omitempty"`
	Private uint16 `json:"private"`
	Type    string `json:"type"`
}

// Container is a normalized, read-only view of a container plus its parsed
// compose provenance.
type Container struct {
	ID         string
	Name       string
	Image      string
	State      string // running, exited, created, paused, ...
	Status     string // human-readable ("Up 3 hours")
	Ports      []Port
	Labels     map[string]string
	Project    string // compose project ("" if not compose-managed)
	Service    string // compose service
	WorkingDir string // compose project working_dir (absolute)
	ConfigHash string // compose per-service config hash (drift detection)
	Oneoff     bool   // compose one-off container (`compose run`); excluded from stack views
}

// Events streams Docker daemon events (container lifecycle etc.) until ctx is
// cancelled. Callers use these to know when to recompute the truth model.
func (c *Client) Events(ctx context.Context) (<-chan events.Message, <-chan error) {
	return c.cli.Events(ctx, events.ListOptions{})
}

// ListContainers returns all containers (running and stopped), normalized.
func (c *Client) ListContainers(ctx context.Context) ([]Container, error) {
	list, err := c.cli.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		return nil, fmt.Errorf("list containers: %w", err)
	}
	out := make([]Container, 0, len(list))
	for _, ct := range list {
		name := ""
		if len(ct.Names) > 0 {
			name = strings.TrimPrefix(ct.Names[0], "/")
		}
		ports := make([]Port, 0, len(ct.Ports))
		for _, p := range ct.Ports {
			ports = append(ports, Port{IP: p.IP, Public: p.PublicPort, Private: p.PrivatePort, Type: p.Type})
		}
		labels := ct.Labels
		if labels == nil {
			labels = map[string]string{}
		}
		out = append(out, Container{
			ID:         ct.ID,
			Name:       name,
			Image:      ct.Image,
			State:      ct.State,
			Status:     ct.Status,
			Ports:      ports,
			Labels:     labels,
			Project:    labels[LabelProject],
			Service:    labels[LabelService],
			WorkingDir: labels[LabelWorkingDir],
			ConfigHash: labels[LabelConfigHash],
			Oneoff:     labels[LabelOneoff] == "True" || labels[LabelOneoff] == "true",
		})
	}
	return out, nil
}
