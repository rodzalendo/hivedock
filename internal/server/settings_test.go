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

// dbHandler builds an auth-disabled handler backed by a real store.
func dbHandler(t *testing.T, cfg config.Config) http.Handler {
	t.Helper()
	db, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	cfg.AuthDisabled = true
	cfg.LogLevel = slog.LevelError
	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
	stacksSvc := stacks.NewManager(cfg.StacksDir, nil, logger)
	hub := events.NewHub(50 * time.Millisecond)
	host := hoststats.NewSampler(time.Second)
	icons := discovery.NewIconResolver(t.TempDir(), func(context.Context, string) ([]byte, string, bool) {
		return nil, "", false
	})
	return New(context.Background(), cfg, logger, db, stacksSvc, hub, host, nil, icons, fstest.MapFS{})
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
