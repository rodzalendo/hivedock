package stacks

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Contained resolves target and returns its real path, requiring it to live
// inside root — both with symlinks resolved. It is the guard behind every file
// operation (compose/.env read and write, create, rename, delete): a compose
// file, .env, or stack directory that symlinks outside STACKS_DIR is refused,
// even though the scanner or docker compose would otherwise follow it.
//
// It works for targets that don't exist yet (a create/save): the deepest
// existing ancestor is symlink-resolved and the remaining segments are appended
// lexically, so a not-yet-created file still can't escape through a symlinked
// parent.
func Contained(root, target string) (string, error) {
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		// Root should exist and resolve; if it doesn't (misconfig, or a dir not
		// created yet), fall back to an absolute clean path so containment still
		// gives a deterministic answer instead of erroring out ambiguously.
		if realRoot, err = filepath.Abs(root); err != nil {
			return "", fmt.Errorf("resolve stacks root %q: %w", root, err)
		}
	}
	abs, err := filepath.Abs(target)
	if err != nil {
		return "", fmt.Errorf("resolve %q: %w", target, err)
	}
	resolved := resolveExisting(abs)
	if resolved != realRoot && !strings.HasPrefix(resolved, realRoot+string(os.PathSeparator)) {
		return "", fmt.Errorf("path %q escapes the stacks directory", target)
	}
	return resolved, nil
}

// resolveExisting resolves symlinks on the longest existing prefix of path and
// re-appends the remaining (non-existent) segments lexically, so a path that is
// about to be created can still be containment-checked without EvalSymlinks
// failing on the missing leaf.
func resolveExisting(path string) string {
	if r, err := filepath.EvalSymlinks(path); err == nil {
		return r
	}
	parent := filepath.Dir(path)
	if parent == path { // reached the filesystem root; nothing left to resolve
		return path
	}
	return filepath.Join(resolveExisting(parent), filepath.Base(path))
}
