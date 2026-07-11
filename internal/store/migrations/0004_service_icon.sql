-- 0004: per-service custom icon override (a URL or a dashboard-icons slug).
-- Lets users fix or set the icon for any homepage card. Empty string = unset.
ALTER TABLE service_prefs ADD COLUMN icon TEXT NOT NULL DEFAULT '';
