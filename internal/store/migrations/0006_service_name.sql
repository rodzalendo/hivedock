-- 0006: per-service custom display name for the dashboard card. Empty = unset
-- (fall back to the label / humanized automatic name).
ALTER TABLE service_prefs ADD COLUMN display_name TEXT NOT NULL DEFAULT '';
