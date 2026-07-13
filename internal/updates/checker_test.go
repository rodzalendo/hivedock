package updates

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/rogalinski/hivedock/internal/registry"
)

type fakeReg struct {
	tags    map[string][]string  // repo -> tags
	digests map[string]string    // repo -> remote digest
	created map[string]time.Time // "repo:tag" -> build date (zero = unknown)
	err     error
}

func (f *fakeReg) Tags(_ context.Context, ref registry.Ref) ([]string, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.tags[ref.Repo], nil
}

func (f *fakeReg) Digest(_ context.Context, ref registry.Ref) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	return f.digests[ref.Repo], nil
}

func (f *fakeReg) Created(_ context.Context, ref registry.Ref, tag string) (time.Time, error) {
	return f.created[ref.Repo+":"+tag], nil // zero time = unknown (fail-open)
}

type fakeLocal struct {
	d   map[string]string
	src map[string]string
}

func (f *fakeLocal) ImageRepoDigest(_ context.Context, image string) (string, error) {
	return f.d[image], nil
}

func (f *fakeLocal) ImageSource(_ context.Context, image string) (string, error) {
	return f.src[image], nil
}

func TestCheckImageSemverUpdate(t *testing.T) {
	reg := &fakeReg{tags: map[string][]string{"traefik/whoami": {"v1.10.0", "v1.11.0", "latest"}}}
	c := NewChecker(reg, nil, nil)

	res := c.CheckImage(context.Background(), "traefik/whoami:v1.10.0")
	if res.Kind != KindSemver || !res.HasUpdate {
		t.Fatalf("kind=%q hasUpdate=%v, want semver update", res.Kind, res.HasUpdate)
	}
	if res.Candidate != "v1.11.0" || res.Diff != "minor" {
		t.Errorf("candidate=%q diff=%q, want v1.11.0/minor", res.Candidate, res.Diff)
	}
}

func TestCheckImageSemverUpToDate(t *testing.T) {
	reg := &fakeReg{tags: map[string][]string{"library/nginx": {"1.26.0", "1.27.0"}}}
	c := NewChecker(reg, nil, nil)

	res := c.CheckImage(context.Background(), "nginx:1.27.0")
	if res.Kind != KindUpToDate || res.HasUpdate {
		t.Errorf("kind=%q hasUpdate=%v, want uptodate", res.Kind, res.HasUpdate)
	}
}

// The real-world qBittorrent case: lscr.io kept a legacy Ubuntu-era "20.04.1"
// tag (built 2021) that parses as numerically newer than the actual current
// release 5.1.2 (built 2025). The build-date guard must reject it.
func TestCheckImageRejectsStaleLegacyTag(t *testing.T) {
	reg := &fakeReg{
		tags: map[string][]string{"linuxserver/qbittorrent": {"5.1.2", "5.1.0", "20.04.1", "latest"}},
		created: map[string]time.Time{
			"linuxserver/qbittorrent:5.1.2":   time.Date(2025, 11, 16, 0, 0, 0, 0, time.UTC),
			"linuxserver/qbittorrent:20.04.1": time.Date(2021, 11, 29, 0, 0, 0, 0, time.UTC),
		},
	}
	c := NewChecker(reg, nil, nil)

	res := c.CheckImage(context.Background(), "lscr.io/linuxserver/qbittorrent:5.1.2")
	if res.HasUpdate {
		t.Fatalf("stale legacy tag surfaced as an update: candidate=%q", res.Candidate)
	}
	if res.Kind != KindUpToDate {
		t.Errorf("kind=%q, want uptodate", res.Kind)
	}
}

// A stale top candidate must not mask a genuinely newer release behind it.
func TestCheckImageStaleTagFallsBackToNextCandidate(t *testing.T) {
	reg := &fakeReg{
		tags: map[string][]string{"linuxserver/qbittorrent": {"5.1.2", "5.2.0", "20.04.1"}},
		created: map[string]time.Time{
			"linuxserver/qbittorrent:5.1.2":   time.Date(2025, 11, 16, 0, 0, 0, 0, time.UTC),
			"linuxserver/qbittorrent:5.2.0":   time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC),
			"linuxserver/qbittorrent:20.04.1": time.Date(2021, 11, 29, 0, 0, 0, 0, time.UTC),
		},
	}
	c := NewChecker(reg, nil, nil)

	res := c.CheckImage(context.Background(), "lscr.io/linuxserver/qbittorrent:5.1.2")
	if !res.HasUpdate || res.Candidate != "5.2.0" {
		t.Fatalf("candidate=%q hasUpdate=%v, want 5.2.0 after skipping stale 20.04.1", res.Candidate, res.HasUpdate)
	}
	if res.Diff != "minor" {
		t.Errorf("diff=%q, want minor", res.Diff)
	}
}

// A candidate slightly older than a freshly rebuilt current tag (security
// rebuilds) stays within tolerance and is still offered.
func TestCheckImageToleratesRecentRebuildSkew(t *testing.T) {
	now := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	reg := &fakeReg{
		tags: map[string][]string{"library/app": {"1.2.3", "1.3.0"}},
		created: map[string]time.Time{
			"library/app:1.2.3": now,                           // rebuilt today
			"library/app:1.3.0": now.Add(-20 * 24 * time.Hour), // released 20d ago
		},
	}
	c := NewChecker(reg, nil, nil)

	res := c.CheckImage(context.Background(), "app:1.2.3")
	if !res.HasUpdate || res.Candidate != "1.3.0" {
		t.Fatalf("candidate=%q hasUpdate=%v, want 1.3.0 within rebuild tolerance", res.Candidate, res.HasUpdate)
	}
}

func TestCheckImageDigestUpdate(t *testing.T) {
	reg := &fakeReg{digests: map[string]string{"library/app": "sha256:new"}}
	local := &fakeLocal{d: map[string]string{"app:latest": "sha256:old"}}
	c := NewChecker(reg, local, nil)

	res := c.CheckImage(context.Background(), "app:latest")
	if res.Kind != KindDigest || !res.HasUpdate {
		t.Fatalf("kind=%q hasUpdate=%v, want digest update", res.Kind, res.HasUpdate)
	}
	if res.CurrentDigest != "sha256:old" || res.LatestDigest != "sha256:new" {
		t.Errorf("digests current=%q latest=%q", res.CurrentDigest, res.LatestDigest)
	}
}

func TestCheckImageDigestUpToDate(t *testing.T) {
	reg := &fakeReg{digests: map[string]string{"library/app": "sha256:same"}}
	local := &fakeLocal{d: map[string]string{"app:latest": "sha256:same"}}
	c := NewChecker(reg, local, nil)

	res := c.CheckImage(context.Background(), "app:latest")
	if res.Kind != KindUpToDate || res.HasUpdate {
		t.Errorf("kind=%q hasUpdate=%v, want uptodate", res.Kind, res.HasUpdate)
	}
}

func TestCheckImageDigestLocalUnknown(t *testing.T) {
	reg := &fakeReg{digests: map[string]string{"library/app": "sha256:remote"}}
	c := NewChecker(reg, nil, nil) // no local digester

	res := c.CheckImage(context.Background(), "app:latest")
	if res.Kind != KindDigest || res.HasUpdate {
		t.Errorf("kind=%q hasUpdate=%v, want digest without a determinable update", res.Kind, res.HasUpdate)
	}
	if res.LatestDigest != "sha256:remote" {
		t.Errorf("latestDigest=%q, want sha256:remote", res.LatestDigest)
	}
}

func TestCheckImageErrorAndUnsupported(t *testing.T) {
	errReg := &fakeReg{err: errors.New("boom")}
	c := NewChecker(errReg, nil, nil)
	if res := c.CheckImage(context.Background(), "nginx:1.27.0"); res.Kind != KindError {
		t.Errorf("kind=%q, want error", res.Kind)
	}

	if res := c.CheckImage(context.Background(), ""); res.Kind != KindUnsupported {
		t.Errorf("empty image kind=%q, want unsupported", res.Kind)
	}
}

func TestCheckAllPreservesOrder(t *testing.T) {
	reg := &fakeReg{tags: map[string][]string{
		"library/a": {"1.0.0", "1.0.1"},
		"library/b": {"2.0.0"},
	}}
	c := NewChecker(reg, nil, nil)
	res := c.CheckAll(context.Background(), []string{"a:1.0.0", "b:2.0.0"})
	if len(res) != 2 || res[0].Image != "a:1.0.0" || res[1].Image != "b:2.0.0" {
		t.Fatalf("order not preserved: %+v", res)
	}
	if !res[0].HasUpdate || res[1].HasUpdate {
		t.Errorf("a should update, b should not: %+v", res)
	}
}
