package store

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ErrAdminExists is returned by CreateAdmin when the single admin is already
// configured (first-run setup can happen exactly once).
var ErrAdminExists = errors.New("admin already exists")

// ErrNoAdmin is returned by AdminCredentials before first-run setup.
var ErrNoAdmin = errors.New("no admin configured")

// AdminExists reports whether the single admin account has been set up.
func (s *Store) AdminExists() (bool, error) {
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM admin WHERE id = 1`).Scan(&n); err != nil {
		return false, fmt.Errorf("count admin: %w", err)
	}
	return n > 0, nil
}

// CreateAdmin stores the single admin's credentials. Returns ErrAdminExists if
// one is already configured — the ON CONFLICT makes this race-free.
func (s *Store) CreateAdmin(username, passwordHash string) error {
	res, err := s.db.Exec(
		`INSERT INTO admin (id, username, password_hash) VALUES (1, ?, ?) ON CONFLICT(id) DO NOTHING`,
		username, passwordHash,
	)
	if err != nil {
		return fmt.Errorf("create admin: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrAdminExists
	}
	return nil
}

// AdminCredentials returns the admin username and stored password hash. Returns
// ErrNoAdmin before setup.
func (s *Store) AdminCredentials() (username, passwordHash string, err error) {
	err = s.db.QueryRow(`SELECT username, password_hash FROM admin WHERE id = 1`).Scan(&username, &passwordHash)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", ErrNoAdmin
	}
	if err != nil {
		return "", "", fmt.Errorf("query admin: %w", err)
	}
	return username, passwordHash, nil
}

// CreateSession records a session token with an absolute expiry.
func (s *Store) CreateSession(token string, expiresAt time.Time) error {
	_, err := s.db.Exec(
		`INSERT INTO sessions (token, expires_at) VALUES (?, ?)`,
		token, expiresAt.UTC().Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	return nil
}

// SessionValid reports whether token names a live (unexpired) session. Expired
// rows are opportunistically deleted on lookup.
func (s *Store) SessionValid(token string) (bool, error) {
	if token == "" {
		return false, nil
	}
	var expiresAt string
	err := s.db.QueryRow(`SELECT expires_at FROM sessions WHERE token = ?`, token).Scan(&expiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("query session: %w", err)
	}
	exp, err := time.Parse(time.RFC3339, expiresAt)
	if err != nil {
		return false, fmt.Errorf("parse session expiry: %w", err)
	}
	if time.Now().After(exp) {
		_, _ = s.db.Exec(`DELETE FROM sessions WHERE token = ?`, token)
		return false, nil
	}
	return true, nil
}

// DeleteSession removes a session (logout). Missing tokens are a no-op.
func (s *Store) DeleteSession(token string) error {
	if _, err := s.db.Exec(`DELETE FROM sessions WHERE token = ?`, token); err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	return nil
}

// DeleteExpiredSessions purges sessions past their expiry.
func (s *Store) DeleteExpiredSessions() error {
	if _, err := s.db.Exec(`DELETE FROM sessions WHERE expires_at < ?`, time.Now().UTC().Format(time.RFC3339)); err != nil {
		return fmt.Errorf("delete expired sessions: %w", err)
	}
	return nil
}
