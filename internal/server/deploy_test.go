package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
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
