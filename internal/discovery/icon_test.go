package discovery

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
)

func TestIconResolverFetchesCachesAndAvoidsRefetch(t *testing.T) {
	dir := t.TempDir()
	var calls int32
	fetch := func(_ context.Context, url string) ([]byte, string, bool) {
		atomic.AddInt32(&calls, 1)
		if url == cdnBase+"/svg/jellyfin.svg" {
			return []byte("<svg/>"), "image/svg+xml", true
		}
		return nil, "", false
	}
	ir := NewIconResolver(dir, fetch)

	data, ct, ok := ir.Icon(context.Background(), "jellyfin")
	if !ok || string(data) != "<svg/>" || ct != "image/svg+xml" {
		t.Fatalf("first fetch: ok=%v ct=%q data=%q", ok, ct, data)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("expected 1 CDN call, got %d", got)
	}

	// Second call hits the on-disk cache — no additional fetch.
	if _, _, ok := ir.Icon(context.Background(), "jellyfin"); !ok {
		t.Fatal("second call should serve from cache")
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("cache should prevent refetch, got %d calls", got)
	}
}

func TestIconResolverAlias(t *testing.T) {
	dir := t.TempDir()
	var requested string
	fetch := func(_ context.Context, url string) ([]byte, string, bool) {
		requested = url
		return nil, "", false
	}
	ir := NewIconResolver(dir, fetch)
	ir.Icon(context.Background(), "ubuntu") // aliased to ubuntu-linux
	if !strings.Contains(requested, "/ubuntu-linux.") {
		t.Fatalf("alias not applied; requested %q", requested)
	}
}

func TestIconResolverMissNegativeCache(t *testing.T) {
	dir := t.TempDir()
	var calls int32
	fetch := func(_ context.Context, _ string) ([]byte, string, bool) {
		atomic.AddInt32(&calls, 1)
		return nil, "", false
	}
	ir := NewIconResolver(dir, fetch)

	if _, _, ok := ir.Icon(context.Background(), "nonexistent"); ok {
		t.Fatal("expected miss")
	}
	// svg + png attempts = 2 calls.
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Fatalf("expected 2 CDN attempts, got %d", got)
	}
	// Negative cache: a second lookup shouldn't hit the CDN again.
	ir.Icon(context.Background(), "nonexistent")
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Fatalf("negative cache should prevent refetch, got %d calls", got)
	}
}

func TestIconResolverRejectsUnsafeSlug(t *testing.T) {
	ir := NewIconResolver(t.TempDir(), func(context.Context, string) ([]byte, string, bool) {
		t.Fatal("must not fetch for an unsafe slug")
		return nil, "", false
	})
	if _, _, ok := ir.Icon(context.Background(), "../../etc/passwd"); ok {
		t.Fatal("path-traversal slug must be rejected")
	}
}
