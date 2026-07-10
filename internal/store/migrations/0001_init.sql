-- 0001_init: baseline app-state tables.
-- NB: no stack definitions here — compose files are the source of truth.

-- Generic key/value app settings (check interval, webhook URL, etc.).
CREATE TABLE IF NOT EXISTS settings (
    key        TEXT PRIMARY KEY,
    value      TEXT NOT NULL,
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

-- Per-service UI preferences that must survive restarts, e.g. the sidecar
-- auto-hide unhide toggle (Phase 2). Keyed by stack + service name.
CREATE TABLE IF NOT EXISTS service_prefs (
    stack      TEXT NOT NULL,
    service    TEXT NOT NULL,
    hidden     INTEGER NOT NULL DEFAULT 0,
    updated_at TEXT NOT NULL DEFAULT (datetime('now')),
    PRIMARY KEY (stack, service)
);
