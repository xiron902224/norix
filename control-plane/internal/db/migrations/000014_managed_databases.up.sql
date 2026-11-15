-- S10 managed databases (postgres/mysql/redis/mongodb records on nodes).

CREATE TABLE IF NOT EXISTS managed_databases (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    engine TEXT NOT NULL CHECK (engine IN ('postgres', 'mysql', 'redis', 'mongodb')),
    name TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'provisioning'
        CHECK (status IN ('provisioning', 'ready', 'failed', 'backing_up', 'restoring')),
    node_id UUID REFERENCES nodes(id) ON DELETE SET NULL,
    volume_path TEXT NOT NULL DEFAULT '',
    connection_secret_enc BYTEA,
    connection_secret_nonce BYTEA,
    backup_schedule TEXT NOT NULL DEFAULT '',
    last_backup_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (organization_id, project_id, name)
);

CREATE INDEX IF NOT EXISTS managed_databases_org_project_idx ON managed_databases (organization_id, project_id);

ALTER TABLE managed_databases ENABLE ROW LEVEL SECURITY;
ALTER TABLE managed_databases FORCE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS managed_databases_tenant_policy ON managed_databases;
CREATE POLICY managed_databases_tenant_policy ON managed_databases USING (
    organization_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid
) WITH CHECK (
    organization_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid
);

GRANT SELECT, INSERT, UPDATE, DELETE ON managed_databases TO norix_app;
