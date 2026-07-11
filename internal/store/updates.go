package store

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/rogalinski/hivedock/internal/updates"
)

// SaveImageChecks upserts a batch of update-check results in one transaction.
func (s *Store) SaveImageChecks(results []updates.Result) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT INTO image_checks
			(image, checked_at, kind, has_update, current_tag, candidate_tag, diff, current_digest, latest_digest, source, error)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(image) DO UPDATE SET
			checked_at=excluded.checked_at, kind=excluded.kind, has_update=excluded.has_update,
			current_tag=excluded.current_tag, candidate_tag=excluded.candidate_tag, diff=excluded.diff,
			current_digest=excluded.current_digest, latest_digest=excluded.latest_digest,
			source=excluded.source, error=excluded.error
	`)
	if err != nil {
		return fmt.Errorf("prepare: %w", err)
	}
	defer stmt.Close()

	for _, r := range results {
		hasUpdate := 0
		if r.HasUpdate {
			hasUpdate = 1
		}
		if _, err := stmt.Exec(
			r.Image, r.CheckedAt.UTC().Format(time.RFC3339), r.Kind, hasUpdate,
			r.Current, r.Candidate, r.Diff, r.CurrentDigest, r.LatestDigest, r.Source, r.Error,
		); err != nil {
			return fmt.Errorf("upsert %s: %w", r.Image, err)
		}
	}
	return tx.Commit()
}

// SetImageIgnored records (or clears) a user's decision to ignore updates for a
// specific image reference. Keyed by the full ref, so re-pinning to a new tag
// clears the ignore on its own.
func (s *Store) SetImageIgnored(image string, ignored bool) error {
	if ignored {
		if _, err := s.db.Exec(`INSERT INTO update_ignores (image) VALUES (?) ON CONFLICT(image) DO NOTHING`, image); err != nil {
			return fmt.Errorf("ignore image: %w", err)
		}
		return nil
	}
	if _, err := s.db.Exec(`DELETE FROM update_ignores WHERE image = ?`, image); err != nil {
		return fmt.Errorf("unignore image: %w", err)
	}
	return nil
}

// IgnoredImages returns the set of image references the user has ignored.
func (s *Store) IgnoredImages() (map[string]bool, error) {
	rows, err := s.db.Query(`SELECT image FROM update_ignores`)
	if err != nil {
		return nil, fmt.Errorf("query update_ignores: %w", err)
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var image string
		if err := rows.Scan(&image); err != nil {
			return nil, fmt.Errorf("scan update_ignores: %w", err)
		}
		out[image] = true
	}
	return out, rows.Err()
}

// ImageChecks loads all cached results, keyed by image reference.
func (s *Store) ImageChecks() (map[string]updates.Result, error) {
	rows, err := s.db.Query(`
		SELECT image, checked_at, kind, has_update, current_tag, candidate_tag, diff, current_digest, latest_digest, source, error
		FROM image_checks
	`)
	if err != nil {
		return nil, fmt.Errorf("query image_checks: %w", err)
	}
	defer rows.Close()

	out := map[string]updates.Result{}
	for rows.Next() {
		var (
			r               updates.Result
			checkedAt       string
			hasUpdate       int
			cur, cand       sql.NullString
			diff, cd        sql.NullString
			ld, src, errStr sql.NullString
		)
		if err := rows.Scan(&r.Image, &checkedAt, &r.Kind, &hasUpdate, &cur, &cand, &diff, &cd, &ld, &src, &errStr); err != nil {
			return nil, fmt.Errorf("scan image_checks: %w", err)
		}
		r.CheckedAt, _ = time.Parse(time.RFC3339, checkedAt)
		r.HasUpdate = hasUpdate != 0
		r.Current = cur.String
		r.Candidate = cand.String
		r.Diff = diff.String
		r.CurrentDigest = cd.String
		r.LatestDigest = ld.String
		r.Source = src.String
		r.Error = errStr.String
		out[r.Image] = r
	}
	return out, rows.Err()
}
