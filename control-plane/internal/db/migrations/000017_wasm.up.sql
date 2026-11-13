-- S15 wasm functions (control-plane registry; agents reconcile later).

CREATE TABLE IF NOT EXISTS wasm_functions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    image_ref TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'ready', 'failed')),
    fuel_limit BIGINT NOT NULL DEFAULT 100000000,
    memory_limit_mb INTEGER NOT NULL DEFAULT 128,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (organization_id, project_id, name)
);

CREATE INDEX IF NOT EXISTS wasm_functions_org_project_idx ON wasm_functions (organization_id, project_id);

ALTER TABLE wasm_functions ENABLE ROW LEVEL SECURITY;
ALTER TABLE wasm_functions FORCE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS wasm_functions_tenant_policy ON wasm_functions;
CREATE POLICY wasm_functions_tenant_policy ON wasm_functions USING (
    organization_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid
) WITH CHECK (
    organization_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid
);

GRANT SELECT, INSERT, UPDATE, DELETE ON wasm_functions TO norix_app;
