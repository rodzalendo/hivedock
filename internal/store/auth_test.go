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

func TestSessionStoredHashed(t *testing.T) {
	s := testStore(t)
	if err := s.CreateSession("secret-token", time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	var stored string
	if err := s.db.QueryRow(`SELECT token_hash FROM sessions`).Scan(&stored); err != nil {
		t.Fatalf("read token_hash: %v", err)
	}
	if stored == "secret-token" {
		t.Fatal("raw token stored in the DB — must be hashed")
	}
	if stored != hashToken("secret-token") {
		t.Fatalf("stored %q is not the SHA-256 of the token", stored)
	}
}

func TestSessionIdleExpiry(t *testing.T) {
	s := testStore(t)
	// Absolute expiry far in the future, but idle beyond the window → invalid.
	if err := s.CreateSession("tok", time.Now().Add(30*24*time.Hour)); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	old := time.Now().Add(-sessionIdleTTL - time.Hour).UTC().Format(time.RFC3339)
	if _, err := s.db.Exec(`UPDATE sessions SET last_seen = ?`, old); err != nil {
		t.Fatalf("age last_seen: %v", err)
	}
	if ok, _ := s.SessionValid("tok"); ok {
		t.Fatal("idle-expired session reported valid")
	}
}

func TestDeleteAllSessions(t *testing.T) {
	s := testStore(t)
	_ = s.CreateSession("a", time.Now().Add(time.Hour))
	_ = s.CreateSession("b", time.Now().Add(time.Hour))
	if err := s.DeleteAllSessions(); err != nil {
		t.Fatalf("DeleteAllSessions: %v", err)
	}
	if ok, _ := s.SessionValid("a"); ok {
		t.Fatal("session a survived DeleteAllSessions")
	}
	if ok, _ := s.SessionValid("b"); ok {
		t.Fatal("session b survived DeleteAllSessions")
	}
}
