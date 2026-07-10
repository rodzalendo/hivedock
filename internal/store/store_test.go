package store

import "testing"

// TestOpenRunsMigrations verifies the store opens on a fresh dir and applies
// all embedded migrations (recording each exactly once, idempotently).
func TestOpenRunsMigrations(t *testing.T) {
	dir := t.TempDir()

	s, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	var n int
	if err := s.DB().QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&n); err != nil {
		t.Fatalf("count migrations: %v", err)
	}
	if n == 0 {
		t.Fatal("expected at least one applied migration, got 0")
	}

	// The baseline tables must exist.
	for _, table := range []string{"settings", "service_prefs"} {
		var name string
		err := s.DB().QueryRow(
			`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, table,
		).Scan(&name)
		if err != nil {
			t.Fatalf("expected table %q to exist: %v", table, err)
		}
	}

	// Re-opening must be idempotent (no duplicate migration rows).
	s.Close()
	s2, err := Open(dir)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	t.Cleanup(func() { s2.Close() })
	var n2 int
	if err := s2.DB().QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&n2); err != nil {
		t.Fatalf("count after reopen: %v", err)
	}
	if n2 != n {
		t.Fatalf("migrations re-applied: had %d, now %d", n, n2)
	}
}
