-- S11 micro-VM isolation mode on workloads.
ALTER TABLE workloads
    ADD COLUMN IF NOT EXISTS isolation_mode TEXT NOT NULL DEFAULT 'container'
        CHECK (isolation_mode IN ('container', 'microvm_preferred', 'microvm_required')),
    ADD COLUMN IF NOT EXISTS execution_runtime TEXT NOT NULL DEFAULT 'docker'
        CHECK (execution_runtime IN ('docker', 'containerd', 'microvm'));

UPDATE workloads SET require_kvm = TRUE WHERE isolation_mode = 'microvm_required';

-- S12 ephemeral sandboxes.
CREATE TABLE IF NOT EXISTS sandboxes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    template TEXT NOT NULL DEFAULT 'blank',
    status TEXT NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'ready', 'running', 'destroyed', 'expired')),
    ttl_seconds INTEGER NOT NULL DEFAULT 3600,
    cpu INTEGER NOT NULL DEFAULT 1,
    memory_mb INTEGER NOT NULL DEFAULT 512,
    egress_mode TEXT NOT NULL DEFAULT 'restricted'
        CHECK (egress_mode IN ('restricted', 'open', 'deny')),
    expires_at TIMESTAMPTZ NOT NULL,
    pool_ready BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (organization_id, project_id, name)
);

CREATE INDEX IF NOT EXISTS sandboxes_org_expires_idx ON sandboxes (organization_id, expires_at);

ALTER TABLE sandboxes ENABLE ROW LEVEL SECURITY;
ALTER TABLE sandboxes FORCE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS sandboxes_tenant_policy ON sandboxes;
CREATE POLICY sandboxes_tenant_policy ON sandboxes USING (
    organization_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid
) WITH CHECK (
    organization_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid
);

GRANT SELECT, INSERT, UPDATE, DELETE ON sandboxes TO norix_app;
