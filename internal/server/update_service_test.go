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

func TestUpdateServicePreviewThenApply(t *testing.T) {
	root := t.TempDir()
	orig := "services:\n  app:\n    image: nginx:1.27.0 # keep me\n"
	writeStack(t, root, "web", orig)
	h := handlerWithStacksDir(t, root)

	post := func(m map[string]any) *httptest.ResponseRecorder {
		b, _ := json.Marshal(m)
		req := httptest.NewRequest(http.MethodPost, "/api/stacks/web/services/app/update", bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		return rr
	}

	// Phase 1 — preview: returns a diff, writes nothing.
	rec := post(map[string]any{"tag": "1.27.2"})
	if rec.Code != http.StatusOK {
		t.Fatalf("preview = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}
	var prev updateServiceResponse
	json.Unmarshal(rec.Body.Bytes(), &prev)
	if !prev.Preview || prev.Sha256 == "" {
		t.Fatalf("preview response = %+v, want preview+sha", prev)
	}
	if !strings.Contains(prev.Diff, "-    image: nginx:1.27.0 # keep me") ||
		!strings.Contains(prev.Diff, "+    image: nginx:1.27.2 # keep me") {
		t.Errorf("diff missing the expected change:\n%s", prev.Diff)
	}
	if got, _ := os.ReadFile(filepath.Join(root, "web", "compose.yaml")); string(got) != orig {
		t.Fatalf("preview wrote to disk: %q", got)
	}

	// Phase 2 — confirm with the preview's hash: applies the byte-exact rewrite.
	rec = post(map[string]any{"tag": "1.27.2", "confirm": true, "baseSha256": prev.Sha256})
	if rec.Code != http.StatusOK {
		t.Fatalf("apply = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}
	got, _ := os.ReadFile(filepath.Join(root, "web", "compose.yaml"))
	if want := "services:\n  app:\n    image: nginx:1.27.2 # keep me\n"; string(got) != want {
		t.Errorf("compose = %q, want %q", got, want)
	}
}

func TestUpdateServiceApplyStaleHash409(t *testing.T) {
	root := t.TempDir()
	writeStack(t, root, "web", "services:\n  app:\n    image: nginx:1.0\n")
	h := handlerWithStacksDir(t, root)
	body, _ := json.Marshal(map[string]any{"tag": "1.1", "confirm": true, "baseSha256": "deadbeef"})
	req := httptest.NewRequest(http.MethodPost, "/api/stacks/web/services/app/update", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("apply with stale hash = %d, want 409", rec.Code)
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
