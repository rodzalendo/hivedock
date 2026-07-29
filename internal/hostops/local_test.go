package hostops

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rogalinski/hivedock/internal/compose"
	"github.com/rogalinski/hivedock/internal/stacks"
)

func composeAvailable() bool {
	return exec.Command("docker", "compose", "version").Run() == nil
}

// TestLocalBackend drives the full file-op surface against a temp STACKS_DIR with
// no docker daemon: create → read → optimistic-lock save → updateService
// preview/apply → env → rename → delete.
func TestLocalBackend(t *testing.T) {
	if !composeAvailable() {
		t.Skip("docker compose CLI not available")
	}
	dir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	be := NewLocal(dir, stacks.NewManager(dir, nil, logger), compose.NewRunner(), nil, nil, logger)
	ctx := context.Background()

	if _, err := be.CreateStack(ctx, "web", ""); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := be.CreateStack(ctx, "web", ""); !errors.Is(err, ErrExists) {
		t.Errorf("duplicate create = %v, want ErrExists", err)
	}
	if _, err := be.CreateStack(ctx, "Bad Name", ""); !errors.Is(err, ErrInvalidName) {
		t.Errorf("bad name = %v, want ErrInvalidName", err)
	}

	cf, err := be.GetCompose(ctx, "web")
	if err != nil {
		t.Fatalf("getCompose: %v", err)
	}
	if !strings.Contains(cf.Content, "services:") {
		t.Errorf("compose missing services block")
	}

	next := []byte("services:\n  web:\n    image: nginx:alpine\n")
	if _, err := be.PutCompose(ctx, "web", next, "deadbeef"); err == nil {
		t.Fatal("putCompose with a stale base should conflict")
	} else {
		var conflict *ConflictError
		if !errors.As(err, &conflict) {
			t.Fatalf("putCompose stale = %v, want *ConflictError", err)
		}
	}
	saved, err := be.PutCompose(ctx, "web", next, cf.Sha256)
	if err != nil {
		t.Fatalf("putCompose: %v", err)
	}

	prev, err := be.UpdateService(ctx, UpdateServiceReq{Name: "web", Service: "web", Tag: "1.27", Confirm: false})
	if err != nil {
		t.Fatalf("updateService preview: %v", err)
	}
	if !prev.Preview || prev.Diff == "" {
		t.Errorf("expected a preview diff, got %+v", prev)
	}
	if _, err := be.UpdateService(ctx, UpdateServiceReq{Name: "web", Service: "web", Tag: "1.27", Confirm: true, BaseSha256: saved.Sha256}); err != nil {
		t.Fatalf("updateService apply: %v", err)
	}
	if cf2, _ := be.GetCompose(ctx, "web"); !strings.Contains(cf2.Content, "nginx:1.27") {
		t.Errorf("tag not applied:\n%s", cf2.Content)
	}
	// A digest-pinned image is refused with a typed error.
	_ = os.WriteFile(filepath.Join(dir, "web", "compose.yaml"), []byte("services:\n  web:\n    image: nginx@sha256:"+strings.Repeat("a", 64)+"\n"), 0o644)
	if _, err := be.UpdateService(ctx, UpdateServiceReq{Name: "web", Service: "web", Tag: "1.27", Confirm: false}); !errors.Is(err, compose.ErrDigestPinned) {
		t.Errorf("digest-pinned update = %v, want ErrDigestPinned", err)
	}

	ef, err := be.GetEnv(ctx, "web")
	if err != nil {
		t.Fatalf("getEnv: %v", err)
	}
	if ef.Exists {
		t.Error("env should not exist yet")
	}
	if _, err := be.PutEnv(ctx, "web", []byte("TZ=UTC\n"), ef.Sha256); err != nil {
		t.Fatalf("putEnv: %v", err)
	}
	if ef2, _ := be.GetEnv(ctx, "web"); !ef2.Exists || !strings.Contains(ef2.Content, "TZ=UTC") {
		t.Errorf("env not saved: %+v", ef2)
	}

	if _, err := be.RenameStack(ctx, "web", "web2"); err != nil {
		t.Fatalf("rename: %v", err)
	}
	if _, err := be.GetStack(ctx, "web"); !errors.Is(err, ErrNotFound) {
		t.Errorf("old name still resolves: %v", err)
	}
	if _, err := be.GetStack(ctx, "web2"); err != nil {
		t.Errorf("new name missing: %v", err)
	}

	if err := be.DeleteStack(ctx, "web2", false); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "web2")); !os.IsNotExist(err) {
		t.Error("stack dir not removed after delete")
	}
}
