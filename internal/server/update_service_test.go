package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeStack(t *testing.T, root, name, compose string) {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "compose.yaml"), []byte(compose), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestUpdateServiceRewritesTag(t *testing.T) {
	root := t.TempDir()
	writeStack(t, root, "web", "services:\n  app:\n    image: nginx:1.27.0 # keep me\n")
	h := handlerWithStacksDir(t, root)

	body, _ := json.Marshal(map[string]string{"tag": "1.27.2"})
	req := httptest.NewRequest(http.MethodPost, "/api/stacks/web/services/app/update", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}
	got, _ := os.ReadFile(filepath.Join(root, "web", "compose.yaml"))
	if want := "services:\n  app:\n    image: nginx:1.27.2 # keep me\n"; string(got) != want {
		t.Errorf("compose = %q, want %q", got, want)
	}
}

func TestUpdateServiceEnvManaged409(t *testing.T) {
	root := t.TempDir()
	writeStack(t, root, "media", "services:\n  jf:\n    image: jellyfin/jellyfin:${TAG}\n")
	h := handlerWithStacksDir(t, root)

	body, _ := json.Marshal(map[string]string{"tag": "10.9.0"})
	req := httptest.NewRequest(http.MethodPost, "/api/stacks/media/services/jf/update", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("env-managed update = %d, want 409", rec.Code)
	}
	// File must be untouched.
	got, _ := os.ReadFile(filepath.Join(root, "media", "compose.yaml"))
	if !strings.Contains(string(got), "${TAG}") {
		t.Errorf("env-managed image was rewritten: %q", got)
	}
}
