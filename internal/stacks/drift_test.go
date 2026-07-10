package stacks

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rogalinski/hivedock/internal/docker"
)

func TestMergeFlagsDrift(t *testing.T) {
	scanned := []ScannedStack{
		{Name: "app", Project: "app", Services: map[string]ScannedSvc{
			"web": {Image: "nginx"},
			"api": {Image: "api"},
		}},
	}
	containers := []docker.Container{
		// web: running config-hash matches the file -> no drift
		{ID: "web000000000", Name: "app-web-1", State: "running", Project: "app", Service: "web", ConfigHash: "HASH_WEB"},
		// api: running config-hash differs from the file -> drift
		{ID: "api000000000", Name: "app-api-1", State: "running", Project: "app", Service: "api", ConfigHash: "OLD_HASH"},
	}
	fileHashes := map[string]map[string]string{
		"app": {"web": "HASH_WEB", "api": "NEW_HASH"},
	}

	stacks := Merge(scanned, containers, true, fileHashes)
	app, ok := findStack(stacks, "app")
	if !ok {
		t.Fatal("app stack missing")
	}
	if !app.Drifted {
		t.Fatal("expected stack Drifted=true")
	}
	for _, svc := range app.Services {
		switch svc.Name {
		case "web":
			if svc.Drifted {
				t.Error("web should not be drifted")
			}
		case "api":
			if !svc.Drifted {
				t.Error("api should be drifted")
			}
		}
	}
}

func TestMergeNoDriftWhenContainerHashMissing(t *testing.T) {
	// A container without a config-hash label (e.g. not compose-created in the
	// usual way) must not be falsely flagged as drifted.
	scanned := []ScannedStack{
		{Name: "app", Project: "app", Services: map[string]ScannedSvc{"web": {Image: "nginx"}}},
	}
	containers := []docker.Container{
		{ID: "web000000000", Name: "app-web-1", State: "running", Project: "app", Service: "web", ConfigHash: ""},
	}
	fileHashes := map[string]map[string]string{"app": {"web": "SOMEHASH"}}

	stacks := Merge(scanned, containers, true, fileHashes)
	app, _ := findStack(stacks, "app")
	if app.Drifted {
		t.Fatal("must not flag drift when container has no config-hash")
	}
}

func TestDriftCheckerCachesByMtime(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "compose.yaml")
	if err := os.WriteFile(file, []byte("services: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	stack := ScannedStack{Name: "app", Dir: dir, ComposeFile: file}

	calls := 0
	fn := func(_ context.Context, _, _ string) (map[string]string, error) {
		calls++
		return map[string]string{"web": "H"}, nil
	}
	dc := newDriftChecker(fn)

	// First call computes; second (unchanged file) is cached.
	dc.hashesFor(context.Background(), stack)
	dc.hashesFor(context.Background(), stack)
	if calls != 1 {
		t.Fatalf("expected 1 compute (cached), got %d", calls)
	}

	// Touch the file with a newer mtime -> cache invalidated, recompute.
	newer := time.Unix(0, newestMtime(file)).Add(2 * time.Second)
	if err := os.Chtimes(file, newer, newer); err != nil {
		t.Fatal(err)
	}
	dc.hashesFor(context.Background(), stack)
	if calls != 2 {
		t.Fatalf("expected recompute after mtime change, got %d calls", calls)
	}
}
