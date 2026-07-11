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

func deleteReq(t *testing.T, h http.Handler, name string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodDelete, "/api/stacks/"+name, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func renameReq(t *testing.T, h http.Handler, name, newName string) *httptest.ResponseRecorder {
	t.Helper()
	b, _ := json.Marshal(map[string]string{"newName": newName})
	req := httptest.NewRequest(http.MethodPost, "/api/stacks/"+name+"/rename", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestDeleteStackRemovesDir(t *testing.T) {
	dir := t.TempDir()
	h := handlerWithStacksDir(t, dir)

	if rec := createReq(t, h, "media"); rec.Code != http.StatusCreated {
		t.Fatalf("create = %d, want 201", rec.Code)
	}
	if rec := deleteReq(t, h, "media"); rec.Code != http.StatusNoContent {
		t.Fatalf("delete = %d, want 204 (body=%s)", rec.Code, rec.Body.String())
	}
	if _, err := os.Stat(filepath.Join(dir, "media")); !os.IsNotExist(err) {
		t.Errorf("stack dir still exists after delete (err=%v)", err)
	}
}

func TestDeleteStackNotFound(t *testing.T) {
	h := handlerWithStacksDir(t, t.TempDir())
	if rec := deleteReq(t, h, "ghost"); rec.Code != http.StatusNotFound {
		t.Fatalf("delete missing = %d, want 404", rec.Code)
	}
}

func TestRenameStackMovesDir(t *testing.T) {
	dir := t.TempDir()
	h := handlerWithStacksDir(t, dir)

	if rec := createReq(t, h, "old"); rec.Code != http.StatusCreated {
		t.Fatalf("create = %d, want 201", rec.Code)
	}
	rec := renameReq(t, h, "old", "new")
	if rec.Code != http.StatusOK {
		t.Fatalf("rename = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}
	var resp createStackResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Name != "new" {
		t.Errorf("name = %q, want new", resp.Name)
	}
	if _, err := os.Stat(filepath.Join(dir, "old")); !os.IsNotExist(err) {
		t.Errorf("old dir still exists after rename")
	}
	if _, err := os.Stat(filepath.Join(dir, "new", "compose.yaml")); err != nil {
		t.Errorf("new compose file missing: %v", err)
	}
}

func TestRenameStackConflict(t *testing.T) {
	dir := t.TempDir()
	h := handlerWithStacksDir(t, dir)

	createReq(t, h, "a")
	createReq(t, h, "b")
	if rec := renameReq(t, h, "a", "b"); rec.Code != http.StatusConflict {
		t.Fatalf("rename onto existing = %d, want 409", rec.Code)
	}
}

func TestRenameStackRejectsBadName(t *testing.T) {
	dir := t.TempDir()
	h := handlerWithStacksDir(t, dir)

	createReq(t, h, "x")
	for _, bad := range []string{"", "../evil", "a/b", ".hidden", "bad name"} {
		if rec := renameReq(t, h, "x", bad); rec.Code != http.StatusBadRequest {
			t.Errorf("rename x -> %q = %d, want 400", bad, rec.Code)
		}
	}
	// The original stack must be untouched.
	if _, err := os.Stat(filepath.Join(dir, "x", "compose.yaml")); err != nil {
		t.Errorf("original stack disturbed by bad rename: %v", err)
	}
}
