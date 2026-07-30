package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestHostScopedLocalRoutesEquivalent asserts the step-5 invariant: the
// host-scoped mirror /api/hosts/local/stacks/… hits the same handlers as the
// unscoped /api/stacks/… and produces identical results (docs/MULTIHOST.md).
func TestHostScopedLocalRoutesEquivalent(t *testing.T) {
	dir := t.TempDir()
	h := handlerWithStacksDir(t, dir)

	if rec := createReq(t, h, "web"); rec.Code != http.StatusCreated {
		t.Fatalf("create = %d, want 201", rec.Code)
	}

	get := func(path string) (int, string) {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		return rec.Code, rec.Body.String()
	}

	unscopedCode, unscopedBody := get("/api/stacks/web/compose")
	scopedCode, scopedBody := get("/api/hosts/local/stacks/web/compose")
	if unscopedCode != http.StatusOK || scopedCode != http.StatusOK {
		t.Fatalf("compose GET: unscoped=%d scoped=%d, want 200/200", unscopedCode, scopedCode)
	}
	if unscopedBody != scopedBody {
		t.Errorf("scoped and unscoped bodies differ:\nunscoped=%s\nscoped=%s", unscopedBody, scopedBody)
	}

	// The host-scoped stack list mirrors the unscoped one.
	if c, _ := get("/api/hosts/local/stacks"); c != http.StatusOK {
		t.Errorf("host-scoped list = %d, want 200", c)
	}
}

// TestUnknownHostIsOffline asserts a named-but-unconnected host reports offline
// (502) rather than falling through to the local backend.
func TestUnknownHostIsOffline(t *testing.T) {
	h := handlerWithStacksDir(t, t.TempDir())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/hosts/ghost/stacks", nil))
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("unknown host = %d, want 502 (body=%s)", rec.Code, rec.Body.String())
	}
}
