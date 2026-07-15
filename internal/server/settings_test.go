package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

// dbHandler builds a handler backed by a real store, authenticated via the
// trusted-header test path (see testAuthCfg/testAuth in server_test.go).
func dbHandler(t *testing.T, cfg config.Config) http.Handler {
	t.Helper()
	db, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	cfg = testAuthCfg(cfg)
	cfg.LogLevel = slog.LevelError
	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
	stacksSvc := stacks.NewManager(cfg.StacksDir, nil, logger)
	hub := events.NewHub(50 * time.Millisecond)
	host := hoststats.NewSampler(time.Second)
	icons := discovery.NewIconResolver(t.TempDir(), func(context.Context, string) ([]byte, string, bool) {
		return nil, "", false
	})
	return testAuth(New(context.Background(), cfg, logger, db, stacksSvc, hub, host, nil, icons, fstest.MapFS{}))
}

func TestGitAutoCommitFlow(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	stackDir := filepath.Join(dir, "web")
	if err := os.MkdirAll(stackDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stackDir, "compose.yaml"), []byte("services:\n  web:\n    image: nginx\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stackDir, ".env"), []byte("A=1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	h := dbHandler(t, config.Config{StacksDir: dir})

	send := func(method, url, body string) *httptest.ResponseRecorder {
		var r io.Reader
		if body != "" {
			r = strings.NewReader(body)
		}
		req := httptest.NewRequest(method, url, r)
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		return rr
	}

	// Enabling auto-commit before the repo exists is refused.
	if rec := send(http.MethodPut, "/api/settings", `{"gitAutoCommit":true}`); rec.Code != http.StatusConflict {
		t.Fatalf("enable before init = %d, want 409 (body=%s)", rec.Code, rec.Body.String())
	}
	// Initialize the repo (commits the existing files).
	if rec := send(http.MethodPost, "/api/settings/git-init", ""); rec.Code != http.StatusOK {
		t.Fatalf("git-init = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}
	// Now enabling succeeds.
	if rec := send(http.MethodPut, "/api/settings", `{"gitAutoCommit":true}`); rec.Code != http.StatusOK {
		t.Fatalf("enable after init = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}

	// Load the .env → base hash, then save → auto-commit fires.
	rec := send(http.MethodGet, "/api/stacks/web/env", "")
	var env envFileResponse
	json.Unmarshal(rec.Body.Bytes(), &env)
	if rec := send(http.MethodPut, "/api/stacks/web/env", `{"content":"A=2\n","baseSha256":"`+env.Sha256+`"}`); rec.Code != http.StatusOK {
		t.Fatalf("save .env = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}

	out, err := exec.Command("git", "-C", dir, "log", "--format=%s").Output()
	if err != nil {
		t.Fatalf("git log: %v", err)
	}
	if !strings.Contains(string(out), "hivedock: save web .env") {
		t.Errorf("no auto-commit for the save; log:\n%s", out)
	}
	// The .env change is committed, so the worktree is clean afterward.
	if st, _ := exec.Command("git", "-C", dir, "status", "--porcelain").Output(); len(bytes.TrimSpace(st)) != 0 {
		t.Errorf("worktree dirty after auto-commit:\n%s", st)
	}
}

func TestSettingsGetReflectsConfig(t *testing.T) {
	h := dbHandler(t, config.Config{StacksDir: "/opt/stacks", CheckInterval: 6 * time.Hour})
	req := httptest.NewRequest(http.MethodGet, "/api/settings", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var got settingsResponse
	json.Unmarshal(rec.Body.Bytes(), &got)
	if got.StacksDir != "/opt/stacks" {
		t.Errorf("stacksDir = %q", got.StacksDir)
	}
	// Durations are tidied for display: 6h0m0s -> 6h.
	if got.CheckInterval != "6h" {
		t.Errorf("checkInterval = %q, want 6h", got.CheckInterval)
	}
}

func TestSettingsCheckIntervalRoundTrip(t *testing.T) {
	h := dbHandler(t, config.Config{StacksDir: "/opt/stacks", CheckInterval: 30 * time.Minute})

	// Save a new interval.
	body, _ := json.Marshal(map[string]string{"checkInterval": "6h"})
	req := httptest.NewRequest(http.MethodPut, "/api/settings", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("put status = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}
	var got settingsResponse
	json.Unmarshal(rec.Body.Bytes(), &got)
	if got.CheckInterval != "6h" {
		t.Errorf("checkInterval = %q, want 6h", got.CheckInterval)
	}

	// Reject a too-short interval.
	body, _ = json.Marshal(map[string]string{"checkInterval": "1m"})
	req = httptest.NewRequest(http.MethodPut, "/api/settings", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("too-short interval status = %d, want 400", rec.Code)
	}
}
