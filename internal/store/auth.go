package store

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"
)

// sessionIdleTTL expires a session unused for this long, independent of the
// absolute expiry the caller sets — a stolen-but-idle token has a short life.
const sessionIdleTTL = 7 * 24 * time.Hour

// hashToken returns the hex SHA-256 of a session token. Only the hash is stored,
// so read access to the DB cannot recover a usable session cookie.
func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

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

// CreateSession records a session (by token hash) with an absolute expiry.
func (s *Store) CreateSession(token string, expiresAt time.Time) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(
		`INSERT INTO sessions (token_hash, created_at, expires_at, last_seen) VALUES (?, ?, ?, ?)`,
		hashToken(token), now, expiresAt.UTC().Format(time.RFC3339), now,
	)
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	return nil
}

// SessionValid reports whether token names a live session — within both its
// absolute expiry and the idle window (§2.4). A valid lookup slides last_seen
// forward; an expired row is opportunistically deleted.
func (s *Store) SessionValid(token string) (bool, error) {
	if token == "" {
		return false, nil
	}
	h := hashToken(token)
	var expiresAt, lastSeen string
	err := s.db.QueryRow(`SELECT expires_at, last_seen FROM sessions WHERE token_hash = ?`, h).Scan(&expiresAt, &lastSeen)
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
	seen, err := time.Parse(time.RFC3339, lastSeen)
	if err != nil {
		return false, fmt.Errorf("parse session last_seen: %w", err)
	}
	now := time.Now()
	if now.After(exp) || now.After(seen.Add(sessionIdleTTL)) {
		_, _ = s.db.Exec(`DELETE FROM sessions WHERE token_hash = ?`, h)
		return false, nil
	}
	_, _ = s.db.Exec(`UPDATE sessions SET last_seen = ? WHERE token_hash = ?`, now.UTC().Format(time.RFC3339), h)
	return true, nil
}

// DeleteSession removes a session (logout). Missing tokens are a no-op.
func (s *Store) DeleteSession(token string) error {
	if _, err := s.db.Exec(`DELETE FROM sessions WHERE token_hash = ?`, hashToken(token)); err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	return nil
}

// DeleteAllSessions invalidates every session (e.g. on a password change).
func (s *Store) DeleteAllSessions() error {
	if _, err := s.db.Exec(`DELETE FROM sessions`); err != nil {
		return fmt.Errorf("delete all sessions: %w", err)
	}
	return nil
}

// DeleteExpiredSessions purges sessions past their absolute expiry or idle window.
func (s *Store) DeleteExpiredSessions() error {
	now := time.Now().UTC().Format(time.RFC3339)
	idleCut := time.Now().Add(-sessionIdleTTL).UTC().Format(time.RFC3339)
	if _, err := s.db.Exec(`DELETE FROM sessions WHERE expires_at < ? OR last_seen < ?`, now, idleCut); err != nil {
		return fmt.Errorf("delete expired sessions: %w", err)
	}
	return nil
}
