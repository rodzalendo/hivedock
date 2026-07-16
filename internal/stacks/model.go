package stacks

import (
	"regexp"
	"sort"
	"strings"

	"github.com/rogalinski/hivedock/internal/docker"
)

// Origin classifies where a stack's definition lives.
type Origin string

const (
	// OriginManaged: a compose file exists under STACKS_DIR. Hivedock can edit it.
	OriginManaged Origin = "managed"
	// OriginExternal: running compose project (or standalone container) with no
	// compose file under STACKS_DIR. Shown read-only — we didn't create it.
	OriginExternal Origin = "external"
)

// Status summarizes a stack's aggregate runtime state.
type Status string

const (
	StatusRunning Status = "running" // all defined/known services running
	StatusPartial Status = "partial" // some running, some not
	StatusStopped Status = "stopped" // defined but nothing running
	StatusUnknown Status = "unknown" // daemon unreachable
)

// Stack is the merged, API-facing view of one stack.
type Stack struct {
	Name        string    `json:"name"`
	Project     string    `json:"project"`
	Origin      Origin    `json:"origin"`
	Status      Status    `json:"status"`
	Drifted     bool      `json:"drifted,omitempty"` // running config differs from the compose file
	Dir         string    `json:"dir,omitempty"`
	ComposeFile string    `json:"composeFile,omitempty"`
	Services    []Service `json:"services"`
}

// Service is a merged service: its desired image (from the compose file) and
// its running container state (from the daemon), whichever exist.
type Service struct {
	Name         string        `json:"name"`
	Image        string        `json:"image"`                  // desired (compose) if known, else running
	RunningImage string        `json:"runningImage,omitempty"` // actual image on the container
	State        string        `json:"state"`                  // running/exited/created/... or "absent"
	Status       string        `json:"status,omitempty"`
	Health       string        `json:"health,omitempty"`  // healthy/unhealthy/starting ("" = no health check)
	Drifted      bool          `json:"drifted,omitempty"` // this service's running config-hash != file
	ContainerID  string        `json:"containerId,omitempty"`
	Ports        []docker.Port `json:"ports,omitempty"`

	// Labels is the merged label set (compose file overlaid with runtime
	// container labels). In-memory only — used by discovery, not serialized to
	// keep /api/stacks lean.
	Labels map[string]string `json:"-"`
	// NetworkFrom is the sibling service this one shares a network namespace with
	// (compose `network_mode: service:<name>`); its published ports live on that
	// sibling. In-memory only — discovery uses it to derive the dashboard link.
	NetworkFrom string `json:"-"`
}

var projectSanitize = regexp.MustCompile(`[^a-z0-9_-]`)

// NormalizeProject reproduces Docker Compose's default project name derivation
// from a directory base name: lowercase, then drop any char outside [a-z0-9_-].
func NormalizeProject(dirBase string) string {
	return projectSanitize.ReplaceAllString(strings.ToLower(dirBase), "")
}

// Merge combines scanned (on-disk) stacks with running containers into the
// classified truth model. daemonOK=false means the container list is unreliable
// (daemon unreachable): managed stacks still show, with unknown status.
//
// fileHashes maps stack name -> service -> on-disk config hash (from
// `docker compose config --hash`); when present, a running container whose
// com.docker.compose.config-hash differs is flagged as drifted. Pass nil to
// skip drift detection.
func Merge(scanned []ScannedStack, containers []docker.Container, daemonOK bool, fileHashes map[string]map[string]string) []Stack {
	// Index running containers by compose project (skip one-off `compose run`).
	byProject := map[string][]docker.Container{}
	var standalone []docker.Container
	for _, ct := range containers {
		if ct.Oneoff {
			continue
		}
		if ct.Project == "" {
			standalone = append(standalone, ct)
			continue
		}
		byProject[ct.Project] = append(byProject[ct.Project], ct)
	}

	var out []Stack
	matchedProjects := map[string]bool{}

	// 1. Managed stacks (compose file on disk).
	for _, s := range scanned {
		cts := byProject[s.Project]
		matchedProjects[s.Project] = true
		out = append(out, buildManaged(s, cts, daemonOK, fileHashes[s.Name]))
	}

	// 2. External compose projects: running, but no compose file under STACKS_DIR.
	if daemonOK {
		var extProjects []string
		for proj := range byProject {
			if !matchedProjects[proj] {
				extProjects = append(extProjects, proj)
			}
		}
		sort.Strings(extProjects)
		for _, proj := range extProjects {
			out = append(out, buildExternal(proj, byProject[proj]))
		}

		// 3. Standalone containers (plain `docker run`): each its own external stack.
		sort.Slice(standalone, func(i, j int) bool { return standalone[i].Name < standalone[j].Name })
		for _, ct := range standalone {
			out = append(out, buildStandalone(ct))
		}
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func buildManaged(s ScannedStack, cts []docker.Container, daemonOK bool, hashes map[string]string) Stack {
	byService := indexByService(cts)

	// Union of compose-defined services and any running services (a running
	// service missing from the file is still shown — drift, surfaced later).
	names := map[string]bool{}
	for name := range s.Services {
		names[name] = true
	}
	for svc := range byService {
		names[svc] = true
	}

	stackDrift := false
	services := make([]Service, 0, len(names)) // never nil — the API contract is a list
	for name := range names {
		svc := Service{Name: name, State: "absent"}
		var composeLabels map[string]string
		if def, ok := s.Services[name]; ok {
			svc.Image = def.Image
			svc.NetworkFrom = def.NetworkFrom
			composeLabels = def.Labels
		}
		if ct, ok := byService[name]; ok {
			applyContainer(&svc, ct)
			svc.Labels = mergeLabels(composeLabels, ct.Labels)
			// Drift: the running container's config-hash differs from the
			// hash of the current on-disk config for this service.
			if fileHash, ok := hashes[name]; ok && ct.ConfigHash != "" && fileHash != ct.ConfigHash {
				svc.Drifted = true
				stackDrift = true
			}
		} else {
			svc.Labels = mergeLabels(composeLabels, nil)
		}
		services = append(services, svc)
	}
	sort.Slice(services, func(i, j int) bool { return services[i].Name < services[j].Name })

	return Stack{
		Name:        s.Name,
		Project:     s.Project,
		Origin:      OriginManaged,
		Status:      summarize(services, daemonOK),
		Drifted:     stackDrift,
		Dir:         s.Dir,
		ComposeFile: s.ComposeFile,
		Services:    services,
	}
}

func buildExternal(project string, cts []docker.Container) Stack {
	byService := indexByService(cts)
	services := make([]Service, 0, len(byService))
	for name, ct := range byService {
		svc := Service{Name: name, State: "absent"}
		applyContainer(&svc, ct)
		svc.Labels = mergeLabels(nil, ct.Labels)
		services = append(services, svc)
	}
	sort.Slice(services, func(i, j int) bool { return services[i].Name < services[j].Name })
	return Stack{
		Name:     project,
		Project:  project,
		Origin:   OriginExternal,
		Status:   summarize(services, true),
		Services: services,
	}
}

func buildStandalone(ct docker.Container) Stack {
	svc := Service{Name: ct.Name, State: "absent"}
	applyContainer(&svc, ct)
	svc.Labels = mergeLabels(nil, ct.Labels)
	return Stack{
		Name:     ct.Name,
		Project:  "",
		Origin:   OriginExternal,
		Status:   summarize([]Service{svc}, true),
		Services: []Service{svc},
	}
}

// indexByService keeps the newest-looking container per service name. A stack
// normally has one container per service; if there are more (e.g. scaled), the
// first by name wins for the summary view.
func indexByService(cts []docker.Container) map[string]docker.Container {
	out := map[string]docker.Container{}
	for _, ct := range cts {
		key := ct.Service
		if key == "" {
			key = ct.Name
		}
		if _, exists := out[key]; !exists {
			out[key] = ct
		}
	}
	return out
}

func applyContainer(svc *Service, ct docker.Container) {
	svc.State = ct.State
	svc.Status = ct.Status
	svc.Health = ct.Health
	svc.ContainerID = shortID(ct.ID)
	svc.RunningImage = ct.Image
	svc.Ports = ct.Ports
	if svc.Image == "" {
		svc.Image = ct.Image
	}
}

func summarize(services []Service, daemonOK bool) Status {
	if !daemonOK {
		return StatusUnknown
	}
	if len(services) == 0 {
		return StatusStopped
	}
	running, total := 0, 0
	for _, s := range services {
		total++
		if s.State == "running" {
			running++
		}
	}
	switch {
	case running == 0:
		return StatusStopped
	case running == total:
		return StatusRunning
	default:
		return StatusPartial
	}
}

// mergeLabels overlays runtime container labels on top of compose-file labels.
// They normally agree (compose applies file labels to containers); the runtime
// set wins for external containers where there's no file. Returns nil if both
// are empty.
func mergeLabels(compose, container map[string]string) map[string]string {
	if len(compose) == 0 && len(container) == 0 {
		return nil
	}
	out := make(map[string]string, len(compose)+len(container))
	for k, v := range compose {
		out[k] = v
	}
	for k, v := range container {
		out[k] = v
	}
	return out
}

func shortID(id string) string {
	if len(id) > 12 {
		return id[:12]
	}
	return id
}
