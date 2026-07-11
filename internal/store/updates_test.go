package store

import (
	"testing"
	"time"

	"github.com/rogalinski/hivedock/internal/updates"
)

func TestImageChecksRoundTrip(t *testing.T) {
	s := testStore(t)

	now := time.Now().UTC().Truncate(time.Second)
	in := []updates.Result{
		{Image: "nginx:1.27.0", CheckedAt: now, Kind: updates.KindSemver, HasUpdate: true, Current: "1.27.0", Candidate: "1.27.2", Diff: "patch"},
		{Image: "app:latest", CheckedAt: now, Kind: updates.KindDigest, HasUpdate: true, CurrentDigest: "sha256:old", LatestDigest: "sha256:new"},
		{Image: "redis:7.4", CheckedAt: now, Kind: updates.KindUpToDate},
	}
	if err := s.SaveImageChecks(in); err != nil {
		t.Fatalf("SaveImageChecks: %v", err)
	}

	got, err := s.ImageChecks()
	if err != nil {
		t.Fatalf("ImageChecks: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d rows, want 3", len(got))
	}
	if r := got["nginx:1.27.0"]; !r.HasUpdate || r.Candidate != "1.27.2" || r.Diff != "patch" {
		t.Errorf("nginx row = %+v", r)
	}
	if r := got["app:latest"]; r.LatestDigest != "sha256:new" || r.CurrentDigest != "sha256:old" {
		t.Errorf("app row = %+v", r)
	}

	// Upsert: re-saving the same image replaces the row.
	in[0].Candidate = "1.28.0"
	in[0].Diff = "minor"
	if err := s.SaveImageChecks(in[:1]); err != nil {
		t.Fatalf("re-save: %v", err)
	}
	got, _ = s.ImageChecks()
	if r := got["nginx:1.27.0"]; r.Candidate != "1.28.0" || r.Diff != "minor" {
		t.Errorf("after upsert nginx row = %+v", r)
	}
}
