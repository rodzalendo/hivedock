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

	for _, name := range []string{"", "..", "../evil", "a/b", ".hidden", "bad name", strings.Repeat("x", 65)} {
		rec := createReq(t, h, name)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("create %q = %d, want 400", name, rec.Code)
		}
	}
	// Nothing should have been created outside the intended layout.
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Errorf("bad names created %d dirs, want 0", len(entries))
	}
}
