package store

import (
	"database/sql"
	"errors"
	"fmt"
)

// GetSetting returns a settings value and whether it exists.
func (s *Store) GetSetting(key string) (string, bool, error) {
	var v string
	err := s.db.QueryRow(`SELECT value FROM settings WHERE key = ?`, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("get setting %q: %w", key, err)
	}
	return v, true, nil
}

// SetSetting upserts a settings value.
func (s *Store) SetSetting(key, value string) error {
	_, err := s.db.Exec(`
		INSERT INTO settings (key, value, updated_at) VALUES (?, ?, datetime('now'))
		ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = datetime('now')
	`, key, value)
	if err != nil {
		return fmt.Errorf("set setting %q: %w", key, err)
	}
	return nil
}

// DeleteSetting removes a settings key (used to clear an override back to the
// env/config default).
func (s *Store) DeleteSetting(key string) error {
	if _, err := s.db.Exec(`DELETE FROM settings WHERE key = ?`, key); err != nil {
		return fmt.Errorf("delete setting %q: %w", key, err)
	}
	return nil
}
