-- 0005: user-ignored image updates. An update exists, but the user has chosen
-- to keep their deliberately-pinned version. Ignored images are excluded from
-- "Update all"/"Select all" and shown in their own section. Keyed by the full
-- image reference, so bumping the pin to a new tag clears the ignore naturally.
CREATE TABLE IF NOT EXISTS update_ignores (
    image      TEXT PRIMARY KEY,
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);
