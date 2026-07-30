package server

import (
	"crypto/subtle"
	"net/http"

	"github.com/rogalinski/hivedock/internal/auth"
)

// settingAgentToken stores the hex SHA-256 of the DB-minted agent enrollment
// token (docs/MULTIHOST.md). Only the hash is persisted; the plaintext is shown
// once at generation, mirroring the read-only API token (§6.5). This is the
// UI-friendly alternative to the env AGENT_TOKEN — either enrolls an agent.
const settingAgentToken = "agent_token"

// agentTokenConfigured reports whether agent enrollment is enabled at all: an env
// AGENT_TOKEN or a DB-minted token. When neither is set, /api/agent/connect is
// disabled (404) and multi-host is off.
func (a *api) agentTokenConfigured() bool {
	if a.cfg.AgentToken != "" {
		return true
	}
	if a.db == nil {
		return false
	}
	v, ok, err := a.db.GetSetting(settingAgentToken)
	return err == nil && ok && v != ""
}

// agentTokenAuthorized reports whether an `Authorization: Bearer <token>` header
// matches the env AGENT_TOKEN OR the hashed DB token, in constant time. Either is
// accepted, so an operator can enroll with the env var or the minted token.
func (a *api) agentTokenAuthorized(r *http.Request) bool {
	tok := bearerToken(r)
	if tok == "" {
		return false
	}
	if a.cfg.AgentToken != "" && subtle.ConstantTimeCompare([]byte(tok), []byte(a.cfg.AgentToken)) == 1 {
		return true
	}
	if a.db == nil {
		return false
	}
	stored, ok, err := a.db.GetSetting(settingAgentToken)
	if err != nil || !ok || stored == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(sha256hex([]byte(tok))), []byte(stored)) == 1
}

// generateAgentToken mints a new DB agent-enrollment token, stores only its hash,
// and returns the plaintext once. Regenerating replaces any existing token.
// Requires an admin session (it lives in the guarded group).
func (a *api) generateAgentToken(w http.ResponseWriter, r *http.Request) {
	if a.db == nil {
		writeError(w, http.StatusServiceUnavailable, "store unavailable")
		return
	}
	tok, err := auth.NewToken()
	if err != nil {
		a.logger.Error("agent token: generate", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to generate token")
		return
	}
	if err := a.db.SetSetting(settingAgentToken, sha256hex([]byte(tok))); err != nil {
		a.logger.Error("agent token: store", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to store token")
		return
	}
	a.logger.Info("agent enrollment token generated")
	writeJSON(w, http.StatusOK, map[string]string{"token": tok})
}

// revokeAgentToken deletes the DB agent-enrollment token. An env AGENT_TOKEN, if
// set, still works (it is configured out-of-band).
func (a *api) revokeAgentToken(w http.ResponseWriter, r *http.Request) {
	if a.db == nil {
		writeError(w, http.StatusServiceUnavailable, "store unavailable")
		return
	}
	if err := a.db.DeleteSetting(settingAgentToken); err != nil {
		a.logger.Error("agent token: revoke", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to revoke token")
		return
	}
	a.logger.Info("agent enrollment token revoked")
	writeJSON(w, http.StatusOK, map[string]bool{"revoked": true})
}
