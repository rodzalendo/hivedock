package hostops

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"regexp"

	"github.com/rogalinski/hivedock/internal/stacks"
)

// StackNamePattern constrains a stack directory name to a single safe path
// segment (compose project-name rule): lowercase alnum + dash/underscore, max 64,
// starting alphanumeric. No separators, dots, or way to spell ".." — so it can't
// escape the stacks directory (§4.1).
var StackNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)

// ValidStackName reports whether name is an allowed stack directory name.
func ValidStackName(name string) bool { return StackNamePattern.MatchString(name) }

// Sha256Hex is the lowercase hex SHA-256 of b (of "" for a missing file).
func Sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// Contain resolves p (a file/dir belonging to a stack) with symlinks and requires
// it to stay inside stacksDir (§4.2); returns the real path, or ErrEscape when it
// points outside the tree.
func Contain(stacksDir, p string) (string, error) {
	real, err := stacks.Contained(stacksDir, p)
	if err != nil {
		return "", ErrEscape
	}
	return real, nil
}

// CheckLock enforces the optimistic lock (§5.1): if base (the hash the editor
// loaded) is set and no longer matches the file at real, returns a *ConflictError
// carrying the current bytes. A missing file hashes as "".
func CheckLock(real, base string) error {
	if base == "" {
		return nil
	}
	current, err := os.ReadFile(real)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if cur := Sha256Hex(current); cur != base {
		return &ConflictError{Content: string(current), Sha256: cur}
	}
	return nil
}

// AtomicWrite writes data to a temp file in the target's directory and renames it
// over path, preserving the existing file's mode. The rename is atomic on the same
// filesystem, so a reader (or the daemon) never sees a half-written file.
func AtomicWrite(path string, data []byte) error {
	dir := filepath.Dir(path)
	mode := os.FileMode(0o644)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode()
	}
	tmp, err := os.CreateTemp(dir, ".hivedock-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op after a successful rename

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, mode); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}
