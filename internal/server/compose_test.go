package server

import (
	"bytes"
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

func TestEnvSaveOptimisticLock(t *testing.T) {
	dir := t.TempDir()
	stackDir := filepath.Join(dir, "web")
	if err := os.MkdirAll(stackDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stackDir, "compose.yaml"), []byte("services:\n  web:\n    image: nginx\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	envPath := filepath.Join(stackDir, ".env")
	if err := os.WriteFile(envPath, []byte("A=1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	h := handlerWithStacksDir(t, dir)

	// Load the file → capture the base hash the editor would hold.
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/stacks/web/env", nil))
	var loaded envFileResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &loaded); err != nil || loaded.Sha256 == "" {
		t.Fatalf("load: %v (sha=%q)", err, loaded.Sha256)
	}

	// Someone edits the file out of band (e.g. over SSH).
	if err := os.WriteFile(envPath, []byte("A=2\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	put := func(content, base string) *httptest.ResponseRecorder {
		b, _ := json.Marshal(map[string]string{"content": content, "baseSha256": base})
		req := httptest.NewRequest(http.MethodPut, "/api/stacks/web/env", bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		return rr
	}

	// Saving against the stale hash is refused with 409 + the on-disk version.
	rec = put("A=99\n", loaded.Sha256)
	if rec.Code != http.StatusConflict {
		t.Fatalf("stale save = %d, want 409 (body=%s)", rec.Code, rec.Body.String())
	}
	var conflict struct{ Content, Sha256 string }
	json.Unmarshal(rec.Body.Bytes(), &conflict)
	if conflict.Content != "A=2\n" || conflict.Sha256 == "" {
		t.Fatalf("conflict = %+v, want on-disk A=2 with a hash", conflict)
	}

	// Re-saving against the fresh hash (the "overwrite" path) succeeds.
	if rec := put("A=99\n", conflict.Sha256); rec.Code != http.StatusOK {
		t.Fatalf("overwrite save = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}
	if final, _ := os.ReadFile(envPath); string(final) != "A=99\n" {
		t.Errorf("final = %q, want A=99", final)
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
