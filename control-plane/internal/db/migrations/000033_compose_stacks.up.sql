-- #143: compose stacks applied as a unit by the agent (docker compose up)
CREATE TABLE IF NOT EXISTS compose_stacks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    git_url TEXT NOT NULL,
    git_ref TEXT NOT NULL DEFAULT 'main',
    compose_path TEXT NOT NULL DEFAULT 'docker-compose.yml',
    assigned_node_id UUID REFERENCES nodes(id) ON DELETE SET NULL,
    desired_action TEXT NOT NULL DEFAULT 'up'
        CHECK (desired_action IN ('up', 'stop', 'down', 'none')),
    desired_generation BIGINT NOT NULL DEFAULT 1,
    observed_generation BIGINT NOT NULL DEFAULT 0,
    status TEXT NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'deploying', 'running', 'degraded', 'stopped', 'failed', 'removing')),
    message TEXT NOT NULL DEFAULT '',
    services JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (project_id, name)
);

CREATE INDEX IF NOT EXISTS compose_stacks_node_idx
    ON compose_stacks (organization_id, assigned_node_id);

CREATE TABLE IF NOT EXISTS compose_stack_logs (
    id BIGSERIAL PRIMARY KEY,
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    stack_id UUID NOT NULL REFERENCES compose_stacks(id) ON DELETE CASCADE,
    service TEXT NOT NULL DEFAULT '',
    line TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS compose_stack_logs_stack_idx
    ON compose_stack_logs (stack_id, id DESC);
CREATE INDEX IF NOT EXISTS compose_stack_logs_created_idx
    ON compose_stack_logs (created_at);

ALTER TABLE compose_stacks ENABLE ROW LEVEL SECURITY;
ALTER TABLE compose_stacks FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS compose_stacks_tenant_policy ON compose_stacks;
CREATE POLICY compose_stacks_tenant_policy ON compose_stacks USING (
    organization_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid
) WITH CHECK (
    organization_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid
);

ALTER TABLE compose_stack_logs ENABLE ROW LEVEL SECURITY;
ALTER TABLE compose_stack_logs FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS compose_stack_logs_tenant_policy ON compose_stack_logs;
CREATE POLICY compose_stack_logs_tenant_policy ON compose_stack_logs USING (
    organization_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid
) WITH CHECK (
    organization_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid
);

GRANT SELECT, INSERT, UPDATE, DELETE ON compose_stacks TO norix_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON compose_stack_logs TO norix_app;
GRANT USAGE, SELECT ON SEQUENCE compose_stack_logs_id_seq TO norix_app;
