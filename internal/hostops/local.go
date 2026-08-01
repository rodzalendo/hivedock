package hostops

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/rogalinski/hivedock/internal/compose"
	"github.com/rogalinski/hivedock/internal/docker"
	"github.com/rogalinski/hivedock/internal/stacks"
)

// composeTemplate is the starter compose.yaml written for a blank new stack.
const composeTemplate = `# %s — starter compose file. Edit on the Compose tab, then Save and Deploy.
services:
  app:
    image: nginx:alpine
    restart: unless-stopped
    ports:
      - "8080:80"
`

// LocalBackend runs the stack-management surface in-process against one host's
// STACKS_DIR + docker socket. The manager builds one for "local"; a `hivedock
// agent` builds one for its own host. Both share this exact code, so remote and
// local behave identically (docs/MULTIHOST.md).
type LocalBackend struct {
	stacksDir string
	stacks    *stacks.Manager
	runner    *compose.Runner
	docker    *docker.Client // may be nil (no daemon)
	git       func() bool    // git auto-commit enabled? nil ⇒ never (git is manager-only)
	logger    *slog.Logger
}

// NewLocal builds a LocalBackend. git is the auto-commit seam: the manager passes
// its `a.gitAutoCommitEnabled` (reads the DB); the agent passes nil so remote
// writes never touch git (git stays a manager-only feature).
func NewLocal(stacksDir string, m *stacks.Manager, r *compose.Runner, d *docker.Client, git func() bool, logger *slog.Logger) *LocalBackend {
	return &LocalBackend{stacksDir: stacksDir, stacks: m, runner: r, docker: d, git: git, logger: logger}
}

// getManaged loads a managed stack, or a typed error the caller maps to a status.
func (b *LocalBackend) getManaged(ctx context.Context, name string) (stacks.Stack, error) {
	st, ok, err := b.stacks.Get(ctx, name)
	if err != nil {
		return stacks.Stack{}, err
	}
	if !ok {
		return stacks.Stack{}, ErrNotFound
	}
	if st.Origin != stacks.OriginManaged || st.Dir == "" {
		return stacks.Stack{}, ErrExternal
	}
	return st, nil
}

func (b *LocalBackend) snapshotBefore(action string) error {
	if b.git == nil || !b.git() {
		return nil
	}
	return stacks.GitCommitAll(b.stacksDir, "snapshot before "+action)
}

func (b *LocalBackend) commitAfter(action string) error {
	if b.git == nil || !b.git() {
		return nil
	}
	return stacks.GitCommitAll(b.stacksDir, action)
}

// --- reads ---

func (b *LocalBackend) ListStacks(ctx context.Context) ([]stacks.Stack, error) {
	return b.stacks.List(ctx)
}

func (b *LocalBackend) GetStack(ctx context.Context, name string) (stacks.Stack, error) {
	st, ok, err := b.stacks.Get(ctx, name)
	if err != nil {
		return stacks.Stack{}, err
	}
	if !ok {
		return stacks.Stack{}, ErrNotFound
	}
	return st, nil
}

func (b *LocalBackend) GetCompose(ctx context.Context, name string) (ComposeFile, error) {
	st, err := b.getManaged(ctx, name)
	if err != nil {
		return ComposeFile{}, err
	}
	real, err := Contain(b.stacksDir, st.ComposeFile)
	if err != nil {
		return ComposeFile{}, err
	}
	data, err := os.ReadFile(real)
	if err != nil {
		return ComposeFile{}, err
	}
	return ComposeFile{Path: st.ComposeFile, Content: string(data), Sha256: Sha256Hex(data)}, nil
}

func (b *LocalBackend) ValidateCompose(ctx context.Context, name string, content []byte) (Validation, error) {
	st, err := b.getManaged(ctx, name)
	if err != nil {
		return Validation{}, err
	}
	if err := compose.Validate(ctx, st.Dir, content); err != nil {
		return Validation{Valid: false, Error: err.Error()}, nil
	}
	return Validation{Valid: true}, nil
}

func (b *LocalBackend) GetEnv(ctx context.Context, name string) (EnvFile, error) {
	st, err := b.getManaged(ctx, name)
	if err != nil {
		return EnvFile{}, err
	}
	path, err := Contain(b.stacksDir, filepath.Join(st.Dir, ".env"))
	if err != nil {
		return EnvFile{}, err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return EnvFile{Path: path, Content: "", Exists: false, Sha256: Sha256Hex(nil)}, nil
	}
	if err != nil {
		return EnvFile{}, err
	}
	return EnvFile{Path: path, Content: string(data), Exists: true, Sha256: Sha256Hex(data)}, nil
}

// --- file writes ---

func (b *LocalBackend) PutCompose(ctx context.Context, name string, content []byte, baseSha string) (ComposeFile, error) {
	st, err := b.getManaged(ctx, name)
	if err != nil {
		return ComposeFile{}, err
	}
	real, err := Contain(b.stacksDir, st.ComposeFile)
	if err != nil {
		return ComposeFile{}, err
	}
	if err := CheckLock(real, baseSha); err != nil {
		return ComposeFile{}, err
	}
	if err := compose.Validate(ctx, st.Dir, content); err != nil {
		return ComposeFile{}, &ValidationError{Msg: err.Error()}
	}
	action := "save " + name + " compose"
	if err := b.snapshotBefore(action); err != nil {
		return ComposeFile{}, err
	}
	if err := AtomicWrite(real, content); err != nil {
		return ComposeFile{}, err
	}
	if err := b.commitAfter(action); err != nil {
		return ComposeFile{}, err
	}
	return ComposeFile{Path: st.ComposeFile, Content: string(content), Sha256: Sha256Hex(content)}, nil
}

func (b *LocalBackend) PutEnv(ctx context.Context, name string, content []byte, baseSha string) (EnvFile, error) {
	st, err := b.getManaged(ctx, name)
	if err != nil {
		return EnvFile{}, err
	}
	path, err := Contain(b.stacksDir, filepath.Join(st.Dir, ".env"))
	if err != nil {
		return EnvFile{}, err
	}
	if err := CheckLock(path, baseSha); err != nil {
		return EnvFile{}, err
	}
	action := "save " + name + " .env"
	if err := b.snapshotBefore(action); err != nil {
		return EnvFile{}, err
	}
	if err := AtomicWrite(path, content); err != nil {
		return EnvFile{}, err
	}
	if err := b.commitAfter(action); err != nil {
		return EnvFile{}, err
	}
	return EnvFile{Path: path, Content: string(content), Exists: true, Sha256: Sha256Hex(content)}, nil
}

func (b *LocalBackend) CreateStack(ctx context.Context, name, composeYAML string) (StackRef, error) {
	if !ValidStackName(name) {
		return StackRef{}, ErrInvalidName
	}
	root, err := filepath.Abs(b.stacksDir)
	if err != nil {
		return StackRef{}, err
	}
	dir := filepath.Join(root, name)
	// Defense in depth: the dir must be a direct child of the root and stay inside it.
	if filepath.Dir(dir) != root {
		return StackRef{}, ErrInvalidName
	}
	if _, err := Contain(b.stacksDir, dir); err != nil {
		return StackRef{}, err
	}
	if _, err := os.Stat(dir); err == nil {
		return StackRef{}, ErrExists
	} else if !os.IsNotExist(err) {
		return StackRef{}, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return StackRef{}, err
	}
	composeFile := filepath.Join(dir, "compose.yaml")
	content := fmt.Sprintf(composeTemplate, name)
	if composeYAML != "" {
		content = composeYAML
	}
	if err := os.WriteFile(composeFile, []byte(content), 0o644); err != nil {
		_ = os.Remove(dir) // roll back the empty dir
		return StackRef{}, err
	}
	return StackRef{Name: name, Dir: dir, ComposeFile: composeFile}, nil
}

func (b *LocalBackend) DeleteStack(ctx context.Context, name string, volumes bool) error {
	st, err := b.getManaged(ctx, name)
	if err != nil {
		return err
	}
	dir, err := b.childOfStacksDir(st.Dir)
	if err != nil {
		return err
	}
	// Tear the stack down (even stopped containers keep the compose project label,
	// so skipping `down` would strand them as an undeletable external stack).
	if hasContainers(st) && st.ComposeFile != "" {
		op := compose.Op{
			Stack: name, Action: compose.ActionDown,
			ComposeFile: st.ComposeFile, ProjectDir: st.Dir, Volumes: volumes,
		}
		if err := b.runner.Exec(ctx, op, func(string) {}); err != nil {
			return fmt.Errorf("failed to tear down the stack before deleting (stop it first, then retry): %w", err)
		}
	}
	return os.RemoveAll(dir)
}

func (b *LocalBackend) RenameStack(ctx context.Context, name, newName string) (StackRef, error) {
	if !ValidStackName(newName) {
		return StackRef{}, ErrInvalidName
	}
	if newName == name {
		return StackRef{}, fmt.Errorf("new name is the same as the current name")
	}
	st, err := b.getManaged(ctx, name)
	if err != nil {
		return StackRef{}, err
	}
	if hasRunning(st) {
		return StackRef{}, ErrRunning
	}
	oldDir, err := b.childOfStacksDir(st.Dir)
	if err != nil {
		return StackRef{}, err
	}
	root := filepath.Dir(oldDir)
	newDir := filepath.Join(root, newName)
	if filepath.Dir(newDir) != root {
		return StackRef{}, ErrInvalidName
	}
	if _, err := os.Stat(newDir); err == nil {
		return StackRef{}, ErrExists
	} else if !os.IsNotExist(err) {
		return StackRef{}, err
	}
	if err := os.Rename(oldDir, newDir); err != nil {
		return StackRef{}, err
	}
	composeFile := ""
	if st.ComposeFile != "" {
		composeFile = filepath.Join(newDir, filepath.Base(st.ComposeFile))
	}
	return StackRef{Name: newName, Dir: newDir, ComposeFile: composeFile}, nil
}

func (b *LocalBackend) UpdateService(ctx context.Context, req UpdateServiceReq) (UpdateServiceResult, error) {
	st, err := b.getManaged(ctx, req.Name)
	if err != nil {
		return UpdateServiceResult{}, err
	}
	real, err := Contain(b.stacksDir, st.ComposeFile)
	if err != nil {
		return UpdateServiceResult{}, err
	}
	content, err := os.ReadFile(real)
	if err != nil {
		return UpdateServiceResult{}, err
	}
	updated, err := compose.SetImageTag(content, req.Service, req.Tag)
	if err != nil {
		return UpdateServiceResult{}, err // env-managed / digest-pinned / not-found / no-image → typed codes
	}
	label := st.Name + "/" + filepath.Base(st.ComposeFile)
	if bytes.Equal(updated, content) {
		return UpdateServiceResult{Stack: st.Name, Service: req.Service, Tag: req.Tag, Changed: false, Sha256: Sha256Hex(content)}, nil
	}
	if !req.Confirm {
		return UpdateServiceResult{
			Stack: st.Name, Service: req.Service, Tag: req.Tag, Changed: true,
			Preview: true, Diff: unifiedDiff(content, updated, label), Sha256: Sha256Hex(content),
		}, nil
	}
	if err := CheckLock(real, req.BaseSha256); err != nil {
		return UpdateServiceResult{}, err
	}
	action := "update " + st.Name + "/" + req.Service + " to " + req.Tag
	if err := b.snapshotBefore(action); err != nil {
		return UpdateServiceResult{}, err
	}
	if err := AtomicWrite(real, updated); err != nil {
		return UpdateServiceResult{}, err
	}
	if err := b.commitAfter(action); err != nil {
		return UpdateServiceResult{}, err
	}
	return UpdateServiceResult{Stack: st.Name, Service: req.Service, Tag: req.Tag, Changed: true, Sha256: Sha256Hex(updated)}, nil
}

// --- streaming (caller holds the per-stack lock) ---

func (b *LocalBackend) RunAction(ctx context.Context, name, action, service string, onLine func(string)) error {
	st, err := b.getManaged(ctx, name)
	if err != nil {
		return err
	}
	act := compose.Action(action)
	if !act.Valid() {
		return fmt.Errorf("unknown action: %s", action)
	}
	if service != "" {
		found := false
		for _, svc := range st.Services {
			if svc.Name == service {
				found = true
				break
			}
		}
		if !found {
			return compose.ErrServiceNotFound
		}
	}
	op := compose.Op{Stack: name, Action: act, ComposeFile: st.ComposeFile, ProjectDir: st.Dir, Service: service}
	return b.runner.Exec(ctx, op, onLine)
}

// Logs follows every container of a stack until ctx is cancelled. Two rules make
// it behave like `docker compose logs -f`:
//
//   - State is not filtered. A crash-looping (restarting) or exited container is
//     exactly when its output matters most; docker's follow survives a restart of
//     the same container, so a crash loop keeps streaming.
//   - Containers are re-resolved on a ticker, so a deploy/recreate — which gives
//     every service a NEW container ID and kills the old follow — reattaches
//     instead of leaving a dead pane. Each container ID is attached at most once.
func (b *LocalBackend) Logs(ctx context.Context, name string, tail int, onLine func(LogLine)) error {
	if b.docker == nil {
		return ErrNoDocker
	}
	if tail <= 0 {
		tail = logTailDefault
	}

	var wg sync.WaitGroup
	attached := map[string]bool{} // container ID -> already following

	attach := func() error {
		st, ok, err := b.stacks.Get(ctx, name)
		if err != nil {
			return err
		}
		if !ok {
			return ErrNotFound
		}
		for _, svc := range logTargets(st) {
			if attached[svc.ContainerID] {
				continue
			}
			attached[svc.ContainerID] = true
			wg.Add(1)
			go func(service, id string) {
				defer wg.Done()
				_ = b.streamOneContainer(ctx, service, id, tail, onLine)
			}(svc.Name, svc.ContainerID)
		}
		return nil
	}

	if err := attach(); err != nil {
		return err
	}
	// Nothing deployed yet is not an error: the rescan below fills the pane in as
	// soon as a deploy creates containers (the UI shows its waiting placeholder).
	ticker := time.NewTicker(logRescanInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			wg.Wait()
			return nil
		case <-ticker.C:
			if err := attach(); err != nil && !errors.Is(err, ErrNotFound) {
				b.logger.Debug("logs rescan failed", "stack", name, "err", err)
			}
		}
	}
}

// Runner exposes the per-stack lock so the server/agent can 409 a busy stack
// before starting a streaming op (the interface keeps locking with the caller).
func (b *LocalBackend) Runner() *compose.Runner { return b.runner }

// --- path helpers ---

// childOfStacksDir requires dir to be a direct child of STACKS_DIR (symlinks
// resolved), returning its real path — defense in depth for delete/rename.
func (b *LocalBackend) childOfStacksDir(dir string) (string, error) {
	root, err := filepath.EvalSymlinks(b.stacksDir)
	if err != nil {
		if root, err = filepath.Abs(b.stacksDir); err != nil {
			return "", err
		}
	}
	abs, err := stacks.Contained(b.stacksDir, dir)
	if err != nil {
		return "", ErrEscape
	}
	if abs == root || filepath.Dir(abs) != root {
		return "", ErrEscape
	}
	return abs, nil
}

// logTargets picks the services whose logs can be followed: every one that has a
// container, whatever its state. Deliberately NOT filtered to "running" — a
// crash-looping (restarting) or exited container is when its output matters most.
func logTargets(st stacks.Stack) []stacks.Service {
	var out []stacks.Service
	for _, svc := range st.Services {
		if svc.ContainerID != "" {
			out = append(out, svc)
		}
	}
	return out
}

func hasContainers(st stacks.Stack) bool {
	for _, svc := range st.Services {
		if svc.State != "absent" {
			return true
		}
	}
	return false
}

func hasRunning(st stacks.Stack) bool {
	for _, svc := range st.Services {
		if svc.State == "running" {
			return true
		}
	}
	return false
}

// unifiedDiff renders a single-hunk unified diff between oldB and newB (our
// machine edits are localized), with up to 3 lines of context.
func unifiedDiff(oldB, newB []byte, label string) string {
	oldL := strings.Split(string(oldB), "\n")
	newL := strings.Split(string(newB), "\n")

	p := 0
	for p < len(oldL) && p < len(newL) && oldL[p] == newL[p] {
		p++
	}
	s := 0
	for s < len(oldL)-p && s < len(newL)-p && oldL[len(oldL)-1-s] == newL[len(newL)-1-s] {
		s++
	}
	if p == len(oldL) && p == len(newL) {
		return ""
	}
	const ctxN = 3
	start := max(0, p-ctxN)
	oldChangeEnd, newChangeEnd := len(oldL)-s, len(newL)-s
	oldEnd := min(len(oldL), oldChangeEnd+ctxN)
	newEnd := min(len(newL), newChangeEnd+ctxN)

	var b strings.Builder
	fmt.Fprintf(&b, "--- %s\n+++ %s\n", label, label)
	fmt.Fprintf(&b, "@@ -%d,%d +%d,%d @@\n", start+1, oldEnd-start, start+1, newEnd-start)
	for i := start; i < p; i++ {
		fmt.Fprintf(&b, " %s\n", oldL[i])
	}
	for i := p; i < oldChangeEnd; i++ {
		fmt.Fprintf(&b, "-%s\n", oldL[i])
	}
	for i := p; i < newChangeEnd; i++ {
		fmt.Fprintf(&b, "+%s\n", newL[i])
	}
	for i := oldChangeEnd; i < oldEnd; i++ {
		fmt.Fprintf(&b, " %s\n", oldL[i])
	}
	return b.String()
}
