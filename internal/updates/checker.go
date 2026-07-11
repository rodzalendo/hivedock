// Package updates checks the images used by managed stacks for newer versions.
// It combines the registry client (tags + digests) with the semver candidate
// engine: version-like tags take the semver path, mutable tags (latest, …) take
// the digest path. Results are cached in SQLite by the caller.
package updates

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/rogalinski/hivedock/internal/registry"
)

// Kinds of check outcome (stored and shown in the UI).
const (
	KindSemver      = "semver"      // a newer version tag exists
	KindDigest      = "digest"      // a mutable tag's digest changed (or remote-only)
	KindUpToDate    = "uptodate"    // no update found
	KindError       = "error"       // the check failed (network/registry)
	KindUnsupported = "unsupported" // image ref can't be parsed/checked
)

// Result is one image's check outcome.
type Result struct {
	Image         string    `json:"image"`
	CheckedAt     time.Time `json:"checkedAt"`
	Kind          string    `json:"kind"`
	HasUpdate     bool      `json:"hasUpdate"`
	Current       string    `json:"current,omitempty"`       // current tag
	Candidate     string    `json:"candidate,omitempty"`     // newer tag (semver)
	Diff          string    `json:"diff,omitempty"`          // major|minor|patch
	CurrentDigest string    `json:"currentDigest,omitempty"` // local manifest digest
	LatestDigest  string    `json:"latestDigest,omitempty"`  // remote manifest digest
	Source        string    `json:"source,omitempty"`        // org.opencontainers.image.source (changelog)
	Error         string    `json:"error,omitempty"`
}

// LocalImages inspects locally-present images (docker.Client implements it). May
// be nil when no daemon is available.
type LocalImages interface {
	ImageRepoDigest(ctx context.Context, imageRef string) (string, error)
	ImageSource(ctx context.Context, imageRef string) (string, error)
}

// TagLister/Digester is the slice of the registry client the checker needs;
// an interface keeps the checker unit-testable without real HTTP.
type Registry interface {
	Tags(ctx context.Context, ref registry.Ref) ([]string, error)
	Digest(ctx context.Context, ref registry.Ref) (string, error)
}

// Checker performs image update checks.
type Checker struct {
	reg     Registry
	local   LocalImages // nil if no daemon
	logger  *slog.Logger
	workers int
}

// NewChecker builds a Checker. local may be nil (mutable-tag updates then can't
// be determined, only the remote digest is recorded).
func NewChecker(reg Registry, local LocalImages, logger *slog.Logger) *Checker {
	return &Checker{reg: reg, local: local, logger: logger, workers: 4}
}

// CheckImage checks a single image reference.
func (c *Checker) CheckImage(ctx context.Context, image string) Result {
	res := Result{Image: image, CheckedAt: time.Now().UTC()}

	ref, err := registry.ParseImageRef(image)
	if err != nil {
		res.Kind = KindUnsupported
		res.Error = err.Error()
		return res
	}
	res.Current = ref.Tag

	// Best-effort changelog source (needs the image present locally).
	if c.local != nil {
		if src, err := c.local.ImageSource(ctx, image); err == nil {
			res.Source = src
		}
	}

	// Version-like tag → semver candidate path.
	if ref.Tag != "" && registry.IsVersion(ref.Tag) {
		tags, err := c.reg.Tags(ctx, ref)
		if err != nil {
			res.Kind = KindError
			res.Error = err.Error()
			return res
		}
		cand, diff, ok := registry.Candidate(ref.Tag, tags)
		if ok {
			res.Kind = KindSemver
			res.HasUpdate = true
			res.Candidate = cand
			res.Diff = string(diff)
		} else {
			res.Kind = KindUpToDate
		}
		return res
	}

	// Mutable/absent tag → digest path.
	remote, err := c.reg.Digest(ctx, ref)
	if err != nil {
		res.Kind = KindError
		res.Error = err.Error()
		return res
	}
	res.LatestDigest = remote
	if c.local != nil {
		if local, err := c.local.ImageRepoDigest(ctx, image); err == nil && local != "" {
			res.CurrentDigest = local
			if local != remote {
				res.Kind = KindDigest
				res.HasUpdate = true
			} else {
				res.Kind = KindUpToDate
			}
			return res
		}
	}
	// Remote digest known but local unknown: record it, can't assert an update.
	res.Kind = KindDigest
	return res
}

// CheckAll checks images concurrently (bounded by the worker count; the registry
// client additionally caps per-host concurrency). Order of the input is
// preserved in the output.
func (c *Checker) CheckAll(ctx context.Context, images []string) []Result {
	results := make([]Result, len(images))
	sem := make(chan struct{}, c.workers)
	var wg sync.WaitGroup
	for i, img := range images {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, img string) {
			defer wg.Done()
			defer func() { <-sem }()
			results[i] = c.CheckImage(ctx, img)
		}(i, img)
	}
	wg.Wait()
	return results
}
