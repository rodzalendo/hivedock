package stacks

import (
	"context"
	"os"
	"path/filepath"
	"sync"

	"github.com/rogalinski/hivedock/internal/compose"
)

// HashFunc computes per-service config hashes for a compose file. Injectable so
// the drift checker is testable without a real `docker compose`.
type HashFunc func(ctx context.Context, composeFile, projectDir string) (map[string]string, error)

// driftChecker computes and caches on-disk compose config hashes. Recomputing
// shells out to `docker compose config --hash`, so results are cached and only
// refreshed when the compose file or its .env changes (mtime-keyed). This keeps
// List cheap even though it's called on every change/poll.
type driftChecker struct {
	hashFn HashFunc

	mu    sync.Mutex
	cache map[string]driftEntry // keyed by compose file path
}

type driftEntry struct {
	stamp  int64             // newest mtime (unix nanos) of composeFile + .env
	hashes map[string]string // service -> hash ("" entry means "compute failed")
	ok     bool
}

func newDriftChecker(hashFn HashFunc) *driftChecker {
	if hashFn == nil {
		hashFn = compose.ConfigHashes
	}
	return &driftChecker{hashFn: hashFn, cache: map[string]driftEntry{}}
}

// hashesFor returns the on-disk per-service hashes for a stack, using the cache
// when the files are unchanged. ok=false means hashes couldn't be computed
// (e.g. compose CLI missing) — callers should then not claim drift.
func (d *driftChecker) hashesFor(ctx context.Context, s ScannedStack) (map[string]string, bool) {
	if s.ComposeFile == "" {
		return nil, false
	}
	stamp := newestMtime(s.ComposeFile, filepath.Join(s.Dir, ".env"))

	d.mu.Lock()
	if e, found := d.cache[s.ComposeFile]; found && e.stamp == stamp {
		d.mu.Unlock()
		return e.hashes, e.ok
	}
	d.mu.Unlock()

	hashes, err := d.hashFn(ctx, s.ComposeFile, s.Dir)
	entry := driftEntry{stamp: stamp, hashes: hashes, ok: err == nil}

	d.mu.Lock()
	d.cache[s.ComposeFile] = entry
	d.mu.Unlock()

	return hashes, entry.ok
}

func newestMtime(paths ...string) int64 {
	var newest int64
	for _, p := range paths {
		if info, err := os.Stat(p); err == nil {
			if m := info.ModTime().UnixNano(); m > newest {
				newest = m
			}
		}
	}
	return newest
}
