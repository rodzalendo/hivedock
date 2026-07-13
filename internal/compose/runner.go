package compose

import (
	"context"
	"fmt"
	"os/exec"
	"sync"
)

// Action is a supported mutating compose lifecycle operation.
type Action string

const (
	ActionUp       Action = "up"       // create/start (detached)
	ActionDown     Action = "down"     // stop and remove
	ActionRestart  Action = "restart"  // restart services
	ActionPull     Action = "pull"     // pull images
	ActionStop     Action = "stop"     // stop without removing
	ActionRecreate Action = "recreate" // up -d --force-recreate: rebuild containers from the file even when compose thinks nothing changed (clears stubborn drift)
	ActionUpdate   Action = "update"   // up -d --pull always: pull newer images (moved digests on latest-style tags) and recreate what changed
)

// Valid reports whether s names a supported action.
func (a Action) Valid() bool {
	switch a {
	case ActionUp, ActionDown, ActionRestart, ActionPull, ActionStop, ActionRecreate, ActionUpdate:
		return true
	default:
		return false
	}
}

// Op describes a single compose invocation against one stack.
type Op struct {
	Stack       string // stack name (the per-stack lock key)
	Action      Action
	ComposeFile string // absolute path to the compose file
	ProjectDir  string // --project-directory (same path in/out of container)
	Service     string // optional: scope the action to a single service
}

// Runner executes mutating `docker compose` subcommands and enforces a
// per-stack concurrency lock so two operations never race on the same stack.
// Reads still go through the Docker SDK; only mutations shell out (see
// docs/CLAUDE.md). The runner is stateless apart from the in-flight lock set.
type Runner struct {
	mu       sync.Mutex
	inflight map[string]bool
}

// NewRunner constructs a Runner.
func NewRunner() *Runner {
	return &Runner{inflight: map[string]bool{}}
}

// Start acquires the per-stack lock. It returns a release func and ok=true when
// the lock was free; ok=false means an operation is already running for stack
// (the caller should reject with a 409). release is safe to call exactly once.
func (r *Runner) Start(stack string) (release func(), ok bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.inflight[stack] {
		return func() {}, false
	}
	r.inflight[stack] = true
	var once sync.Once
	return func() {
		once.Do(func() {
			r.mu.Lock()
			delete(r.inflight, stack)
			r.mu.Unlock()
		})
	}, true
}

// Running reports whether an operation is currently in flight for stack.
func (r *Runner) Running(stack string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.inflight[stack]
}

// Exec runs op's compose command, streaming each line of combined stdout+stderr
// to onLine as it is produced. The caller must already hold the stack lock (via
// Start). It blocks until the subprocess exits or ctx is cancelled; a non-zero
// exit is returned as an error (the streamed output carries the real detail).
func (r *Runner) Exec(ctx context.Context, op Op, onLine func(string)) error {
	if !op.Action.Valid() {
		return fmt.Errorf("unknown compose action %q", op.Action)
	}

	// --ansi never keeps the output pane free of terminal escape codes; there is
	// no TTY on the pipe, so compose already emits plain progress lines.
	args := []string{"compose", "--ansi", "never", "-f", op.ComposeFile, "--project-directory", op.ProjectDir}
	args = append(args, subcommandArgs(op.Action)...)
	if op.Service != "" {
		args = append(args, op.Service)
	}

	cmd := exec.CommandContext(ctx, "docker", args...)
	w := &lineEmitter{emit: onLine}
	cmd.Stdout = w
	cmd.Stderr = w // combined, mutex-guarded so interleaved writes stay line-safe

	if err := cmd.Run(); err != nil {
		w.flush()
		return fmt.Errorf("docker %s: %w", op.Action, err)
	}
	w.flush()
	return nil
}

// subcommandArgs maps an action to its compose subcommand and flags.
func subcommandArgs(a Action) []string {
	switch a {
	case ActionUp:
		return []string{"up", "-d"}
	case ActionDown:
		return []string{"down"}
	case ActionRestart:
		return []string{"restart"}
	case ActionPull:
		return []string{"pull"}
	case ActionStop:
		return []string{"stop"}
	case ActionRecreate:
		return []string{"up", "-d", "--force-recreate"}
	case ActionUpdate:
		return []string{"up", "-d", "--pull", "always"}
	default:
		return nil
	}
}

// lineEmitter buffers writes and emits complete newline-delimited lines. It is
// safe for concurrent Write calls (stdout and stderr share one), so interleaved
// output never splits a line mid-way.
type lineEmitter struct {
	emit func(string)
	mu   sync.Mutex
	buf  []byte
}

func (w *lineEmitter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.buf = append(w.buf, p...)
	for {
		i := indexByte(w.buf, '\n')
		if i < 0 {
			break
		}
		line := w.buf[:i]
		if n := len(line); n > 0 && line[n-1] == '\r' {
			line = line[:n-1]
		}
		w.emit(string(line))
		w.buf = w.buf[i+1:]
	}
	// Bound an unterminated line so a pathological stream can't grow forever.
	if len(w.buf) > 1024*1024 {
		w.emit(string(w.buf))
		w.buf = w.buf[:0]
	}
	return len(p), nil
}

func (w *lineEmitter) flush() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.buf) > 0 {
		w.emit(string(w.buf))
		w.buf = w.buf[:0]
	}
}

func indexByte(b []byte, c byte) int {
	for i := range b {
		if b[i] == c {
			return i
		}
	}
	return -1
}
