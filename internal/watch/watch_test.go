package watch

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rogalinski/hivedock/internal/events"
)

func quietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
}

// TestWatchFSNotifiesOnChange proves the fsnotify path fires a hub notification
// when a compose file changes on a real filesystem. (On Docker Desktop / LXC
// double-bind mounts inotify may not propagate — the periodic rescan covers
// that case; here we exercise the code path on a native fs.)
func TestWatchFSNotifiesOnChange(t *testing.T) {
	dir := t.TempDir()
	stackDir := filepath.Join(dir, "whoami")
	if err := os.MkdirAll(stackDir, 0o755); err != nil {
		t.Fatal(err)
	}
	composeFile := filepath.Join(stackDir, "compose.yaml")
	if err := os.WriteFile(composeFile, []byte("services: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	hub := events.NewHub(20 * time.Millisecond)
	sub, unsub := hub.Subscribe()
	defer unsub()

	w := New(dir, hub, nil, quietLogger())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go w.watchFS(ctx)

	time.Sleep(100 * time.Millisecond) // let watches register

	if err := os.WriteFile(composeFile, []byte("services:\n  whoami:\n    image: x\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	select {
	case msg := <-sub:
		if msg.Type != "stacks:changed" {
			t.Fatalf("type = %q, want stacks:changed", msg.Type)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("expected a change notification after editing a compose file")
	}
}

func TestMeaningfulEvent(t *testing.T) {
	cases := []struct {
		typ, action string
		want        bool
	}{
		{"container", "start", true},
		{"container", "die", true},
		{"container", "health_status: healthy", true},
		{"container", "exec_start: pg_isready", false},
		{"container", "exec_die", false},
		{"container", "top", false},
		{"network", "connect", true},
		{"network", "create", false},
		{"image", "pull", false},
	}
	for _, c := range cases {
		if got := meaningfulEvent(c.typ, c.action); got != c.want {
			t.Errorf("meaningfulEvent(%q,%q) = %v, want %v", c.typ, c.action, got, c.want)
		}
	}
}
