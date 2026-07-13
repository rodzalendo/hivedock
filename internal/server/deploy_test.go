package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/rogalinski/hivedock/internal/config"
)

func TestRunStackActionUnknownAction(t *testing.T) {
	h := testHandler(t, fstest.MapFS{}) // AUTH_DISABLED, empty stacks dir
	req := httptest.NewRequest(http.MethodPost, "/api/stacks/whatever/actions/bogus", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("unknown action = %d, want 400", rec.Code)
	}
}

func TestRunStackActionStackNotFound(t *testing.T) {
	h := testHandler(t, fstest.MapFS{}) // empty dir -> no such stack
	req := httptest.NewRequest(http.MethodPost, "/api/stacks/nope/actions/up", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("missing stack = %d, want 404", rec.Code)
	}
}

func TestRestartServiceValidation(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "whoami"), 0o755); err != nil {
		t.Fatal(err)
	}
	compose := "services:\n  whoami:\n    image: traefik/whoami:v1.10.1\n"
	if err := os.WriteFile(filepath.Join(dir, "whoami", "compose.yaml"), []byte(compose), 0o644); err != nil {
		t.Fatal(err)
	}
	h := dbHandler(t, config.Config{StacksDir: dir})

	// Unknown stack -> 404.
	req := httptest.NewRequest(http.MethodPost, "/api/stacks/nope/services/whoami/restart", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("missing stack = %d, want 404", rec.Code)
	}

	// Known stack, unknown service -> 404.
	req = httptest.NewRequest(http.MethodPost, "/api/stacks/whoami/services/bogus/restart", nil)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("missing service = %d, want 404", rec.Code)
	}
}
