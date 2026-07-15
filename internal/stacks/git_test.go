package stacks

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func gitAvailable() bool {
	_, err := exec.LookPath("git")
	return err == nil
}

func TestGitInitAndCommit(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	if IsGitWorktree(dir) {
		t.Fatal("fresh temp dir reports as a worktree")
	}
	if err := os.WriteFile(filepath.Join(dir, "compose.yaml"), []byte("services:\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := GitInit(dir); err != nil {
		t.Fatalf("GitInit: %v", err)
	}
	if !IsGitWorktree(dir) {
		t.Fatal("dir is not a worktree after GitInit")
	}

	// Nothing changed → commit is a no-op, and the log doesn't grow.
	before := gitLog(t, dir)
	if err := GitCommitAll(dir, "no change"); err != nil {
		t.Fatalf("no-op commit: %v", err)
	}
	if got := gitLog(t, dir); len(got) != len(before) {
		t.Errorf("no-op commit added a commit: %d -> %d", len(before), len(got))
	}

	// A real change → one commit with the HiveDock author + message prefix.
	if err := os.WriteFile(filepath.Join(dir, "compose.yaml"), []byte("services:\n  web:\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := GitCommitAll(dir, "save web compose"); err != nil {
		t.Fatalf("commit: %v", err)
	}
	log := gitLog(t, dir)
	if len(log) != len(before)+1 {
		t.Fatalf("expected one new commit, log=%v", log)
	}
	if !strings.HasPrefix(log[0], "hivedock: save web compose") {
		t.Errorf("commit subject = %q, want hivedock: prefix", log[0])
	}
	author, _ := runGit(dir, "log", "-1", "--format=%an <%ae>")
	if strings.TrimSpace(author) != "HiveDock <hivedock@localhost>" {
		t.Errorf("author = %q, want HiveDock <hivedock@localhost>", strings.TrimSpace(author))
	}
}

func gitLog(t *testing.T, dir string) []string {
	t.Helper()
	out, err := runGit(dir, "log", "--format=%s")
	if err != nil {
		// No commits yet.
		return nil
	}
	var subs []string
	for _, l := range strings.Split(strings.TrimSpace(out), "\n") {
		if l != "" {
			subs = append(subs, l)
		}
	}
	return subs
}
