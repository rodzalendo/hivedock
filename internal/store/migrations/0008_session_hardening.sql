-- 0008: session hardening. Sessions now store a SHA-256 *hash* of the opaque
-- token (never the token itself, so a DB read can't resurrect a session) and
-- track last_seen for idle expiry. Existing rows held raw tokens and are
-- cleared — everyone re-authenticates once (see docs/HARDENING.md §2.4, §9).
-- NB: ADD COLUMN defaults must be constant in SQLite, so last_seen defaults to
-- '' here; every row is deleted immediately after, and inserts always set it.
ALTER TABLE sessions RENAME COLUMN token TO token_hash;
ALTER TABLE sessions ADD COLUMN last_seen TEXT NOT NULL DEFAULT '';
DELETE FROM sessions;
