package server

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
	"time"

	"github.com/rogalinski/hivedock/internal/config"
	"github.com/rogalinski/hivedock/internal/discovery"
	"github.com/rogalinski/hivedock/internal/events"
	"github.com/rogalinski/hivedock/internal/hoststats"
	"github.com/rogalinski/hivedock/internal/stacks"
)

// handlerWithStacksDir builds a handler over a specific stacks dir (so managed
// stacks resolve from real files on disk), no daemon, authenticated via the
// trusted-header test path.
func handlerWithStacksDir(t *testing.T, dir string) http.Handler {
	t.Helper()
	cfg := testAuthCfg(config.Config{Port: "5001", StacksDir: dir, LogLevel: slog.LevelError})
	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
	stacksSvc := stacks.NewManager(dir, nil, logger)
	hub := events.NewHub(50 * time.Millisecond)
	host := hoststats.NewSampler(time.Second)
	icons := discovery.NewIconResolver(t.TempDir(), func(context.Context, string) ([]byte, string, bool) {
		return nil, "", false
	})
	return testAuth(New(context.Background(), cfg, logger, nil, stacksSvc, hub, host, nil, icons, fstest.MapFS{}))
}

func TestGetComposeReturnsFileContent(t *testing.T) {
	dir := t.TempDir()
	stackDir := filepath.Join(dir, "web")
	if err := os.MkdirAll(stackDir, 0o755); err != nil {
		t.Fatal(err)
	}
	want := "services:\n  web:\n    image: nginx:1.27\n"
	if err := os.WriteFile(filepath.Join(stackDir, "compose.yaml"), []byte(want), 0o644); err != nil {
		t.Fatal(err)
	}

	h := handlerWithStacksDir(t, dir)
	req := httptest.NewRequest(http.MethodGet, "/api/stacks/web/compose", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}
	var got composeFileResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Content != want {
		t.Errorf("content = %q, want %q", got.Content, want)
	}
	if got.Path == "" {
		t.Error("empty path in response")
	}
}

func TestGetComposeNotFound(t *testing.T) {
	h := handlerWithStacksDir(t, t.TempDir())
	req := httptest.NewRequest(http.MethodGet, "/api/stacks/ghost/compose", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestAtomicWritePreservesContentAndReplaces(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "compose.yaml")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := atomicWrite(path, []byte("new content\n")); err != nil {
		t.Fatalf("atomicWrite: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new content\n" {
		t.Errorf("content = %q, want %q", got, "new content\n")
	}

	// No leftover temp files in the directory.
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if e.Name() != "compose.yaml" {
			t.Errorf("unexpected leftover file: %s", e.Name())
		}
	}
}
