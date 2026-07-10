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
	Dir         string    `json:"dir,omitempty"`
	ComposeFile string    `json:"composeFile,omitempty"`
	Services    []Service `json:"services"`
}

// Service is a merged service: its desired image (from the compose file) and
// its running container state (from the daemon), whichever exist.
type Service struct {
	Name        string        `json:"name"`
	Image       string        `json:"image"`                 // desired (compose) if known, else running
	RunningImage string       `json:"runningImage,omitempty"` // actual image on the container
	State       string        `json:"state"`                 // running/exited/created/... or "absent"
	Status      string        `json:"status,omitempty"`
	ContainerID string        `json:"containerId,omitempty"`
	Ports       []docker.Port `json:"ports,omitempty"`
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
func Merge(scanned []ScannedStack, containers []docker.Container, daemonOK bool) []Stack {
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
		out = append(out, buildManaged(s, cts, daemonOK))
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

func buildManaged(s ScannedStack, cts []docker.Container, daemonOK bool) Stack {
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

	var services []Service
	for name := range names {
		svc := Service{Name: name, State: "absent"}
		if def, ok := s.Services[name]; ok {
			svc.Image = def.Image
		}
		if ct, ok := byService[name]; ok {
			applyContainer(&svc, ct)
		}
		services = append(services, svc)
	}
	sort.Slice(services, func(i, j int) bool { return services[i].Name < services[j].Name })

	return Stack{
		Name:        s.Name,
		Project:     s.Project,
		Origin:      OriginManaged,
		Status:      summarize(services, daemonOK),
		Dir:         s.Dir,
		ComposeFile: s.ComposeFile,
		Services:    services,
	}
}

func buildExternal(project string, cts []docker.Container) Stack {
	byService := indexByService(cts)
	var services []Service
	for name, ct := range byService {
		svc := Service{Name: name, State: "absent"}
		applyContainer(&svc, ct)
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

func shortID(id string) string {
	if len(id) > 12 {
		return id[:12]
	}
	return id
}
