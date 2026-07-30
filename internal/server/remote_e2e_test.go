package server

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/rogalinski/hivedock/internal/agent"
	"github.com/rogalinski/hivedock/internal/config"
	"github.com/rogalinski/hivedock/internal/discovery"
	"github.com/rogalinski/hivedock/internal/events"
	"github.com/rogalinski/hivedock/internal/hostops"
	"github.com/rogalinski/hivedock/internal/hoststats"
	"github.com/rogalinski/hivedock/internal/stacks"
)

func composeAvailable() bool { return exec.Command("docker", "compose", "version").Run() == nil }

// TestRemoteBackendE2E drives the full remote stack-management path end to end: a
// real `hivedock agent` dials a real manager over a WebSocket, and the manager's
// remoteBackend runs create → getCompose → putCompose (stale→409) → updateService
// (preview then apply) → env → delete against the agent's temp STACKS_DIR —
// asserting the typed errors and results match a local backend (docs/MULTIHOST.md).
func TestRemoteBackendE2E(t *testing.T) {
	if !composeAvailable() {
		t.Skip("docker compose CLI not available (putCompose validates)")
	}
	agentStacks := t.TempDir()
	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))

	cfg := testAuthCfg(config.Config{Port: "5001", StacksDir: t.TempDir(), AgentToken: "tok", LogLevel: slog.LevelError})
	stacksSvc := stacks.NewManager(cfg.StacksDir, nil, logger)
	hub := events.NewHub(50 * time.Millisecond)
	host := hoststats.NewSampler(time.Second)
	icons := discovery.NewIconResolver(t.TempDir(), func(context.Context, string) ([]byte, string, bool) {
		return nil, "", false
	})
	api := newServer(context.Background(), cfg, logger, nil, stacksSvc, hub, host, nil, icons, fstest.MapFS{})
	srv := httptest.NewServer(api.mux)
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		_ = agent.Run(ctx, agent.Options{
			ManagerURL: srv.URL, Token: "tok", Name: "remote",
			Version: "test", Logger: logger, StacksDir: agentStacks,
		})
	}()

	var h *hostConn
	for i := 0; i < 300 && h == nil; i++ {
		h = api.hosts.get("remote")
		time.Sleep(10 * time.Millisecond)
	}
	if h == nil {
		t.Fatal("agent never registered with the manager")
	}
	be := newRemoteBackend(h)
	cctx := context.Background()

	// create + duplicate → ErrExists
	if _, err := be.CreateStack(cctx, "web", ""); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := be.CreateStack(cctx, "web", ""); !errors.Is(err, hostops.ErrExists) {
		t.Errorf("duplicate create = %v, want ErrExists", err)
	}
	if _, err := be.CreateStack(cctx, "Bad Name", ""); !errors.Is(err, hostops.ErrInvalidName) {
		t.Errorf("bad name = %v, want ErrInvalidName", err)
	}

	cf, err := be.GetCompose(cctx, "web")
	if err != nil {
		t.Fatalf("getCompose: %v", err)
	}

	// A stale base surfaces the conflict with the current bytes (the 409 reconcile
	// payload must survive the wire).
	next := []byte("services:\n  web:\n    image: nginx:alpine\n")
	_, err = be.PutCompose(cctx, "web", next, "deadbeef")
	var conflict *hostops.ConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("stale putCompose = %v, want *ConflictError", err)
	}
	if conflict.Sha256 == "" || !strings.Contains(conflict.Content, "services:") {
		t.Errorf("conflict payload lost across the wire: %+v", conflict)
	}
	saved, err := be.PutCompose(cctx, "web", next, cf.Sha256)
	if err != nil {
		t.Fatalf("putCompose: %v", err)
	}

	prev, err := be.UpdateService(cctx, hostops.UpdateServiceReq{Name: "web", Service: "web", Tag: "1.27"})
	if err != nil {
		t.Fatalf("updateService preview: %v", err)
	}
	if !prev.Preview || prev.Diff == "" {
		t.Errorf("expected a preview diff, got %+v", prev)
	}
	if _, err := be.UpdateService(cctx, hostops.UpdateServiceReq{Name: "web", Service: "web", Tag: "1.27", Confirm: true, BaseSha256: saved.Sha256}); err != nil {
		t.Fatalf("updateService apply: %v", err)
	}
	if cf2, _ := be.GetCompose(cctx, "web"); !strings.Contains(cf2.Content, "nginx:1.27") {
		t.Errorf("remote tag not applied:\n%s", cf2.Content)
	}

	// env round-trip.
	ef, err := be.GetEnv(cctx, "web")
	if err != nil {
		t.Fatalf("getEnv: %v", err)
	}
	if ef.Exists {
		t.Error("env should not exist yet")
	}
	if _, err := be.PutEnv(cctx, "web", []byte("TZ=UTC\n"), ef.Sha256); err != nil {
		t.Fatalf("putEnv: %v", err)
	}

	// delete → gone.
	if err := be.DeleteStack(cctx, "web", false); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := be.GetStack(cctx, "web"); !errors.Is(err, hostops.ErrNotFound) {
		t.Errorf("deleted stack still resolves: %v", err)
	}
}

// TestRemoteBackendReadOnly asserts a read-only agent refuses mutations with the
// typed error (→ 403) while reads still work.
func TestRemoteBackendReadOnly(t *testing.T) {
	agentStacks := t.TempDir()
	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))

	cfg := testAuthCfg(config.Config{Port: "5001", StacksDir: t.TempDir(), AgentToken: "tok", LogLevel: slog.LevelError})
	stacksSvc := stacks.NewManager(cfg.StacksDir, nil, logger)
	hub := events.NewHub(50 * time.Millisecond)
	host := hoststats.NewSampler(time.Second)
	icons := discovery.NewIconResolver(t.TempDir(), func(context.Context, string) ([]byte, string, bool) {
		return nil, "", false
	})
	api := newServer(context.Background(), cfg, logger, nil, stacksSvc, hub, host, nil, icons, fstest.MapFS{})
	srv := httptest.NewServer(api.mux)
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		_ = agent.Run(ctx, agent.Options{
			ManagerURL: srv.URL, Token: "tok", Name: "ro",
			Version: "test", Logger: logger, StacksDir: agentStacks,
			ReadOnlyReason: "STACKS_DIR bind mismatch",
		})
	}()

	var h *hostConn
	for i := 0; i < 300 && h == nil; i++ {
		h = api.hosts.get("ro")
		time.Sleep(10 * time.Millisecond)
	}
	if h == nil {
		t.Fatal("agent never registered")
	}
	be := newRemoteBackend(h)

	// A read works.
	if _, err := be.ListStacks(context.Background()); err != nil {
		t.Fatalf("listStacks on a read-only agent should work: %v", err)
	}
	// A mutation is refused with the read-only error.
	if _, err := be.CreateStack(context.Background(), "web", ""); !errors.Is(err, hostops.ErrReadOnly) {
		t.Errorf("create on a read-only agent = %v, want ErrReadOnly", err)
	}
}
