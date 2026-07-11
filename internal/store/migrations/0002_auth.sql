-- 0002_auth: single-admin authentication (Phase 3).
-- Auth is app state, not a stack definition — it belongs in SQLite.

-- The single admin account. Enforced as a singleton via the id = 1 check so a
-- second account can never be created.
CREATE TABLE IF NOT EXISTS admin (
    id            INTEGER PRIMARY KEY CHECK (id = 1),
    username      TEXT NOT NULL,
    password_hash TEXT NOT NULL,
    created_at    TEXT NOT NULL DEFAULT (datetime('now'))
);

-- Opaque session tokens (set as an HttpOnly cookie). DB-backed so sessions and
-- logout survive process restarts, and expiry is authoritative server-side.
CREATE TABLE IF NOT EXISTS sessions (
    token      TEXT PRIMARY KEY,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    expires_at TEXT NOT NULL
);
