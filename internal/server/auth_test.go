package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
	"time"

	"github.com/rogalinski/hivedock/internal/config"
	"github.com/rogalinski/hivedock/internal/discovery"
	"github.com/rogalinski/hivedock/internal/events"
	"github.com/rogalinski/hivedock/internal/hoststats"
	"github.com/rogalinski/hivedock/internal/stacks"
	"github.com/rogalinski/hivedock/internal/store"
)

// authTestServer builds a server with a real store and password auth (no
// trusted header). Returns the *api so tests can read the first-run setup
// token; use s.mux as the handler.
func authTestServer(t *testing.T) *api {
	t.Helper()
	stacksDir := t.TempDir()
	db, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	cfg := config.Config{Port: "5001", StacksDir: stacksDir, LogLevel: slog.LevelError}
	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
	stacksSvc := stacks.NewManager(stacksDir, nil, logger)
	hub := events.NewHub(50 * time.Millisecond)
	host := hoststats.NewSampler(time.Second)
	icons := discovery.NewIconResolver(t.TempDir(), func(context.Context, string) ([]byte, string, bool) {
		return nil, "", false
	})
	return newServer(context.Background(), cfg, logger, db, stacksSvc, hub, host, nil, icons, fstest.MapFS{})
}

func postJSON(t *testing.T, h http.Handler, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func cookiesFrom(rec *httptest.ResponseRecorder) []*http.Cookie {
	return (&http.Response{Header: rec.Header()}).Cookies()
}

func findCookie(cookies []*http.Cookie, name string) *http.Cookie {
	for _, c := range cookies {
		if c.Name == name {
			return c
		}
	}
	return nil
}

func TestAuthGuardsProtectedRoutes(t *testing.T) {
	h := authTestServer(t).mux

	// Protected route with no session -> 401.
	req := httptest.NewRequest(http.MethodGet, "/api/stacks", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated /api/stacks = %d, want 401", rec.Code)
	}

	// Health stays public.
	req = httptest.NewRequest(http.MethodGet, "/api/health", nil)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("/api/health = %d, want 200", rec.Code)
	}
}

func TestAuthSetupLoginFlow(t *testing.T) {
	s := authTestServer(t)
	h := s.mux
	token := s.setupToken
	if token == "" {
		t.Fatal("expected a first-run setup token")
	}

	// Initially needs setup.
	req := httptest.NewRequest(http.MethodGet, "/api/auth/status", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var st authStatusResponse
	json.Unmarshal(rec.Body.Bytes(), &st)
	if !st.NeedsSetup || st.Authenticated {
		t.Fatalf("initial status = %+v, want needsSetup && !authenticated", st)
	}

	// Setup without the token -> 403.
	rec = postJSON(t, h, "/api/auth/setup", map[string]string{"username": "admin", "password": "hunter2!pass"})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("tokenless setup = %d, want 403", rec.Code)
	}

	// Setup with too-short password (token valid) -> 400.
	rec = postJSON(t, h, "/api/auth/setup", map[string]string{"username": "admin", "password": "short", "token": token})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("weak-password setup = %d, want 400", rec.Code)
	}

	// Proper setup -> 200 + session cookies.
	rec = postJSON(t, h, "/api/auth/setup", map[string]string{"username": "admin", "password": "hunter2!pass", "token": token})
	if rec.Code != http.StatusOK {
		t.Fatalf("setup = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}
	cookies := cookiesFrom(rec)
	sess := findCookie(cookies, sessionCookie)
	csrf := findCookie(cookies, csrfCookie)
	if sess == nil || sess.Value == "" || csrf == nil || csrf.Value == "" {
		t.Fatalf("setup did not set session/csrf cookies: %+v", cookies)
	}
	if !sess.HttpOnly {
		t.Error("session cookie must be HttpOnly")
	}
	if csrf.HttpOnly {
		t.Error("csrf cookie must be readable by JS (not HttpOnly)")
	}

	// Second setup attempt -> 409.
	rec = postJSON(t, h, "/api/auth/setup", map[string]string{"username": "x", "password": "another!pass"})
	if rec.Code != http.StatusConflict {
		t.Fatalf("second setup = %d, want 409", rec.Code)
	}

	// Authenticated request with the session cookie succeeds.
	req = httptest.NewRequest(http.MethodGet, "/api/stacks", nil)
	req.AddCookie(sess)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("authenticated /api/stacks = %d, want 200", rec.Code)
	}

	// Login on a fresh handler-less path: wrong password -> 401.
	rec = postJSON(t, h, "/api/auth/login", map[string]string{"username": "admin", "password": "wrong"})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("bad login = %d, want 401", rec.Code)
	}
	// Correct login -> 200.
	rec = postJSON(t, h, "/api/auth/login", map[string]string{"username": "admin", "password": "hunter2!pass"})
	if rec.Code != http.StatusOK {
		t.Fatalf("good login = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}
}

func TestLoginRateLimited(t *testing.T) {
	s := authTestServer(t)
	h := s.mux
	rec := postJSON(t, h, "/api/auth/setup", map[string]string{"username": "admin", "password": "hunter2!pass", "token": s.setupToken})
	if rec.Code != http.StatusOK {
		t.Fatalf("setup: %d", rec.Code)
	}

	// Threshold bad logins each return 401; the next is blocked with 429.
	for i := 0; i < loginFailThreshold; i++ {
		rec = postJSON(t, h, "/api/auth/login", map[string]string{"username": "admin", "password": "wrong"})
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("bad login #%d = %d, want 401", i+1, rec.Code)
		}
	}
	rec = postJSON(t, h, "/api/auth/login", map[string]string{"username": "admin", "password": "wrong"})
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("blocked login = %d, want 429", rec.Code)
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Error("429 response should carry Retry-After")
	}
}

// TestCSRFCookieSelfHeal is the regression guard for the "update check / save
// stopped working with 'invalid or missing CSRF token'" report: a session that
// outlives the CSRF cookie (redeploy, browser eviction) left the session valid
// but every mutation permanently 403ing, because nothing reissued the CSRF
// cookie. A safe request must now hand back a fresh one.
func TestCSRFCookieSelfHeal(t *testing.T) {
	s := authTestServer(t)
	h := s.mux
	rec := postJSON(t, h, "/api/auth/setup", map[string]string{"username": "admin", "password": "hunter2!pass", "token": s.setupToken})
	if rec.Code != http.StatusOK {
		t.Fatalf("setup failed: %d", rec.Code)
	}
	sess := findCookie(cookiesFrom(rec), sessionCookie)

	// Simulate the broken state: valid session cookie, NO csrf cookie. A
	// mutation with neither csrf cookie nor header must still 403 (we don't
	// trust an absent cookie), and it must NOT be the request that heals.
	body, _ := json.Marshal(map[string]bool{"hidden": true})
	req := httptest.NewRequest(http.MethodPut, "/api/home/foo/bar/visibility", bytes.NewReader(body))
	req.AddCookie(sess)
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("mutation with no csrf cookie = %d, want 403", rec.Code)
	}
	if findCookie(cookiesFrom(rec), csrfCookie) != nil {
		t.Error("an unsafe request must not reissue the csrf cookie (would defeat double-submit)")
	}

	// A safe request (the SPA polls these constantly) reissues the csrf cookie.
	req = httptest.NewRequest(http.MethodGet, "/api/stacks", nil)
	req.AddCookie(sess)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("authenticated GET = %d, want 200", rec.Code)
	}
	healed := findCookie(cookiesFrom(rec), csrfCookie)
	if healed == nil || healed.Value == "" {
		t.Fatal("safe request did not reissue the csrf cookie")
	}
	if healed.HttpOnly {
		t.Error("reissued csrf cookie must be readable by JS (not HttpOnly)")
	}

	// With the healed cookie echoed back, the mutation now works — the SPA has
	// recovered without a re-login.
	req = httptest.NewRequest(http.MethodPut, "/api/home/foo/bar/visibility", bytes.NewReader(body))
	req.AddCookie(sess)
	req.AddCookie(healed)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(csrfHeader, healed.Value)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("mutation after self-heal = %d, want 204 (body=%s)", rec.Code, rec.Body.String())
	}

	// A GET that already has a csrf cookie must not churn it (stable value).
	req = httptest.NewRequest(http.MethodGet, "/api/stacks", nil)
	req.AddCookie(sess)
	req.AddCookie(healed)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if c := findCookie(cookiesFrom(rec), csrfCookie); c != nil && c.Value != healed.Value {
		t.Error("csrf cookie should be left alone when already present")
	}
}

func TestTrustedHeaderAuth(t *testing.T) {
	stacksDir := t.TempDir()
	db, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	cfg := testAuthCfg(config.Config{Port: "5001", StacksDir: stacksDir, LogLevel: slog.LevelError})
	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
	stacksSvc := stacks.NewManager(stacksDir, nil, logger)
	hub := events.NewHub(50 * time.Millisecond)
	host := hoststats.NewSampler(time.Second)
	icons := discovery.NewIconResolver(t.TempDir(), func(context.Context, string) ([]byte, string, bool) {
		return nil, "", false
	})
	// No testAuth wrapper here: this test drives the header/peer itself.
	h := New(context.Background(), cfg, logger, db, stacksSvc, hub, host, nil, icons, fstest.MapFS{})

	// In-CIDR peer (httptest default 192.0.2.1) + header → authenticated.
	req := httptest.NewRequest(http.MethodGet, "/api/stacks", nil)
	req.Header.Set("X-Test-User", "alice")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("in-CIDR trusted header = %d, want 200", rec.Code)
	}

	// Header present but the real TCP peer is OUTSIDE the trusted CIDR — a
	// spoofed forward-auth header from an untrusted client → rejected. This is
	// the security-critical case.
	req = httptest.NewRequest(http.MethodGet, "/api/stacks", nil)
	req.RemoteAddr = "10.0.0.9:5555"
	req.Header.Set("X-Test-User", "alice")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("out-of-CIDR trusted header = %d, want 401", rec.Code)
	}

	// Spoofing X-Forwarded-For must not fool the CIDR test (capturePeer runs
	// before RealIP; the decision uses the genuine peer, still 10.0.0.9).
	req = httptest.NewRequest(http.MethodGet, "/api/stacks", nil)
	req.RemoteAddr = "10.0.0.9:5555"
	req.Header.Set("X-Forwarded-For", "192.0.2.1")
	req.Header.Set("X-Test-User", "alice")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("XFF-spoofed peer = %d, want 401", rec.Code)
	}

	// No header from an in-CIDR peer → falls through to session auth → 401.
	req = httptest.NewRequest(http.MethodGet, "/api/stacks", nil)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("no header = %d, want 401", rec.Code)
	}

	// authStatus reflects proxy auth.
	req = httptest.NewRequest(http.MethodGet, "/api/auth/status", nil)
	req.Header.Set("X-Test-User", "alice")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var st authStatusResponse
	json.Unmarshal(rec.Body.Bytes(), &st)
	if !st.Authenticated || !st.ViaProxy || st.Username != "alice" {
		t.Fatalf("authStatus via proxy = %+v", st)
	}

	// A mutation via trusted header needs no CSRF token (no cookie to forge).
	body, _ := json.Marshal(map[string]bool{"hidden": true})
	req = httptest.NewRequest(http.MethodPut, "/api/home/foo/bar/visibility", bytes.NewReader(body))
	req.Header.Set("X-Test-User", "alice")
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("trusted-header mutation = %d, want 204 (body=%s)", rec.Code, rec.Body.String())
	}
}

func TestCSRFRequiredForMutations(t *testing.T) {
	s := authTestServer(t)
	h := s.mux
	rec := postJSON(t, h, "/api/auth/setup", map[string]string{"username": "admin", "password": "hunter2!pass", "token": s.setupToken})
	if rec.Code != http.StatusOK {
		t.Fatalf("setup failed: %d", rec.Code)
	}
	cookies := cookiesFrom(rec)
	sess := findCookie(cookies, sessionCookie)
	csrf := findCookie(cookies, csrfCookie)

	body, _ := json.Marshal(map[string]bool{"hidden": true})

	// Authenticated PUT without CSRF header -> 403.
	req := httptest.NewRequest(http.MethodPut, "/api/home/foo/bar/visibility", bytes.NewReader(body))
	req.AddCookie(sess)
	req.AddCookie(csrf)
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("mutation without CSRF header = %d, want 403", rec.Code)
	}

	// With the matching CSRF header it passes the guard (503: nil db... actually
	// db is present here, so it should reach the handler and succeed with 204).
	req = httptest.NewRequest(http.MethodPut, "/api/home/foo/bar/visibility", bytes.NewReader(body))
	req.AddCookie(sess)
	req.AddCookie(csrf)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(csrfHeader, csrf.Value)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("mutation with CSRF header = %d, want 204 (body=%s)", rec.Code, rec.Body.String())
	}
}
