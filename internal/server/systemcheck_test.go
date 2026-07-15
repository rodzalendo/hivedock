package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestReadOnlyModeBlocksMutations exercises the §6.3 read-only mode: reads still
// work, unsafe methods are refused with 503, and /api/health surfaces the banner.
func TestReadOnlyModeBlocksMutations(t *testing.T) {
	a := newTestAPI(t, nil)
	a.readOnlyReason = "STACKS_DIR bind mismatch: fix the bind and restart"
	a.systemWarnings = []string{a.readOnlyReason}
	h := testAuth(a.mux)

	do := func(method, url string) *httptest.ResponseRecorder {
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, httptest.NewRequest(method, url, nil))
		return rr
	}

	if rec := do(http.MethodGet, "/api/stacks"); rec.Code != http.StatusOK {
		t.Errorf("GET /api/stacks in read-only = %d, want 200", rec.Code)
	}
	if rec := do(http.MethodPost, "/api/app/update"); rec.Code != http.StatusServiceUnavailable {
		t.Errorf("POST in read-only = %d, want 503 (body=%s)", rec.Code, rec.Body.String())
	}
	if rec := do(http.MethodPost, "/api/system/prune"); rec.Code != http.StatusServiceUnavailable {
		t.Errorf("prune in read-only = %d, want 503", rec.Code)
	}

	rec := do(http.MethodGet, "/api/health")
	var got healthResponse
	json.Unmarshal(rec.Body.Bytes(), &got)
	if !got.ReadOnly || len(got.Warnings) == 0 {
		t.Errorf("health = %+v, want readOnly=true with warnings", got)
	}
}

// Without a read-only reason, mutations flow normally (dev/no-daemon path).
func TestReadWriteWhenHealthy(t *testing.T) {
	a := newTestAPI(t, nil)
	h := testAuth(a.mux)
	// version is "dev" in tests → self-update refuses with 409 (not 503).
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/app/update", nil))
	if rec.Code == http.StatusServiceUnavailable {
		t.Errorf("healthy server returned 503 for a mutation")
	}
}
