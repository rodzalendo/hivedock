-- 0007: per-service custom link URL for the dashboard card. Empty = unset
-- (fall back to the automatic port-derived URL). This is the reliable escape
-- hatch for services whose ports the heuristic can't see: host-network
-- containers (e.g. jellyfin) and services sharing another's network stack
-- (e.g. qbittorrent behind gluetun).
ALTER TABLE service_prefs ADD COLUMN url TEXT NOT NULL DEFAULT '';
