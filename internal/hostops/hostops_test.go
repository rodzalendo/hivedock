package hostops

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/rogalinski/hivedock/internal/compose"
)

func TestValidStackName(t *testing.T) {
	ok := []string{"web", "media-server", "immich_db", "n8n"}
	bad := []string{"", "..", "../evil", "MyStack", "has.dot", "_lead", "-lead", "a/b"}
	for _, n := range ok {
		if !ValidStackName(n) {
			t.Errorf("%q should be valid", n)
		}
	}
	for _, n := range bad {
		if ValidStackName(n) {
			t.Errorf("%q should be invalid", n)
		}
	}
}

func TestCheckLockAndAtomicWrite(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "compose.yaml")
	if err := AtomicWrite(p, []byte("services:\n")); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(p)
	sha := Sha256Hex(data)

	if err := CheckLock(p, sha); err != nil {
		t.Fatalf("CheckLock (matching) = %v, want nil", err)
	}
	if err := CheckLock(p, ""); err != nil {
		t.Fatalf("CheckLock (no base) = %v, want nil", err)
	}

	// Change the file, then a stale base must surface a ConflictError with the
	// current bytes.
	next := []byte("services:\n  web:\n")
	if err := AtomicWrite(p, next); err != nil {
		t.Fatal(err)
	}
	err := CheckLock(p, sha)
	var conflict *ConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("CheckLock (stale) = %v, want *ConflictError", err)
	}
	if conflict.Sha256 != Sha256Hex(next) || conflict.Content != string(next) {
		t.Errorf("conflict payload mismatch: %+v", conflict)
	}
}

func TestContain(t *testing.T) {
	dir := t.TempDir()
	inside := filepath.Join(dir, "stack", "compose.yaml")
	if _, err := Contain(dir, inside); err != nil {
		t.Errorf("Contain(inside) = %v, want nil", err)
	}
	if _, err := Contain(dir, filepath.Join(dir, "..", "evil")); !errors.Is(err, ErrEscape) {
		t.Errorf("Contain(escape) = %v, want ErrEscape", err)
	}
}

func TestCodeRoundTrip(t *testing.T) {
	cases := []error{ErrNotFound, ErrExternal, ErrBusy, ErrReadOnly, ErrInvalidName, ErrExists, ErrRunning, compose.ErrEnvManaged, compose.ErrDigestPinned}
	for _, e := range cases {
		code := CodeFor(e)
		if code == "" {
			t.Errorf("CodeFor(%v) = empty", e)
			continue
		}
		if got := ErrorForCode(code, "", nil); !errors.Is(got, e) {
			t.Errorf("round-trip %v: code %q → %v", e, code, got)
		}
	}
	// Conflict carries its bytes across the wire.
	c := &ConflictError{Content: "x", Sha256: "abc"}
	data, _ := json.Marshal(c)
	back := ErrorForCode(CodeFor(c), "", data)
	var bc *ConflictError
	if !errors.As(back, &bc) || bc.Sha256 != "abc" {
		t.Errorf("conflict round-trip lost data: %v", back)
	}
	// A generic error survives as its text.
	if got := ErrorForCode("", "boom", nil); got == nil || got.Error() != "boom" {
		t.Errorf("generic round-trip = %v", got)
	}
}
