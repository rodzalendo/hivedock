package server

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"os"
	"strconv"
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
		// Read-only API token: valid only for allowlisted GET routes (§6.5).
		if a.readOnlyTokenAuthorized(r) {
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
		// Self-heal a missing CSRF cookie. A session cookie and its CSRF cookie
		// are minted together at login, but they can desynchronize — a session
		// that outlives a redeploy which changed the cookie scheme, or plain
		// browser eviction of the (non-HttpOnly) CSRF cookie. When that happens
		// the session stays valid but every mutation 403s forever, because
		// nothing ever reissued the CSRF cookie. Reissue it on safe requests
		// (the SPA polls several per minute), so the next mutation has a matching
		// token instead of a permanently stuck 403. Never on unsafe requests:
		// that would defeat the double-submit check by handing the caller a fresh
		// pair mid-mutation.
		if isSafeMethod(r.Method) {
			if _, err := r.Cookie(csrfCookie); err != nil {
				if csrf, err := auth.NewToken(); err == nil {
					a.setCSRFCookie(w, r, csrf)
				}
			}
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
// While a one-time setup token is active (no admin yet, no env bootstrap), the
// request must present it — closing the unclaimed-instance race. Rate limited.
func (a *api) authSetup(w http.ResponseWriter, r *http.Request) {
	if a.db == nil {
		writeError(w, http.StatusServiceUnavailable, "store unavailable")
		return
	}
	ip := clientIP(r)
	key := "setup:" + ip
	if d := a.login.retryAfter(key); d > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(int(d.Seconds())+1))
		writeError(w, http.StatusTooManyRequests, "too many attempts; try again later")
		return
	}
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Token    string `json:"token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// One-time setup token gate (skipped once setup is done / when bootstrapped).
	a.setupMu.Lock()
	need := a.setupToken
	a.setupMu.Unlock()
	if need != "" {
		if subtle.ConstantTimeCompare([]byte(strings.TrimSpace(body.Token)), []byte(need)) != 1 {
			a.login.fail(key)
			a.logger.Warn("auth: invalid setup token", "ip", ip)
			time.Sleep(400 * time.Millisecond)
			writeError(w, http.StatusForbidden, "invalid or missing setup token — see the container log (docker logs hivedock)")
			return
		}
	}

	username := strings.TrimSpace(body.Username)
	if username == "" {
		writeError(w, http.StatusBadRequest, "username is required")
		return
	}
	if len(body.Password) < auth.MinPasswordLen {
		writeError(w, http.StatusBadRequest, "password must be at least 8 characters")
		return
	}
	hash, err := auth.HashPassword(body.Password)
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
	// Setup done: retire the token and clear the limiter.
	a.setupMu.Lock()
	a.setupToken = ""
	a.setupMu.Unlock()
	a.login.reset(key)

	if err := a.startSession(w, r); err != nil {
		a.logger.Error("auth: start session", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to start session")
		return
	}
	writeJSON(w, http.StatusOK, authStatusResponse{Authenticated: true, Username: username})
}

// authLogin verifies credentials and establishes a session. Failures are
// rate-limited per (username, ip) with exponential backoff.
func (a *api) authLogin(w http.ResponseWriter, r *http.Request) {
	if a.db == nil {
		writeError(w, http.StatusServiceUnavailable, "store unavailable")
		return
	}
	username, password, ok := decodeCredentials(w, r)
	if !ok {
		return
	}
	ip := clientIP(r)
	key := "login:" + username + "|" + ip
	if d := a.login.retryAfter(key); d > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(int(d.Seconds())+1))
		writeError(w, http.StatusTooManyRequests, "too many attempts; try again later")
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
		a.login.fail(key)
		// Fixed-format line for the fail2ban filter in SECURITY.md.
		a.logger.Warn("auth: failed login", "user", username, "ip", ip)
		time.Sleep(400 * time.Millisecond) // flat per-attempt damper on top of backoff
		writeError(w, http.StatusUnauthorized, "invalid username or password")
		return
	}
	a.login.reset(key)
	if err := a.startSession(w, r); err != nil {
		a.logger.Error("auth: start session", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to start session")
		return
	}
	writeJSON(w, http.StatusOK, authStatusResponse{Authenticated: true, Username: user})
}

// initFirstRun runs once at startup when no admin exists yet: it bootstraps the
// admin from ADMIN_USER + ADMIN_PASSWORD_FILE if provided, otherwise mints a
// one-time setup token and logs it (so first-run setup can't be claimed by a
// stranger who reaches the box first).
func (a *api) initFirstRun() {
	if a.db == nil {
		return
	}
	exists, err := a.db.AdminExists()
	if err != nil {
		a.logger.Error("first-run: admin check", "err", err)
		return
	}
	if exists {
		return
	}
	if a.cfg.AdminUser != "" && a.cfg.AdminPasswordFile != "" {
		if a.bootstrapAdmin() {
			return
		}
		// fall through to the token path on any bootstrap failure
	}
	tok, err := auth.NewToken()
	if err != nil {
		a.logger.Error("first-run: mint setup token", "err", err)
		return
	}
	a.setupToken = tok
	a.logger.Info("FIRST-RUN SETUP: create the admin account in the UI using this one-time token", "setup_token", tok)
}

// bootstrapAdmin creates the admin from ADMIN_USER + ADMIN_PASSWORD_FILE.
// Reports success; on failure the caller falls back to the setup-token path.
func (a *api) bootstrapAdmin() bool {
	data, err := os.ReadFile(a.cfg.AdminPasswordFile)
	if err != nil {
		a.logger.Error("bootstrap admin: read ADMIN_PASSWORD_FILE", "err", err)
		return false
	}
	pw := strings.TrimSpace(string(data))
	if len(pw) < auth.MinPasswordLen {
		a.logger.Error("bootstrap admin: password must be at least 8 characters")
		return false
	}
	hash, err := auth.HashPassword(pw)
	if err != nil {
		a.logger.Error("bootstrap admin: hash password", "err", err)
		return false
	}
	if err := a.db.CreateAdmin(a.cfg.AdminUser, hash); err != nil {
		if errors.Is(err, store.ErrAdminExists) {
			return true
		}
		a.logger.Error("bootstrap admin: create", "err", err)
		return false
	}
	a.logger.Info("admin bootstrapped from ADMIN_USER / ADMIN_PASSWORD_FILE", "user", a.cfg.AdminUser)
	return true
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
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: session, Path: "/",
		HttpOnly: true, Secure: isHTTPS(r), SameSite: http.SameSiteLaxMode,
		Expires: time.Now().Add(sessionTTL), MaxAge: int(sessionTTL.Seconds()),
	})
	a.setCSRFCookie(w, r, csrf)
}

// setCSRFCookie sets just the CSRF cookie. Deliberately not HttpOnly so the SPA
// can read it and echo it back in the X-CSRF-Token header (double-submit). Used
// at login (alongside the session cookie) and by requireAuth's self-heal.
func (a *api) setCSRFCookie(w http.ResponseWriter, r *http.Request, csrf string) {
	http.SetCookie(w, &http.Cookie{
		Name: csrfCookie, Value: csrf, Path: "/",
		HttpOnly: false, Secure: isHTTPS(r), SameSite: http.SameSiteLaxMode,
		Expires: time.Now().Add(sessionTTL), MaxAge: int(sessionTTL.Seconds()),
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
