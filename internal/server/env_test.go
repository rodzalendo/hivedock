package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func stacksDirWithStack(t *testing.T) (root, stackDir string) {
	t.Helper()
	root = t.TempDir()
	stackDir = filepath.Join(root, "web")
	if err := os.MkdirAll(stackDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stackDir, "compose.yaml"), []byte("services:\n  web:\n    image: nginx\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return root, stackDir
}

func TestGetEnvMissingIsEmpty(t *testing.T) {
	root, _ := stacksDirWithStack(t)
	h := handlerWithStacksDir(t, root)

	req := httptest.NewRequest(http.MethodGet, "/api/stacks/web/env", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var got envFileResponse
	json.Unmarshal(rec.Body.Bytes(), &got)
	if got.Exists {
		t.Error("exists = true, want false for a stack with no .env")
	}
	if got.Content != "" {
		t.Errorf("content = %q, want empty", got.Content)
	}
}

func TestPutThenGetEnv(t *testing.T) {
	root, stackDir := stacksDirWithStack(t)
	h := handlerWithStacksDir(t, root)

	body, _ := json.Marshal(map[string]string{"content": "TAG=1.2.3\nPUID=1000\n"})
	req := httptest.NewRequest(http.MethodPut, "/api/stacks/web/env", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("put status = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}

	// File landed on disk.
	data, err := os.ReadFile(filepath.Join(stackDir, ".env"))
	if err != nil {
		t.Fatalf("read .env: %v", err)
	}
	if string(data) != "TAG=1.2.3\nPUID=1000\n" {
		t.Errorf("disk content = %q", data)
	}

	// GET now reports it exists with the content.
	req = httptest.NewRequest(http.MethodGet, "/api/stacks/web/env", nil)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var got envFileResponse
	json.Unmarshal(rec.Body.Bytes(), &got)
	if !got.Exists || got.Content != "TAG=1.2.3\nPUID=1000\n" {
		t.Errorf("get after put = %+v", got)
	}
}

func TestEnvExternalStackRejected(t *testing.T) {
	h := handlerWithStacksDir(t, t.TempDir()) // no such stack -> 404
	req := httptest.NewRequest(http.MethodGet, "/api/stacks/ghost/env", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}
