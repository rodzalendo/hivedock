package server

import (
	"context"
	"encoding/json"
	"errors"
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
	"github.com/rogalinski/hivedock/internal/registry"
	"github.com/rogalinski/hivedock/internal/stacks"
	"github.com/rogalinski/hivedock/internal/store"
)

// newTestAPI builds an *api (with the trusted-header test auth config) so tests
// can both inspect internal state and drive routes via testAuth(a.mux). db may
// be nil; docker is always nil (no daemon in tests).
func newTestAPI(t *testing.T, db *store.Store) *api {
	t.Helper()
	stacksDir := t.TempDir()
	cfg := testAuthCfg(config.Config{Port: "5001", StacksDir: stacksDir, LogLevel: slog.LevelError})
	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
	stacksSvc := stacks.NewManager(stacksDir, nil, logger)
	hub := events.NewHub(50 * time.Millisecond)
	host := hoststats.NewSampler(time.Second)
	icons := discovery.NewIconResolver(t.TempDir(), func(context.Context, string) ([]byte, string, bool) {
		return nil, "", false
	})
	return newServer(context.Background(), cfg, logger, db, stacksSvc, hub, host, nil, icons, fstest.MapFS{})
}

func testStore(t *testing.T) *store.Store {
	t.Helper()
	db, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// --- fakes for the digest resolver + signature verifier ---

type fakeSelfReg struct {
	digest string
	err    error
}

func (f fakeSelfReg) Digest(context.Context, registry.Ref) (string, error) { return f.digest, f.err }

type fakeVerifier struct{ err error }

func (f fakeVerifier) Verify(context.Context, string) error { return f.err }

func TestAppUpdateDevBuildNotCheckable(t *testing.T) {
	h := testHandler(t, fstest.MapFS{}) // version is "dev" in tests
	req := httptest.NewRequest(http.MethodGet, "/api/app/update", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var got appUpdateResponse
	json.Unmarshal(rec.Body.Bytes(), &got)
	if got.Checkable || got.HasUpdate {
		t.Errorf("dev build should not be checkable: %+v", got)
	}
	if got.Current != "dev" {
		t.Errorf("current = %q, want dev", got.Current)
	}
}

// evaluateCandidate is the security-critical gate: only a resolvable AND
// signature-verified candidate may be offered; every other outcome is the
// VerifyFailed alert state, never a silent skip and never an offer.
func TestEvaluateCandidate(t *testing.T) {
	tests := []struct {
		name             string
		reg              selfRegistry
		verify           imageVerifier
		wantOffer        bool
		wantDigest       string
		wantVerifyFailed bool
	}{
		{
			name:       "verified",
			reg:        fakeSelfReg{digest: "sha256:abc123"},
			verify:     fakeVerifier{err: nil},
			wantOffer:  true,
			wantDigest: "sha256:abc123",
		},
		{
			name:             "signature invalid",
			reg:              fakeSelfReg{digest: "sha256:abc123"},
			verify:           fakeVerifier{err: errors.New("no matching signatures")},
			wantVerifyFailed: true,
		},
		{
			name:             "digest resolve fails",
			reg:              fakeSelfReg{err: errors.New("registry unreachable")},
			verify:           fakeVerifier{err: nil}, // must not even be consulted
			wantVerifyFailed: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := newTestAPI(t, nil)
			a.selfReg = tt.reg
			a.verify = tt.verify
			offer, digest, verifyFailed := a.evaluateCandidate(context.Background(), "9.9.9")
			if offer != tt.wantOffer {
				t.Errorf("offer = %v, want %v", offer, tt.wantOffer)
			}
			if digest != tt.wantDigest {
				t.Errorf("digest = %q, want %q", digest, tt.wantDigest)
			}
			if verifyFailed != tt.wantVerifyFailed {
				t.Errorf("verifyFailed = %v, want %v", verifyFailed, tt.wantVerifyFailed)
			}
		})
	}
}

func TestAppUpdateOffModeSkipsCheck(t *testing.T) {
	db := testStore(t)
	if err := db.SetSetting(settingAppUpdateMode, updateModeOff); err != nil {
		t.Fatal(err)
	}
	a := newTestAPI(t, db)
	// A verifier that would panic proves the check is skipped entirely.
	a.selfReg = fakeSelfReg{err: errors.New("must not be called")}

	req := httptest.NewRequest(http.MethodGet, "/api/app/update", nil)
	rec := httptest.NewRecorder()
	testAuth(a.mux).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var got appUpdateResponse
	json.Unmarshal(rec.Body.Bytes(), &got)
	if got.Mode != updateModeOff || got.Checkable || got.HasUpdate {
		t.Errorf("off mode should not check: %+v", got)
	}
}

func TestSelfUpdateRefusedWhenNotFull(t *testing.T) {
	for _, mode := range []string{updateModeCheckOnly, updateModeOff} {
		t.Run(mode, func(t *testing.T) {
			db := testStore(t)
			if err := db.SetSetting(settingAppUpdateMode, mode); err != nil {
				t.Fatal(err)
			}
			a := newTestAPI(t, db)
			req := httptest.NewRequest(http.MethodPost, "/api/app/update", nil)
			rec := httptest.NewRecorder()
			testAuth(a.mux).ServeHTTP(rec, req)
			if rec.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want 403 (body=%s)", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestSelfUpdateDevBuildRefused(t *testing.T) {
	// version is "dev" in tests: full mode passes the mode gate, then the
	// dev/edge guard refuses with 409 before any compose/daemon work — never a
	// hang or a stray helper container.
	h := testHandler(t, fstest.MapFS{})
	req := httptest.NewRequest(http.MethodPost, "/api/app/update", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409 (body=%s)", rec.Code, rec.Body.String())
	}
}

func TestApplyUpdateValidatesOpts(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
	tests := []struct {
		name string
		o    ApplyUpdateOpts
	}{
		{"missing digest", ApplyUpdateOpts{ProjectDir: "/w", ComposeFile: "/w/c.yaml", ImageRef: "img"}},
		{"missing image ref", ApplyUpdateOpts{Digest: "sha256:abc", ProjectDir: "/w", ComposeFile: "/w/c.yaml"}},
		{"digest not a manifest digest", ApplyUpdateOpts{Digest: "latest", ProjectDir: "/w", ComposeFile: "/w/c.yaml", ImageRef: "img"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := ApplyUpdate(context.Background(), logger, tt.o); err == nil {
				t.Errorf("ApplyUpdate(%+v) = nil, want error", tt.o)
			}
		})
	}
}
