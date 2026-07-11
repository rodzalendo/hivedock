package store

import (
	"errors"
	"testing"
	"time"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestAdminLifecycle(t *testing.T) {
	s := testStore(t)

	exists, err := s.AdminExists()
	if err != nil {
		t.Fatalf("AdminExists: %v", err)
	}
	if exists {
		t.Fatal("admin exists before setup")
	}

	if _, _, err := s.AdminCredentials(); !errors.Is(err, ErrNoAdmin) {
		t.Fatalf("AdminCredentials before setup = %v, want ErrNoAdmin", err)
	}

	if err := s.CreateAdmin("admin", "hash1"); err != nil {
		t.Fatalf("CreateAdmin: %v", err)
	}

	exists, _ = s.AdminExists()
	if !exists {
		t.Fatal("admin missing after setup")
	}

	// Second attempt must fail — the account is a singleton.
	if err := s.CreateAdmin("intruder", "hash2"); !errors.Is(err, ErrAdminExists) {
		t.Fatalf("second CreateAdmin = %v, want ErrAdminExists", err)
	}

	user, hash, err := s.AdminCredentials()
	if err != nil {
		t.Fatalf("AdminCredentials: %v", err)
	}
	if user != "admin" || hash != "hash1" {
		t.Fatalf("credentials = %q/%q, want admin/hash1", user, hash)
	}
}

func TestSessionLifecycle(t *testing.T) {
	s := testStore(t)

	if ok, _ := s.SessionValid("nope"); ok {
		t.Fatal("unknown token reported valid")
	}

	if err := s.CreateSession("tok-live", time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if ok, err := s.SessionValid("tok-live"); err != nil || !ok {
		t.Fatalf("live session valid=%v err=%v", ok, err)
	}

	if err := s.DeleteSession("tok-live"); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}
	if ok, _ := s.SessionValid("tok-live"); ok {
		t.Fatal("deleted session still valid")
	}

	// Expired sessions are treated as invalid and cleaned up.
	if err := s.CreateSession("tok-old", time.Now().Add(-time.Minute)); err != nil {
		t.Fatalf("CreateSession expired: %v", err)
	}
	if ok, _ := s.SessionValid("tok-old"); ok {
		t.Fatal("expired session reported valid")
	}
}
