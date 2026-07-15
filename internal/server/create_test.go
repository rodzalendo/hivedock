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

func createReq(t *testing.T, h http.Handler, name string) *httptest.ResponseRecorder {
	t.Helper()
	b, _ := json.Marshal(map[string]string{"name": name})
	req := httptest.NewRequest(http.MethodPost, "/api/stacks", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestCreateStackWritesTemplate(t *testing.T) {
	dir := t.TempDir()
	h := handlerWithStacksDir(t, dir)

	rec := createReq(t, h, "media")
	if rec.Code != http.StatusCreated {
		t.Fatalf("create = %d, want 201 (body=%s)", rec.Code, rec.Body.String())
	}
	var resp createStackResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Name != "media" {
		t.Errorf("name = %q, want media", resp.Name)
	}

	data, err := os.ReadFile(filepath.Join(dir, "media", "compose.yaml"))
	if err != nil {
		t.Fatalf("read created compose: %v", err)
	}
	if !strings.Contains(string(data), "services:") {
		t.Errorf("template missing services block:\n%s", data)
	}
	if !strings.Contains(string(data), "media") {
		t.Errorf("template comment should mention the stack name")
	}
}

func TestCreateStackConflict(t *testing.T) {
	dir := t.TempDir()
	h := handlerWithStacksDir(t, dir)

	if rec := createReq(t, h, "dup"); rec.Code != http.StatusCreated {
		t.Fatalf("first create = %d, want 201", rec.Code)
	}
	if rec := createReq(t, h, "dup"); rec.Code != http.StatusConflict {
		t.Fatalf("duplicate create = %d, want 409", rec.Code)
	}
}

func TestCreateStackRejectsBadNames(t *testing.T) {
	dir := t.TempDir()
	h := handlerWithStacksDir(t, dir)

	// §4.1: lowercase [a-z0-9_-], starting alphanumeric, max 64. Uppercase,
	// dots, and leading separators are now rejected (compose project-name rule).
	bad := []string{
		"", "..", "../evil", "a/b", ".hidden", "bad name", strings.Repeat("x", 65),
		"MyStack", "Web", "has.dot", "_leading", "-leading", "trailing/",
	}
	for _, name := range bad {
		rec := createReq(t, h, name)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("create %q = %d, want 400", name, rec.Code)
		}
	}
	// Valid lowercase names with dash/underscore are accepted.
	for _, name := range []string{"web", "media-server", "immich_db", "n8n"} {
		if rec := createReq(t, h, name); rec.Code != http.StatusCreated {
			t.Errorf("create %q = %d, want 201 (body=%s)", name, rec.Code, rec.Body.String())
		}
	}
	// Only the four valid names should have produced directories; the bad
	// names must have created nothing.
	entries, _ := os.ReadDir(dir)
	if len(entries) != 4 {
		t.Errorf("got %d stack dirs, want 4 (bad names must create nothing)", len(entries))
	}
}
