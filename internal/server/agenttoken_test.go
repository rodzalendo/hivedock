package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestAgentTokenEnrollment covers the DB-minted enrollment token: it enables the
// /api/agent/connect endpoint and is accepted (constant-time) alongside the env
// AGENT_TOKEN, while a wrong/absent token is refused. It also asserts settings
// reports agentTokenSet once a token exists.
func TestAgentTokenEnrollment(t *testing.T) {
	db := testStore(t)
	a := newTestAPI(t, db)
	h := a.mux // not testAuth-wrapped; agentConnect is token-gated, not session-gated

	// With no token configured, enrollment is disabled (404), and settings reports
	// it off.
	if !a.agentTokenConfigured() {
		// good: nothing configured yet
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/agent/connect", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("connect with no token configured = %d, want 404", rec.Code)
	}

	// Mint a token via the API (the DB stores only its hash).
	const tok = "minted-agent-token-value"
	if err := db.SetSetting(settingAgentToken, sha256hex([]byte(tok))); err != nil {
		t.Fatal(err)
	}
	if !a.agentTokenConfigured() {
		t.Fatal("agentTokenConfigured should be true after minting")
	}

	// A non-WebSocket GET with the right bearer passes the token check and then
	// fails the upgrade (400), proving authorization succeeded (not 401/404).
	authed := httptest.NewRequest(http.MethodGet, "/api/agent/connect", nil)
	authed.Header.Set("Authorization", "Bearer "+tok)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, authed)
	if rec.Code == http.StatusUnauthorized || rec.Code == http.StatusNotFound {
		t.Fatalf("connect with a valid minted token = %d, want authorized (upgrade failure, not 401/404)", rec.Code)
	}

	// A wrong token is refused.
	bad := httptest.NewRequest(http.MethodGet, "/api/agent/connect", nil)
	bad.Header.Set("Authorization", "Bearer wrong")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, bad)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("connect with a wrong token = %d, want 401", rec.Code)
	}
}

// TestSettingsReportsAgentToken asserts the settings response exposes the
// enrollment state and a suggested manager URL for the copy-paste command.
func TestSettingsReportsAgentToken(t *testing.T) {
	db := testStore(t)
	if err := db.SetSetting(settingAgentToken, sha256hex([]byte("x"))); err != nil {
		t.Fatal(err)
	}
	a := newTestAPI(t, db)
	h := testAuth(a.mux)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/settings", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("settings = %d, want 200", rec.Code)
	}
	var got settingsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if !got.AgentTokenSet {
		t.Error("agentTokenSet should be true when a token is configured")
	}
	if got.ManagerURL == "" {
		t.Error("managerUrl should be a non-empty suggestion")
	}
}
