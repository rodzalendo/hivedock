// Package docker wraps the Docker Engine API client with the narrow, read-only
// surface Hivedock needs in Phase 1. Mutations (Phase 3) shell out to
// `docker compose` and do not live here.
package docker

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/api/types/filters"
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

// SelfBindSource inspects the container named id (HiveDock's own container id /
// hostname) and returns the host-side Source of the bind mounted at dest, plus
// whether such a mount was found. Used by the invariant-4 startup self-check: the
// source must equal dest, or compose relative-path resolution breaks (§6.3).
func (c *Client) SelfBindSource(ctx context.Context, id, dest string) (source string, found bool, err error) {
	insp, err := c.cli.ContainerInspect(ctx, id)
	if err != nil {
		return "", false, fmt.Errorf("inspect self %q: %w", id, err)
	}
	for _, m := range insp.Mounts {
		if m.Destination == dest {
			return m.Source, true, nil
		}
	}
	return "", false, nil
}

// DaemonRuntime reports whether the daemon looks like rootless Docker or Podman —
// both are unsupported and get an explicit banner rather than silent breakage
// (§6.4). Best-effort: unknown on error.
func (c *Client) DaemonRuntime(ctx context.Context) (rootless, podman bool) {
	if info, err := c.cli.Info(ctx); err == nil {
		for _, opt := range info.SecurityOptions {
			if strings.Contains(opt, "rootless") {
				rootless = true
			}
		}
		if strings.Contains(strings.ToLower(info.Name), "podman") ||
			strings.Contains(strings.ToLower(info.OperatingSystem), "podman") {
			podman = true
		}
	}
	if ver, err := c.cli.ServerVersion(ctx); err == nil {
		if strings.Contains(strings.ToLower(ver.Platform.Name), "podman") {
			podman = true
		}
		for _, comp := range ver.Components {
			if strings.Contains(strings.ToLower(comp.Name), "podman") {
				podman = true
			}
		}
	}
	return rootless, podman
}

// Ping verifies the daemon is reachable.
func (c *Client) Ping(ctx context.Context) error {
	_, err := c.cli.Ping(ctx)
	return err
}

// PruneReport summarizes what a prune reclaimed.
type PruneReport struct {
	ImagesDeleted  int    `json:"imagesDeleted"`
	SpaceReclaimed uint64 `json:"spaceReclaimed"`
}

// Prune removes dangling images (untagged layers left behind by updates) and
// stale build cache. It never touches tagged images, containers, volumes, or
// networks — the conservative subset that's always safe after image updates.
func (c *Client) Prune(ctx context.Context) (PruneReport, error) {
	var rep PruneReport

	// Empty filter args = the API's default: dangling images only.
	imgs, err := c.cli.ImagesPrune(ctx, filters.NewArgs())
	if err != nil {
		return rep, fmt.Errorf("prune images: %w", err)
	}
	rep.ImagesDeleted = len(imgs.ImagesDeleted)
	rep.SpaceReclaimed = imgs.SpaceReclaimed

	// Build cache prune is best-effort (older daemons may not support it).
	if bc, err := c.cli.BuildCachePrune(ctx, types.BuildCachePruneOptions{}); err == nil && bc != nil {
		rep.SpaceReclaimed += bc.SpaceReclaimed
	}
	return rep, nil
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
	Health     string // health-check state: healthy/unhealthy/starting ("" = no check)
	Ports      []Port
	Labels     map[string]string
	Project    string // compose project ("" if not compose-managed)
	Service    string // compose service
	WorkingDir string // compose project working_dir (absolute)
	ConfigHash string // compose per-service config hash (drift detection)
	Oneoff     bool   // compose one-off container (`compose run`); excluded from stack views
}

// parseHealth extracts the health-check state from the human status string the
// container list reports (e.g. "Up 5 minutes (unhealthy)"). Docker only appends
// the parenthetical when the image/compose defines a HEALTHCHECK, so "" means
// no health check — not healthy. It avoids a per-container inspect call.
func parseHealth(status string) string {
	switch {
	case strings.Contains(status, "(healthy)"):
		return "healthy"
	case strings.Contains(status, "(unhealthy)"):
		return "unhealthy"
	case strings.Contains(status, "(health: starting)"):
		return "starting"
	default:
		return ""
	}
}

// Events streams Docker daemon events (container lifecycle etc.) until ctx is
// cancelled. Callers use these to know when to recompute the truth model.
func (c *Client) Events(ctx context.Context) (<-chan events.Message, <-chan error) {
	return c.cli.Events(ctx, events.ListOptions{})
}

// ContainerLogs returns the raw log stream for a container. If tty is false the
// stream is Docker's multiplexed stdout/stderr framing and must be demuxed with
// stdcopy; if true it's a raw byte stream. Follow keeps it open until ctx ends.
func (c *Client) ContainerLogs(ctx context.Context, id string, tail int, follow bool) (io.ReadCloser, bool, error) {
	tty := false
	if info, err := c.cli.ContainerInspect(ctx, id); err == nil {
		tty = info.Config != nil && info.Config.Tty
	}

	tailStr := "all"
	if tail > 0 {
		tailStr = strconv.Itoa(tail)
	}
	rc, err := c.cli.ContainerLogs(ctx, id, container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     follow,
		Tail:       tailStr,
		Timestamps: false,
	})
	if err != nil {
		return nil, tty, fmt.Errorf("container logs %s: %w", id, err)
	}
	return rc, tty, nil
}

// declaredHostPorts reads a container's configured host port bindings via
// inspect (HostConfig.PortBindings) — available even when the container is
// stopped, unlike the port list from ContainerList. Best-effort: returns nil on
// any error.
func (c *Client) declaredHostPorts(ctx context.Context, id string) []Port {
	info, err := c.cli.ContainerInspect(ctx, id)
	if err != nil || info.HostConfig == nil {
		return nil
	}
	var out []Port
	for port, bindings := range info.HostConfig.PortBindings {
		for _, b := range bindings {
			if b.HostPort == "" {
				continue
			}
			hp, err := strconv.Atoi(b.HostPort)
			if err != nil || hp <= 0 || hp > 65535 {
				continue
			}
			out = append(out, Port{
				IP:      b.HostIP,
				Public:  uint16(hp),
				Private: uint16(port.Int()),
				Type:    port.Proto(),
			})
		}
	}
	return out
}

// ImageRepoDigest returns the local image's registry manifest digest
// ("sha256:…") for imageRef, from its RepoDigests. This is what the registry's
// Docker-Content-Digest is compared against for the mutable-tag update path.
// Returns "" (no error) when the image has no repo digest (e.g. locally built).
func (c *Client) ImageRepoDigest(ctx context.Context, imageRef string) (string, error) {
	inspect, _, err := c.cli.ImageInspectWithRaw(ctx, imageRef)
	if err != nil {
		return "", fmt.Errorf("inspect image %q: %w", imageRef, err)
	}
	// Prefer a RepoDigest whose repository matches the reference; otherwise take
	// the first. An image can carry digests for several repos (retags).
	repo := imageRef
	if i := strings.LastIndex(repo, ":"); i >= 0 && !strings.Contains(repo[i+1:], "/") {
		repo = repo[:i]
	}
	first := ""
	for _, rd := range inspect.RepoDigests {
		at := strings.LastIndex(rd, "@")
		if at < 0 {
			continue
		}
		if first == "" {
			first = rd[at+1:]
		}
		if strings.HasPrefix(rd, repo+"@") {
			return rd[at+1:], nil
		}
	}
	return first, nil
}

// ImageSource returns the image's org.opencontainers.image.source label (the
// upstream repo URL, used for a changelog link), or "" if absent. Best-effort:
// requires the image to be present locally.
func (c *Client) ImageSource(ctx context.Context, imageRef string) (string, error) {
	inspect, _, err := c.cli.ImageInspectWithRaw(ctx, imageRef)
	if err != nil {
		return "", fmt.Errorf("inspect image %q: %w", imageRef, err)
	}
	if inspect.Config != nil {
		if s := inspect.Config.Labels["org.opencontainers.image.source"]; s != "" {
			return s, nil
		}
	}
	return "", nil
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
		hasPublished := false
		for _, p := range ct.Ports {
			ports = append(ports, Port{IP: p.IP, Public: p.PublicPort, Private: p.PrivatePort, Type: p.Type})
			if p.PublicPort != 0 {
				hasPublished = true
			}
		}
		// A stopped container reports no *published* ports in the list, so its
		// dashboard link would vanish while it's down. Recover the declared host
		// bindings from inspect so a stopped app keeps a clickable URL.
		if !hasPublished && ct.State != "running" {
			if declared := c.declaredHostPorts(ctx, ct.ID); len(declared) > 0 {
				ports = declared
			}
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
			Health:     parseHealth(ct.Status),
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
