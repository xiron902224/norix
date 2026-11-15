DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'workloads_organization_id_id_key'
    ) THEN
        ALTER TABLE workloads ADD CONSTRAINT workloads_organization_id_id_key UNIQUE (organization_id, id);
    END IF;
END $$;

ALTER TABLE workloads
    ADD COLUMN IF NOT EXISTS desired_generation BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS observed_generation BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS host_port INTEGER,
    ADD COLUMN IF NOT EXISTS container_port INTEGER NOT NULL DEFAULT 80,
    ADD COLUMN IF NOT EXISTS assigned_node_id UUID REFERENCES nodes(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS container_id TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS last_error TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS desired_action TEXT NOT NULL DEFAULT 'none'
        CHECK (desired_action IN ('none', 'run', 'stop'));

CREATE TABLE IF NOT EXISTS deployments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    workload_id UUID NOT NULL,
    node_id UUID,
    status TEXT NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'deploying', 'running', 'failed', 'stopped')),
    host_port INTEGER,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    FOREIGN KEY (organization_id, workload_id) REFERENCES workloads(organization_id, id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS workload_logs (
    id BIGSERIAL PRIMARY KEY,
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    workload_id UUID NOT NULL,
    line TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    FOREIGN KEY (organization_id, workload_id) REFERENCES workloads(organization_id, id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS workloads_assigned_node_idx ON workloads (assigned_node_id, desired_action);
CREATE INDEX IF NOT EXISTS deployments_workload_idx ON deployments (workload_id, created_at DESC);
CREATE INDEX IF NOT EXISTS workload_logs_workload_idx ON workload_logs (workload_id, id DESC);

ALTER TABLE deployments ENABLE ROW LEVEL SECURITY;
ALTER TABLE deployments FORCE ROW LEVEL SECURITY;
ALTER TABLE workload_logs ENABLE ROW LEVEL SECURITY;
ALTER TABLE workload_logs FORCE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS deployments_tenant_policy ON deployments;
CREATE POLICY deployments_tenant_policy ON deployments USING (
    organization_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid
) WITH CHECK (
    organization_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid
);

DROP POLICY IF EXISTS workload_logs_tenant_policy ON workload_logs;
CREATE POLICY workload_logs_tenant_policy ON workload_logs USING (
    organization_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid
) WITH CHECK (
    organization_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid
);

GRANT SELECT, INSERT, UPDATE, DELETE ON deployments, workload_logs TO norix_app;
GRANT USAGE, SELECT ON SEQUENCE workload_logs_id_seq TO norix_app;
