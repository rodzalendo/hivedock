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

func TestGitRemoteAndPull(t *testing.T) {
	if !gitAvailable() {
		t.Skip("git not installed")
	}
	base := t.TempDir()
	remote := filepath.Join(base, "remote.git")
	src := filepath.Join(base, "src")
	work := filepath.Join(base, "work") // stands in for STACKS_DIR

	mustGit := func(dir string, args ...string) {
		t.Helper()
		if out, err := runGit(dir, args...); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	commit := func(dir, msg string) {
		mustGit(dir, "-c", "user.name=t", "-c", "user.email=t@t", "add", "-A")
		mustGit(dir, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "--no-gpg-sign", "-m", msg)
	}

	// A bare remote, an initial commit pushed from a source clone.
	mustGit(base, "init", "--bare", remote)
	mustGit(base, "clone", remote, src)
	if err := os.WriteFile(filepath.Join(src, "compose.yaml"), []byte("services:\n  web:\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	commit(src, "init")
	mustGit(src, "push", "origin", "HEAD:refs/heads/main")
	mustGit(remote, "symbolic-ref", "HEAD", "refs/heads/main")

	// The "stacks dir" is a clone of that remote.
	mustGit(base, "clone", remote, work)

	url, branch, sha, ok := GitRemote(work)
	if !ok || url == "" {
		t.Fatalf("GitRemote ok=%v url=%q", ok, url)
	}
	if branch != "main" {
		t.Errorf("branch = %q, want main", branch)
	}
	if sha == "" {
		t.Error("commit is empty")
	}
	if _, _, _, ok := GitRemote(t.TempDir()); ok {
		t.Error("a non-repo dir reported a remote")
	}

	// Advance the remote, then fast-forward the stacks dir into it.
	if err := os.WriteFile(filepath.Join(src, "new.yaml"), []byte("services:\n  db:\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	commit(src, "add db")
	mustGit(src, "push", "origin", "HEAD:refs/heads/main")

	if _, err := GitPull(work); err != nil {
		t.Fatalf("GitPull: %v", err)
	}
	if _, err := os.Stat(filepath.Join(work, "new.yaml")); err != nil {
		t.Errorf("pulled file missing after fast-forward: %v", err)
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
