package server

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/rogalinski/hivedock/internal/auth"
	"github.com/rogalinski/hivedock/internal/store"
)

// Cookie/header names and session lifetime. The session cookie is HttpOnly; the
// CSRF cookie is deliberately readable by JS so the SPA can echo it back in the
// X-CSRF-Token header (double-submit-cookie CSRF defense).
const (
	sessionCookie = "hivedock_session"
	csrfCookie    = "hivedock_csrf"
	csrfHeader    = "X-CSRF-Token"
	sessionTTL    = 30 * 24 * time.Hour
)

// authStatusResponse drives the SPA's initial routing: setup vs login vs app.
type authStatusResponse struct {
	NeedsSetup    bool   `json:"needsSetup"`
	Authenticated bool   `json:"authenticated"`
	Username      string `json:"username,omitempty"`
	ViaProxy      bool   `json:"viaProxy,omitempty"` // authenticated by a trusted proxy header (no local session)
}

// requireAuth gates a route group on a valid session and (for unsafe methods) a
// matching CSRF token. A trusted-header (forward-auth) request from a configured
// proxy CIDR is authenticated directly — no session cookie, so no CSRF check
// applies (a browser can't forge a request carrying that header from within the
// trusted network).
func (a *api) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := a.trustedHeaderUser(r); ok {
			next.ServeHTTP(w, r)
			return
		}
		if a.db == nil {
			writeError(w, http.StatusServiceUnavailable, "store unavailable")
			return
		}
		c, err := r.Cookie(sessionCookie)
		if err != nil || c.Value == "" {
			writeError(w, http.StatusUnauthorized, "authentication required")
			return
		}
		ok, err := a.db.SessionValid(c.Value)
		if err != nil {
			a.logger.Error("auth: session check", "err", err)
			writeError(w, http.StatusInternalServerError, "session check failed")
			return
		}
		if !ok {
			writeError(w, http.StatusUnauthorized, "authentication required")
			return
		}
		// Double-submit CSRF check for state-changing requests only.
		if !isSafeMethod(r.Method) && !csrfOK(r) {
			writeError(w, http.StatusForbidden, "invalid or missing CSRF token")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// trustedHeaderUser returns the authenticated username from the configured
// forward-auth header when the request's *direct TCP peer* (captured before any
// X-Forwarded-For rewriting, see peerIP) is inside a trusted proxy CIDR. This is
// the SSO replacement for the removed AUTH_DISABLED. The value is used only as a
// display/audit name — the model is single-admin.
func (a *api) trustedHeaderUser(r *http.Request) (string, bool) {
	if a.cfg.TrustedHeader == "" || len(a.cfg.TrustedProxyCIDRs) == 0 {
		return "", false
	}
	if !a.peerTrusted(peerIP(r)) {
		return "", false
	}
	v := strings.TrimSpace(r.Header.Get(a.cfg.TrustedHeader))
	if v == "" {
		return "", false
	}
	return v, true
}

// peerTrusted reports whether ip parses and falls inside a configured CIDR.
func (a *api) peerTrusted(ip string) bool {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return false
	}
	for _, n := range a.cfg.TrustedProxyCIDRs {
		if n.Contains(parsed) {
			return true
		}
	}
	return false
}

func isSafeMethod(m string) bool {
	switch m {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	default:
		return false
	}
}

// csrfOK verifies the X-CSRF-Token header matches the CSRF cookie.
func csrfOK(r *http.Request) bool {
	c, err := r.Cookie(csrfCookie)
	if err != nil || c.Value == "" {
		return false
	}
	got := r.Header.Get(csrfHeader)
	return got != "" && subtle.ConstantTimeCompare([]byte(got), []byte(c.Value)) == 1
}

// authStatus reports whether first-run setup is needed and whether the caller
// is currently authenticated (via a trusted proxy header or a session). Public
// (drives the login gate).
func (a *api) authStatus(w http.ResponseWriter, r *http.Request) {
	var resp authStatusResponse
	// A trusted proxy header authenticates directly; no local admin/session is
	// needed, so short-circuit before the setup check.
	if user, ok := a.trustedHeaderUser(r); ok {
		resp.Authenticated = true
		resp.Username = user
		resp.ViaProxy = true
		writeJSON(w, http.StatusOK, resp)
		return
	}
	if a.db == nil {
		writeError(w, http.StatusServiceUnavailable, "store unavailable")
		return
	}
	exists, err := a.db.AdminExists()
	if err != nil {
		a.logger.Error("auth: admin exists", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to read auth state")
		return
	}
	resp.NeedsSetup = !exists
	if exists {
		if c, err := r.Cookie(sessionCookie); err == nil {
			if ok, _ := a.db.SessionValid(c.Value); ok {
				resp.Authenticated = true
				if u, _, err := a.db.AdminCredentials(); err == nil {
					resp.Username = u
				}
			}
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

// authSetup performs first-run admin creation, then logs the new admin in.
func (a *api) authSetup(w http.ResponseWriter, r *http.Request) {
	if a.db == nil {
		writeError(w, http.StatusServiceUnavailable, "store unavailable")
		return
	}
	username, password, ok := decodeCredentials(w, r)
	if !ok {
		return
	}
	if username == "" {
		writeError(w, http.StatusBadRequest, "username is required")
		return
	}
	if len(password) < auth.MinPasswordLen {
		writeError(w, http.StatusBadRequest, "password must be at least 8 characters")
		return
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		a.logger.Error("auth: hash password", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to set password")
		return
	}
	if err := a.db.CreateAdmin(username, hash); err != nil {
		if errors.Is(err, store.ErrAdminExists) {
			writeError(w, http.StatusConflict, "an admin is already configured")
			return
		}
		a.logger.Error("auth: create admin", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to create admin")
		return
	}
	if err := a.startSession(w, r); err != nil {
		a.logger.Error("auth: start session", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to start session")
		return
	}
	writeJSON(w, http.StatusOK, authStatusResponse{Authenticated: true, Username: username})
}

// authLogin verifies credentials and establishes a session.
func (a *api) authLogin(w http.ResponseWriter, r *http.Request) {
	if a.db == nil {
		writeError(w, http.StatusServiceUnavailable, "store unavailable")
		return
	}
	username, password, ok := decodeCredentials(w, r)
	if !ok {
		return
	}
	user, hash, err := a.db.AdminCredentials()
	if err != nil {
		if errors.Is(err, store.ErrNoAdmin) {
			writeError(w, http.StatusConflict, "no admin configured; complete setup first")
			return
		}
		a.logger.Error("auth: read credentials", "err", err)
		writeError(w, http.StatusInternalServerError, "login failed")
		return
	}
	// Always run the hash comparison to avoid leaking whether the username
	// matched via response timing.
	passwordOK := auth.CheckPassword(hash, password)
	if username != user || !passwordOK {
		// Flat delay on failure: with a single admin account this is a simple,
		// state-free brute-force damper (~2.5 attempts/sec/connection at most,
		// on top of bcrypt's own cost).
		time.Sleep(400 * time.Millisecond)
		writeError(w, http.StatusUnauthorized, "invalid username or password")
		return
	}
	if err := a.startSession(w, r); err != nil {
		a.logger.Error("auth: start session", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to start session")
		return
	}
	writeJSON(w, http.StatusOK, authStatusResponse{Authenticated: true, Username: user})
}

// authLogout deletes the session and clears cookies. Behind requireAuth.
func (a *api) authLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil && c.Value != "" && a.db != nil {
		if err := a.db.DeleteSession(c.Value); err != nil {
			a.logger.Warn("auth: delete session", "err", err)
		}
	}
	a.clearSessionCookies(w, r)
	w.WriteHeader(http.StatusNoContent)
}

// startSession mints session + CSRF tokens, persists the session, and sets both
// cookies on the response.
func (a *api) startSession(w http.ResponseWriter, r *http.Request) error {
	token, err := auth.NewToken()
	if err != nil {
		return err
	}
	csrf, err := auth.NewToken()
	if err != nil {
		return err
	}
	if err := a.db.CreateSession(token, time.Now().Add(sessionTTL)); err != nil {
		return err
	}
	a.setSessionCookies(w, r, token, csrf)
	return nil
}

func (a *api) setSessionCookies(w http.ResponseWriter, r *http.Request, session, csrf string) {
	secure := isHTTPS(r)
	expiry := time.Now().Add(sessionTTL)
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: session, Path: "/",
		HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode,
		Expires: expiry, MaxAge: int(sessionTTL.Seconds()),
	})
	http.SetCookie(w, &http.Cookie{
		Name: csrfCookie, Value: csrf, Path: "/",
		HttpOnly: false, Secure: secure, SameSite: http.SameSiteLaxMode,
		Expires: expiry, MaxAge: int(sessionTTL.Seconds()),
	})
}

func (a *api) clearSessionCookies(w http.ResponseWriter, r *http.Request) {
	secure := isHTTPS(r)
	for _, name := range []string{sessionCookie, csrfCookie} {
		http.SetCookie(w, &http.Cookie{
			Name: name, Value: "", Path: "/",
			HttpOnly: name == sessionCookie, Secure: secure, SameSite: http.SameSiteLaxMode,
			MaxAge: -1,
		})
	}
}

// isHTTPS reports whether the request arrived over TLS, honoring a terminating
// reverse proxy's X-Forwarded-Proto. Determines the cookie Secure flag — many
// homelab deployments front Hivedock with plain HTTP, so it can't be hardcoded.
func isHTTPS(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	return strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

// decodeCredentials parses a {username, password} JSON body, writing a 400 and
// returning ok=false on malformed input.
func decodeCredentials(w http.ResponseWriter, r *http.Request) (username, password string, ok bool) {
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return "", "", false
	}
	return strings.TrimSpace(body.Username), body.Password, true
}
