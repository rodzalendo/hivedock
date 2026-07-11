-- 0003_updates: cached image update-check results (Phase 4).
-- Cache only — the source of truth for "what update exists" is always a fresh
-- registry check; this table just avoids re-hitting registries on every page load
-- and survives restarts. Keyed by the image reference as written in compose.

CREATE TABLE IF NOT EXISTS image_checks (
    image          TEXT PRIMARY KEY,
    checked_at     TEXT NOT NULL,
    kind           TEXT NOT NULL,             -- semver | digest | uptodate | error | unsupported
    has_update     INTEGER NOT NULL DEFAULT 0,
    current_tag    TEXT,
    candidate_tag  TEXT,
    diff           TEXT,                       -- major | minor | patch
    current_digest TEXT,
    latest_digest  TEXT,
    error          TEXT
);
