package stacks

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Git auto-commit (HARDENING.md §5.4) keeps a local audit trail of every change
// under STACKS_DIR — HiveDock's own writes and out-of-band ones alike. It is
// opt-in and local only: no remotes, no push, no branching. All operations shell
// out to git with argument arrays (no shell — invariant 9).

// IsGitWorktree reports whether dir sits inside a git working tree.
func IsGitWorktree(dir string) bool {
	out, err := runGit(dir, "rev-parse", "--is-inside-work-tree")
	return err == nil && strings.TrimSpace(out) == "true"
}

// GitInit initializes dir as a git repository and makes an initial commit of
// whatever is already there, so later auto-commits have a base. No-op if dir is
// already a worktree.
func GitInit(dir string) error {
	if IsGitWorktree(dir) {
		return nil
	}
	if out, err := runGit(dir, "init"); err != nil {
		return fmt.Errorf("git init: %w: %s", err, strings.TrimSpace(out))
	}
	return GitCommitAll(dir, "initialize stacks repository")
}

// GitCommitAll stages everything under dir and commits it with a fixed HiveDock
// author. It is a no-op (nil) when the worktree is already clean. No remotes, no
// push — a local paper trail only.
func GitCommitAll(dir, action string) error {
	if out, err := runGit(dir, "add", "-A"); err != nil {
		return fmt.Errorf("git add: %w: %s", err, strings.TrimSpace(out))
	}
	// `git diff --cached --quiet` exits 0 when the index matches HEAD (nothing
	// staged) — then there is nothing to commit.
	if _, err := runGit(dir, "diff", "--cached", "--quiet"); err == nil {
		return nil
	}
	out, err := runGit(dir,
		"-c", "user.name=HiveDock",
		"-c", "user.email=hivedock@localhost",
		"commit", "--no-gpg-sign", "-m", "hivedock: "+action)
	if err != nil {
		return fmt.Errorf("git commit: %w: %s", err, strings.TrimSpace(out))
	}
	return nil
}

// GitRemote returns the "origin" URL, current branch, and short commit for dir's
// repo, with ok=false when dir isn't a worktree or has no origin remote. Used to
// drive the "Pull from git" UI: it only appears when there's a remote to pull.
func GitRemote(dir string) (url, branch, commit string, ok bool) {
	if !IsGitWorktree(dir) {
		return "", "", "", false
	}
	u, err := runGit(dir, "remote", "get-url", "origin")
	if err != nil || strings.TrimSpace(u) == "" {
		return "", "", "", false
	}
	url = strings.TrimSpace(u)
	if b, err := runGit(dir, "rev-parse", "--abbrev-ref", "HEAD"); err == nil {
		branch = strings.TrimSpace(b)
	}
	if c, err := runGit(dir, "rev-parse", "--short", "HEAD"); err == nil {
		commit = strings.TrimSpace(c)
	}
	return url, branch, commit, true
}

// GitPull fast-forwards dir's repo from origin. --ff-only never creates a merge
// commit: if local history diverged (e.g. local auto-commits the remote doesn't
// have), it fails cleanly instead of entangling the trees — pull-only GitOps, no
// surprises. Bounded by a timeout, and prompts stay disabled so a private repo
// fails fast rather than hanging on a credential prompt.
func GitPull(dir string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "-C", dir, "pull", "--ff-only")
	cmd.Env = append(os.Environ(),
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_TERMINAL_PROMPT=0",
		"HOME="+dir,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return strings.TrimSpace(string(out)), fmt.Errorf("git pull: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// runGit runs one git command in dir. The environment is scoped so commits are
// hermetic and can never reach a network: no system/global config (author, gpg,
// hooks come only from the -c flags above), no credential/terminal prompts.
func runGit(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(),
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_TERMINAL_PROMPT=0",
		"HOME="+dir, // don't read the invoking user's ~/.gitconfig
	)
	out, err := cmd.CombinedOutput()
	return string(out), err
}
