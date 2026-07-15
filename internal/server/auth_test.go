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

// authTestHandler builds a handler with a real store and auth ENABLED.
func authTestHandler(t *testing.T) http.Handler {
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
	return New(context.Background(), cfg, logger, db, stacksSvc, hub, host, nil, icons, fstest.MapFS{})
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
	h := authTestHandler(t)

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
	h := authTestHandler(t)

	// Initially needs setup.
	req := httptest.NewRequest(http.MethodGet, "/api/auth/status", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var st authStatusResponse
	json.Unmarshal(rec.Body.Bytes(), &st)
	if !st.NeedsSetup || st.Authenticated {
		t.Fatalf("initial status = %+v, want needsSetup && !authenticated", st)
	}

	// Setup with too-short password -> 400.
	rec = postJSON(t, h, "/api/auth/setup", map[string]string{"username": "admin", "password": "short"})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("weak-password setup = %d, want 400", rec.Code)
	}

	// Proper setup -> 200 + session cookies.
	rec = postJSON(t, h, "/api/auth/setup", map[string]string{"username": "admin", "password": "hunter2!pass"})
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
	h := authTestHandler(t)
	rec := postJSON(t, h, "/api/auth/setup", map[string]string{"username": "admin", "password": "hunter2!pass"})
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
