-- #144–#147: project network is derived from project_id (no column).
-- Public services, domains and observed volumes live on the stack.
-- Variables are encrypted per name. Generated secrets are inserted once.

ALTER TABLE compose_stacks
    ADD COLUMN IF NOT EXISTS public_services JSONB NOT NULL DEFAULT '[]'::jsonb,
    ADD COLUMN IF NOT EXISTS domains JSONB NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN IF NOT EXISTS volumes JSONB NOT NULL DEFAULT '[]'::jsonb,
    ADD COLUMN IF NOT EXISTS purge_volumes BOOLEAN NOT NULL DEFAULT FALSE,
    -- Set when a stack's routes may have changed; the next edge plan clears it.
    ADD COLUMN IF NOT EXISTS edge_pending BOOLEAN NOT NULL DEFAULT FALSE;

CREATE TABLE IF NOT EXISTS compose_stack_variables (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    stack_id UUID NOT NULL REFERENCES compose_stacks(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    ciphertext BYTEA NOT NULL,
    nonce BYTEA NOT NULL,
    generated BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (stack_id, name)
);

CREATE INDEX IF NOT EXISTS compose_stack_variables_stack_idx
    ON compose_stack_variables (stack_id);

ALTER TABLE compose_stack_variables ENABLE ROW LEVEL SECURITY;
ALTER TABLE compose_stack_variables FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS compose_stack_variables_tenant_policy ON compose_stack_variables;
CREATE POLICY compose_stack_variables_tenant_policy ON compose_stack_variables USING (
    organization_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid
) WITH CHECK (
    organization_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid
);

GRANT SELECT, INSERT, UPDATE, DELETE ON compose_stack_variables TO norix_app;
