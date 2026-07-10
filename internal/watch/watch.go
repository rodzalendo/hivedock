// Package watch turns filesystem changes (compose file edits) and Docker daemon
// events (container lifecycle) into hub notifications, so the UI reflects
// reality without polling. Both sources are best-effort: if one is unavailable
// (e.g. inotify doesn't propagate through LXC bind mounts — see
// docs/DEPLOYMENT.md), a periodic rescan still guarantees eventual freshness.
package watch

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/rogalinski/hivedock/internal/docker"
	"github.com/rogalinski/hivedock/internal/events"
)

// Watcher fans change signals into the hub.
type Watcher struct {
	stacksDir string
	hub       *events.Hub
	docker    *docker.Client // may be nil
	logger    *slog.Logger

	// rescanEvery bounds staleness when fs events don't propagate (LXC double
	// bind mounts). See DEPLOYMENT.md — 30s worst case is acceptable.
	rescanEvery time.Duration
}

// New builds a watcher. dockerClient may be nil (no daemon).
func New(stacksDir string, hub *events.Hub, dockerClient *docker.Client, logger *slog.Logger) *Watcher {
	return &Watcher{
		stacksDir:   stacksDir,
		hub:         hub,
		docker:      dockerClient,
		logger:      logger,
		rescanEvery: 30 * time.Second,
	}
}

// Run blocks until ctx is cancelled, watching all sources concurrently.
func (w *Watcher) Run(ctx context.Context) {
	go w.watchFS(ctx)
	go w.watchDocker(ctx)
	w.periodicRescan(ctx)
}

func (w *Watcher) periodicRescan(ctx context.Context) {
	t := time.NewTicker(w.rescanEvery)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			w.hub.NotifyChanged("rescan")
		}
	}
}

func (w *Watcher) watchFS(ctx context.Context) {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		w.logger.Warn("fsnotify unavailable; relying on periodic rescan", "err", err)
		return
	}
	defer fsw.Close()

	w.addWatches(fsw)

	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-fsw.Events:
			if !ok {
				return
			}
			// A newly created directory (new stack) needs its own watch so we
			// see the compose file that lands in it.
			if event.Op&fsnotify.Create != 0 {
				if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
					_ = fsw.Add(event.Name)
				}
			}
			w.logger.Debug("fs event", "op", event.Op.String(), "name", event.Name)
			w.hub.NotifyChanged("fs")
		case err, ok := <-fsw.Errors:
			if !ok {
				return
			}
			w.logger.Warn("fsnotify error", "err", err)
		}
	}
}

// containerActions are the lifecycle events that can change what the stacks
// view shows. Everything else (exec_*, attach, top, resize, health-check
// execs, ...) is ignored to avoid churn.
var containerActions = map[string]bool{
	"create":  true,
	"start":   true,
	"stop":    true,
	"die":     true,
	"kill":    true,
	"restart": true,
	"pause":   true,
	"unpause": true,
	"destroy": true,
	"rename":  true,
	"update":  true,
	"oom":     true,
}

// meaningfulEvent reports whether a daemon event should trigger a refetch.
func meaningfulEvent(evType, action string) bool {
	switch evType {
	case "container":
		if containerActions[action] {
			return true
		}
		// health transitions change the displayed status.
		return strings.HasPrefix(action, "health_status")
	case "network":
		return action == "connect" || action == "disconnect"
	case "volume":
		return action == "create" || action == "destroy"
	default:
		return false
	}
}

// addWatches watches the stacks root and each immediate subdirectory (compose
// files live one level down).
func (w *Watcher) addWatches(fsw *fsnotify.Watcher) {
	if err := fsw.Add(w.stacksDir); err != nil {
		w.logger.Warn("watch stacks dir failed", "dir", w.stacksDir, "err", err)
		return
	}
	entries, err := os.ReadDir(w.stacksDir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() {
			if err := fsw.Add(filepath.Join(w.stacksDir, e.Name())); err != nil {
				w.logger.Debug("watch subdir failed", "err", err)
			}
		}
	}
}

func (w *Watcher) watchDocker(ctx context.Context) {
	if w.docker == nil {
		return
	}
	// The events stream can drop (daemon restart); reconnect with backoff.
	backoff := time.Second
	for {
		if ctx.Err() != nil {
			return
		}
		msgs, errs := w.docker.Events(ctx)
		reconnect := false
		for !reconnect {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-msgs:
				if !ok {
					reconnect = true
					break
				}
				backoff = time.Second // healthy stream; reset backoff
				// Filter out high-frequency, state-irrelevant events (health-check
				// execs etc.) so the UI isn't churned by `pg_isready` loops.
				if !meaningfulEvent(string(msg.Type), string(msg.Action)) {
					continue
				}
				w.logger.Debug("docker event", "type", string(msg.Type), "action", string(msg.Action))
				w.hub.NotifyChanged("docker")
			case err, ok := <-errs:
				if ok && err != nil {
					w.logger.Debug("docker events stream error", "err", err)
				}
				reconnect = true
			}
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		if backoff < 30*time.Second {
			backoff *= 2
		}
	}
}
