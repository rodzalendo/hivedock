package stacks

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
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
