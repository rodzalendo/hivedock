-- 0004_update_source: changelog source URL (org.opencontainers.image.source)
-- captured during an update check, for a "changelog" link in the Updates view.

ALTER TABLE image_checks ADD COLUMN source TEXT;
