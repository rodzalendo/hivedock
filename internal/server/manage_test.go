package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/rogalinski/hivedock/internal/stacks"
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

// TestHasContainersIncludesStopped is the regression guard for orphaned stacks:
// deleting a stopped stack used to skip `compose down` (it only checked for
// *running* services), leaving exited containers behind. With the directory gone
// those containers still carry their compose project label, so the scanner
// reclassifies them as an external stack — which is read-only and therefore
// impossible to delete from the UI.
func TestHasContainersIncludesStopped(t *testing.T) {
	cases := []struct {
		name  string
		state string
		want  bool
	}{
		{"running", "running", true},
		{"exited", "exited", true},
		{"created", "created", true},
		{"paused", "paused", true},
		{"no container at all", "absent", false},
	}
	for _, tc := range cases {
		st := stacks.Stack{Services: []stacks.Service{{Name: "app", State: tc.state}}}
		if got := hasContainers(st); got != tc.want {
			t.Errorf("hasContainers(state=%q) = %v, want %v", tc.state, got, tc.want)
		}
	}

	// A stack with no services declared has nothing to tear down.
	if hasContainers(stacks.Stack{}) {
		t.Error("hasContainers on an empty stack should be false")
	}
	// Mixed: one absent service must not mask a real container on another.
	mixed := stacks.Stack{Services: []stacks.Service{
		{Name: "a", State: "absent"},
		{Name: "b", State: "exited"},
	}}
	if !hasContainers(mixed) {
		t.Error("hasContainers should be true when any service has a container")
	}
}

func TestDeleteStackWithVolumesQuery(t *testing.T) {
	dir := t.TempDir()
	h := handlerWithStacksDir(t, dir)

	if rec := createReq(t, h, "media"); rec.Code != http.StatusCreated {
		t.Fatalf("create = %d, want 201", rec.Code)
	}
	req := httptest.NewRequest(http.MethodDelete, "/api/stacks/media?volumes=true", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete?volumes=true = %d, want 204 (body=%s)", rec.Code, rec.Body.String())
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
