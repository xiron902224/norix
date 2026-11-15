DROP INDEX IF EXISTS pr_previews_expires_at_idx;
ALTER TABLE pr_previews DROP COLUMN IF EXISTS expires_at;
ALTER TABLE pr_previews DROP COLUMN IF EXISTS ttl_seconds;
