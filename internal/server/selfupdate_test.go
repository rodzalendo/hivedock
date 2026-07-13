package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
)

func TestAppUpdateDevBuildNotCheckable(t *testing.T) {
	h := testHandler(t, fstest.MapFS{}) // version is "dev" in tests
	req := httptest.NewRequest(http.MethodGet, "/api/app/update", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var got appUpdateResponse
	json.Unmarshal(rec.Body.Bytes(), &got)
	if got.Checkable || got.HasUpdate {
		t.Errorf("dev build should not be checkable: %+v", got)
	}
	if got.Current != "dev" {
		t.Errorf("current = %q, want dev", got.Current)
	}
}

func TestSelfUpdateOutsideComposeFails(t *testing.T) {
	// In the test environment there is no compose-labelled hivedock container
	// (and possibly no docker CLI at all), so the self-inspection must fail
	// cleanly with a 409 — never a hang or a stray helper container.
	h := testHandler(t, fstest.MapFS{})
	req := httptest.NewRequest(http.MethodPost, "/api/app/update", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409 (body=%s)", rec.Code, rec.Body.String())
	}
}
