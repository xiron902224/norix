-- #14: immutable workload release history for rollback
CREATE TABLE IF NOT EXISTS workload_releases (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    workload_id UUID NOT NULL,
    deployment_id UUID,
    generation BIGINT NOT NULL DEFAULT 0,
    image_or_source TEXT NOT NULL,
    image_digest TEXT NOT NULL DEFAULT '',
    env_vars JSONB NOT NULL DEFAULT '{}'::jsonb,
    container_port INTEGER NOT NULL DEFAULT 80,
    cpu_shares INTEGER NOT NULL DEFAULT 1024,
    memory_limit_mb BIGINT NOT NULL DEFAULT 512,
    source TEXT NOT NULL DEFAULT 'deploy'
        CHECK (source IN ('deploy', 'rollback', 'auto-rollback')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    FOREIGN KEY (organization_id, workload_id) REFERENCES workloads(organization_id, id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS workload_releases_workload_idx
    ON workload_releases (workload_id, created_at DESC);

ALTER TABLE workload_releases ENABLE ROW LEVEL SECURITY;
ALTER TABLE workload_releases FORCE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS workload_releases_tenant_policy ON workload_releases;
CREATE POLICY workload_releases_tenant_policy ON workload_releases USING (
    organization_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid
) WITH CHECK (
    organization_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid
);

GRANT SELECT, INSERT, UPDATE, DELETE ON workload_releases TO norix_app;
