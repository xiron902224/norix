-- S14/GA: TTL + GC for PR preview environments.

ALTER TABLE pr_previews
    ADD COLUMN IF NOT EXISTS ttl_seconds INTEGER NOT NULL DEFAULT 86400
        CHECK (ttl_seconds >= 60 AND ttl_seconds <= 604800),
    ADD COLUMN IF NOT EXISTS expires_at TIMESTAMPTZ NOT NULL DEFAULT (NOW() + INTERVAL '24 hours');

CREATE INDEX IF NOT EXISTS pr_previews_expires_at_idx ON pr_previews (expires_at)
    WHERE status NOT IN ('destroyed', 'failed');

COMMENT ON COLUMN pr_previews.ttl_seconds IS 'Preview lifetime; GC marks expired rows destroyed';
