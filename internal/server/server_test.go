package server

import (
	"context"
	"encoding/json"
	"io"
	"io/fs"
	"log/slog"
	"net"
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
)

// testAuthCfg configures trusted-header auth so tests authenticate without a
// database or session: httptest requests carry RemoteAddr 192.0.2.1, inside the
// CIDR below, and testAuth() injects the header. This exercises the real
// forward-auth path — the supported replacement for the removed AUTH_DISABLED.
func testAuthCfg(cfg config.Config) config.Config {
	_, cidr, _ := net.ParseCIDR("192.0.2.0/24")
	cfg.TrustedHeader = "X-Test-User"
	cfg.TrustedProxyCIDRs = []*net.IPNet{cidr}
	return cfg
}

// testAuth wraps a handler to inject the trusted-header on every request, so the
// existing (pre-auth) tests keep passing unchanged.
func testAuth(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Header.Set("X-Test-User", "admin")
		h.ServeHTTP(w, r)
	})
}

func testHandler(t *testing.T, dist fs.FS) http.Handler {
	t.Helper()
	stacksDir := t.TempDir() // empty dir -> empty stacks list, no daemon
	cfg := testAuthCfg(config.Config{Port: "5001", StacksDir: stacksDir, LogLevel: slog.LevelError})
	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
	stacksSvc := stacks.NewManager(stacksDir, nil, logger)
	hub := events.NewHub(50 * time.Millisecond)
	host := hoststats.NewSampler(time.Second)
	icons := discovery.NewIconResolver(t.TempDir(), func(context.Context, string) ([]byte, string, bool) {
		return nil, "", false
	})
	// db and docker are unused by the routes under test; keep them nil.
	return testAuth(New(context.Background(), cfg, logger, nil, stacksSvc, hub, host, nil, icons, dist))
}

func TestHealth(t *testing.T) {
	h := testHandler(t, fstest.MapFS{})
	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var got healthResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v (body=%q)", err, rec.Body.String())
	}
	if got.Status != "ok" {
		t.Errorf("status = %q, want ok", got.Status)
	}
	if got.StacksDir == "" {
		t.Errorf("stacksDir is empty")
	}
}

func TestListStacksEmpty(t *testing.T) {
	h := testHandler(t, fstest.MapFS{})
	req := httptest.NewRequest(http.MethodGet, "/api/stacks", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	// Empty dir with no daemon must return `[]`, never `null`.
	if body := rec.Body.String(); body != "[]\n" && body != "[]" {
		t.Errorf("body = %q, want []", body)
	}
}

func TestGetStackNotFound(t *testing.T) {
	h := testHandler(t, fstest.MapFS{})
	req := httptest.NewRequest(http.MethodGet, "/api/stacks/nope", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestSPAServesIndex(t *testing.T) {
	dist := fstest.MapFS{
		"index.html": {Data: []byte("<!doctype html><title>Hivedock</title>")},
	}
	h := testHandler(t, dist)

	// A client-side route with no matching file must fall back to index.html.
	req := httptest.NewRequest(http.MethodGet, "/stacks/whoami", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct == "" {
		t.Errorf("missing Content-Type on SPA response")
	}
	if body := rec.Body.String(); body == "" {
		t.Errorf("empty SPA body")
	}
}

func TestSPANotBuiltReturnsHint(t *testing.T) {
	h := testHandler(t, fstest.MapFS{}) // no index.html
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}
