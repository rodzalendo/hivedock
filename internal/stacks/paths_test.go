package stacks

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestContainedAllowsPathsUnderRoot(t *testing.T) {
	root := t.TempDir()
	stack := filepath.Join(root, "web")
	if err := os.MkdirAll(stack, 0o755); err != nil {
		t.Fatal(err)
	}
	compose := filepath.Join(stack, "compose.yaml")
	if err := os.WriteFile(compose, []byte("services:\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	for _, p := range []string{
		stack,                              // the stack dir itself
		compose,                            // an existing file
		filepath.Join(stack, ".env"),       // a not-yet-created file
		filepath.Join(root, "a", "b.yaml"), // a nested not-yet-created path
	} {
		if _, err := Contained(root, p); err != nil {
			t.Errorf("Contained(%q) = err %v, want ok", p, err)
		}
	}
}

func TestContainedRejectsTraversal(t *testing.T) {
	root := t.TempDir()
	for _, p := range []string{
		filepath.Join(root, "..", "escape.yaml"),
		filepath.Join(root, "..", ".."),
		filepath.Dir(root), // the parent directory
		filepath.Join(root, "web", "..", "..", "etc", "passwd"),
	} {
		if _, err := Contained(root, p); err == nil {
			t.Errorf("Contained(%q) = ok, want escape error", p)
		}
	}
}

func TestContainedRejectsSymlinkEscape(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation typically needs elevation on Windows")
	}
	root := t.TempDir()
	outside := t.TempDir()
	secret := filepath.Join(outside, "secret")
	if err := os.WriteFile(secret, []byte("top secret"), 0o600); err != nil {
		t.Fatal(err)
	}

	stack := filepath.Join(root, "web")
	if err := os.MkdirAll(stack, 0o755); err != nil {
		t.Fatal(err)
	}
	// A compose file that is a symlink pointing outside the stacks root.
	link := filepath.Join(stack, "compose.yaml")
	if err := os.Symlink(secret, link); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	if got, err := Contained(root, link); err == nil {
		t.Errorf("Contained(symlink→outside) = %q, want escape error", got)
	}

	// A symlink that stays inside the root is fine.
	inside := filepath.Join(root, "other")
	if err := os.MkdirAll(inside, 0o755); err != nil {
		t.Fatal(err)
	}
	innerLink := filepath.Join(stack, "shared.yaml")
	if err := os.Symlink(filepath.Join(inside, "shared.yaml"), innerLink); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	if _, err := Contained(root, innerLink); err != nil {
		t.Errorf("Contained(symlink→inside) = err %v, want ok", err)
	}
}

func TestContainedRejectsSymlinkedStackDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation typically needs elevation on Windows")
	}
	root := t.TempDir()
	outside := t.TempDir()
	// A whole stack directory that is a symlink out of the tree.
	link := filepath.Join(root, "evil")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	if _, err := Contained(root, filepath.Join(link, "compose.yaml")); err == nil {
		t.Errorf("Contained(compose under symlinked-out dir) = ok, want escape error")
	}
}

func TestContainedResultStaysUnderRealRoot(t *testing.T) {
	root := t.TempDir()
	real, err := Contained(root, filepath.Join(root, "web", "compose.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	realRoot, _ := filepath.EvalSymlinks(root)
	if !strings.HasPrefix(real, realRoot) {
		t.Errorf("resolved %q not under real root %q", real, realRoot)
	}
}
