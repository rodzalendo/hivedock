package server

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/rogalinski/hivedock/internal/auth"
)

// settingReadOnlyToken stores the hex SHA-256 of the read-only API token (§6.5).
// Only the hash is persisted; the plaintext is shown once at generation.
const settingReadOnlyToken = "readonly_api_token"

// readOnlyAPIRoutes is the exact GET allowlist the read-only token can reach:
// health + the stack list + the update list, for uptime-kuma / gatus / scripts.
// Everything else — mutations, /api/settings, per-stack detail — is off-limits.
var readOnlyAPIRoutes = map[string]bool{
	"/api/health":  true,
	"/api/stacks":  true,
	"/api/updates": true,
}

// readOnlyTokenAuthorized reports whether the request carries a valid read-only
// API bearer token AND targets an allowlisted GET route. It can never authorize
// a mutation or any route outside the allowlist.
func (a *api) readOnlyTokenAuthorized(r *http.Request) bool {
	if a.db == nil || r.Method != http.MethodGet || !readOnlyAPIRoutes[r.URL.Path] {
		return false
	}
	tok := bearerToken(r)
	if tok == "" {
		return false
	}
	stored, ok, err := a.db.GetSetting(settingReadOnlyToken)
	if err != nil || !ok || stored == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(sha256hex([]byte(tok))), []byte(stored)) == 1
}

// bearerToken extracts the token from an `Authorization: Bearer <token>` header.
func bearerToken(r *http.Request) string {
	const prefix = "Bearer "
	h := r.Header.Get("Authorization")
	if strings.HasPrefix(h, prefix) {
		return strings.TrimSpace(h[len(prefix):])
	}
	return ""
}

// apiTokenExists reports whether a read-only API token is currently set.
func (a *api) apiTokenExists() bool {
	if a.db == nil {
		return false
	}
	_, ok, err := a.db.GetSetting(settingReadOnlyToken)
	return err == nil && ok
}

// generateAPIToken mints a new read-only API token, stores only its hash, and
// returns the plaintext once. Regenerating replaces any existing token. Requires
// an admin session (this route is not in the token's own allowlist).
func (a *api) generateAPIToken(w http.ResponseWriter, r *http.Request) {
	if a.db == nil {
		writeError(w, http.StatusServiceUnavailable, "store unavailable")
		return
	}
	tok, err := auth.NewToken()
	if err != nil {
		a.logger.Error("api token: generate", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to generate token")
		return
	}
	if err := a.db.SetSetting(settingReadOnlyToken, sha256hex([]byte(tok))); err != nil {
		a.logger.Error("api token: store", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to store token")
		return
	}
	a.logger.Info("read-only API token generated")
	writeJSON(w, http.StatusOK, map[string]string{"token": tok})
}

// revokeAPIToken deletes the read-only API token.
func (a *api) revokeAPIToken(w http.ResponseWriter, r *http.Request) {
	if a.db == nil {
		writeError(w, http.StatusServiceUnavailable, "store unavailable")
		return
	}
	if err := a.db.DeleteSetting(settingReadOnlyToken); err != nil {
		a.logger.Error("api token: revoke", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to revoke token")
		return
	}
	a.logger.Info("read-only API token revoked")
	writeJSON(w, http.StatusOK, map[string]bool{"revoked": true})
}
