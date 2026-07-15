package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestReadOnlyAPIToken verifies the §6.5 allowlist: a valid token reaches the
// three GET routes and nothing else — not /api/settings, not any mutation, and
// not with a wrong token. Requests carry no session header, so the token is the
// only thing being tested.
func TestReadOnlyAPIToken(t *testing.T) {
	db := testStore(t)
	const tok = "test-readonly-token-value"
	if err := db.SetSetting(settingReadOnlyToken, sha256hex([]byte(tok))); err != nil {
		t.Fatal(err)
	}
	a := newTestAPI(t, db)
	h := a.mux // not testAuth-wrapped: no injected trusted-header user

	req := func(method, url, token string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, url, nil)
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, r)
		return rr
	}

	// Allowed GET routes.
	for _, url := range []string{"/api/stacks", "/api/updates"} {
		if rec := req(http.MethodGet, url, tok); rec.Code != http.StatusOK {
			t.Errorf("token GET %s = %d, want 200 (body=%s)", url, rec.Code, rec.Body.String())
		}
	}
	// Off the allowlist → rejected.
	if rec := req(http.MethodGet, "/api/settings", tok); rec.Code != http.StatusUnauthorized {
		t.Errorf("token GET /api/settings = %d, want 401", rec.Code)
	}
	if rec := req(http.MethodGet, "/api/home", tok); rec.Code != http.StatusUnauthorized {
		t.Errorf("token GET /api/home = %d, want 401", rec.Code)
	}
	// Mutations → rejected.
	if rec := req(http.MethodPost, "/api/updates/check", tok); rec.Code != http.StatusUnauthorized {
		t.Errorf("token POST /api/updates/check = %d, want 401", rec.Code)
	}
	// Wrong / missing token → rejected.
	if rec := req(http.MethodGet, "/api/stacks", "nope"); rec.Code != http.StatusUnauthorized {
		t.Errorf("wrong token = %d, want 401", rec.Code)
	}
	if rec := req(http.MethodGet, "/api/stacks", ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("no token = %d, want 401", rec.Code)
	}
}
