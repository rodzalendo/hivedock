package stacks

import (
	"context"
	"log/slog"

	"github.com/rogalinski/hivedock/internal/docker"
)

// ContainerLister is the read-only slice of the Docker client this package
// needs. Satisfied by *docker.Client; stubbed in tests.
type ContainerLister interface {
	ListContainers(ctx context.Context) ([]docker.Container, error)
}

// Manager produces the merged truth model on demand.
type Manager struct {
	stacksDir string
	docker    ContainerLister
	logger    *slog.Logger
}

// NewManager builds the stacks manager. docker may be nil (e.g. daemon
// unavailable at startup); List then returns managed stacks with unknown status.
func NewManager(stacksDir string, dockerClient ContainerLister, logger *slog.Logger) *Manager {
	return &Manager{stacksDir: stacksDir, docker: dockerClient, logger: logger}
}

// List returns all stacks (managed + external), merged with runtime state.
func (s *Manager) List(ctx context.Context) ([]Stack, error) {
	scanned, err := Scan(s.stacksDir)
	if err != nil {
		return nil, err
	}

	containers, daemonOK := s.listContainers(ctx)
	return Merge(scanned, containers, daemonOK), nil
}

// Get returns a single stack by name (directory name for managed, project or
// container name for external), or false if not found.
func (s *Manager) Get(ctx context.Context, name string) (Stack, bool, error) {
	all, err := s.List(ctx)
	if err != nil {
		return Stack{}, false, err
	}
	for _, st := range all {
		if st.Name == name {
			return st, true, nil
		}
	}
	return Stack{}, false, nil
}

func (s *Manager) listContainers(ctx context.Context) ([]docker.Container, bool) {
	if s.docker == nil {
		return nil, false
	}
	containers, err := s.docker.ListContainers(ctx)
	if err != nil {
		s.logger.Warn("docker unavailable; showing on-disk stacks only", "err", err)
		return nil, false
	}
	return containers, true
}
