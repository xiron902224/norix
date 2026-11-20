-- #27: per-project notification channels and a delivery outbox.
-- The target (webhook URL or email address) is encrypted. The outbox row is
-- written in the same transaction as the event, then a worker delivers it.

CREATE TABLE IF NOT EXISTS notification_channels (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    kind TEXT NOT NULL,
    target_ciphertext BYTEA NOT NULL,
    target_nonce BYTEA NOT NULL,
    events JSONB NOT NULL DEFAULT '["deploy.succeeded","deploy.failed","node.down"]'::jsonb,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT notification_channels_kind_chk CHECK (kind IN ('webhook', 'slack', 'discord', 'email'))
);

CREATE INDEX IF NOT EXISTS notification_channels_project_idx
    ON notification_channels (project_id);

CREATE TABLE IF NOT EXISTS notification_outbox (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    channel_id UUID NOT NULL REFERENCES notification_channels(id) ON DELETE CASCADE,
    event_kind TEXT NOT NULL,
    dedupe_key TEXT NOT NULL,
    payload JSONB NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    attempts INT NOT NULL DEFAULT 0,
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_error TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (channel_id, dedupe_key),
    CONSTRAINT notification_outbox_status_chk CHECK (status IN ('pending', 'sending', 'delivered', 'failed'))
);

CREATE INDEX IF NOT EXISTS notification_outbox_due_idx
    ON notification_outbox (organization_id, status, next_attempt_at);

ALTER TABLE notification_channels ENABLE ROW LEVEL SECURITY;
ALTER TABLE notification_channels FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS notification_channels_tenant_policy ON notification_channels;
CREATE POLICY notification_channels_tenant_policy ON notification_channels USING (
    organization_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid
) WITH CHECK (
    organization_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid
);

ALTER TABLE notification_outbox ENABLE ROW LEVEL SECURITY;
ALTER TABLE notification_outbox FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS notification_outbox_tenant_policy ON notification_outbox;
CREATE POLICY notification_outbox_tenant_policy ON notification_outbox USING (
    organization_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid
) WITH CHECK (
    organization_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid
);

GRANT SELECT, INSERT, UPDATE, DELETE ON notification_channels TO norix_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON notification_outbox TO norix_app;
