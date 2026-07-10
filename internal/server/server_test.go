package server

import (
	"encoding/json"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	"github.com/rogalinski/hivedock/internal/config"
)

func testHandler(t *testing.T, dist fs.FS) http.Handler {
	t.Helper()
	cfg := config.Config{Port: "5001", StacksDir: "/opt/stacks-test", AuthDisabled: true, LogLevel: slog.LevelError}
	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
	// db is unused by the routes under test; keep it nil to avoid touching disk.
	return New(cfg, logger, nil, dist)
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
	if got.StacksDir != "/opt/stacks-test" {
		t.Errorf("stacksDir = %q, want /opt/stacks-test", got.StacksDir)
	}
	if !got.AuthDisabled {
		t.Errorf("authDisabled = false, want true")
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
