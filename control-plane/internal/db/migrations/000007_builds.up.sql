ALTER TABLE workloads
    ADD COLUMN IF NOT EXISTS source_kind TEXT NOT NULL DEFAULT 'image'
        CHECK (source_kind IN ('image', 'git')),
    ADD COLUMN IF NOT EXISTS git_url TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS git_ref TEXT NOT NULL DEFAULT 'main',
    ADD COLUMN IF NOT EXISTS dockerfile_path TEXT NOT NULL DEFAULT 'Dockerfile',
    ADD COLUMN IF NOT EXISTS latest_build_id UUID;

CREATE TABLE IF NOT EXISTS builds (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    workload_id UUID NOT NULL,
    git_url TEXT NOT NULL,
    git_ref TEXT NOT NULL DEFAULT 'main',
    dockerfile_path TEXT NOT NULL DEFAULT 'Dockerfile',
    status TEXT NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'building', 'pushing', 'succeeded', 'failed')),
    image TEXT NOT NULL DEFAULT '',
    assigned_node_id UUID REFERENCES nodes(id) ON DELETE SET NULL,
    last_error TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    FOREIGN KEY (organization_id, workload_id) REFERENCES workloads(organization_id, id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS build_logs (
    id BIGSERIAL PRIMARY KEY,
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    build_id UUID NOT NULL REFERENCES builds(id) ON DELETE CASCADE,
    line TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS build_secrets (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    workload_id UUID NOT NULL,
    name TEXT NOT NULL,
    ciphertext BYTEA NOT NULL,
    nonce BYTEA NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (organization_id, workload_id, name),
    FOREIGN KEY (organization_id, workload_id) REFERENCES workloads(organization_id, id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS webhook_tokens (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    workload_id UUID NOT NULL,
    token_hash TEXT NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    FOREIGN KEY (organization_id, workload_id) REFERENCES workloads(organization_id, id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS builds_workload_idx ON builds (workload_id, created_at DESC);
CREATE INDEX IF NOT EXISTS builds_node_status_idx ON builds (assigned_node_id, status);
CREATE INDEX IF NOT EXISTS build_logs_build_idx ON build_logs (build_id, id DESC);
CREATE INDEX IF NOT EXISTS workloads_git_url_idx ON workloads (organization_id, git_url) WHERE source_kind = 'git';

ALTER TABLE builds ENABLE ROW LEVEL SECURITY;
ALTER TABLE builds FORCE ROW LEVEL SECURITY;
ALTER TABLE build_logs ENABLE ROW LEVEL SECURITY;
ALTER TABLE build_logs FORCE ROW LEVEL SECURITY;
ALTER TABLE build_secrets ENABLE ROW LEVEL SECURITY;
ALTER TABLE build_secrets FORCE ROW LEVEL SECURITY;
ALTER TABLE webhook_tokens ENABLE ROW LEVEL SECURITY;
ALTER TABLE webhook_tokens FORCE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS builds_tenant_policy ON builds;
CREATE POLICY builds_tenant_policy ON builds USING (
    organization_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid
) WITH CHECK (
    organization_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid
);

DROP POLICY IF EXISTS build_logs_tenant_policy ON build_logs;
CREATE POLICY build_logs_tenant_policy ON build_logs USING (
    organization_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid
) WITH CHECK (
    organization_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid
);

DROP POLICY IF EXISTS build_secrets_tenant_policy ON build_secrets;
CREATE POLICY build_secrets_tenant_policy ON build_secrets USING (
    organization_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid
) WITH CHECK (
    organization_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid
);

DROP POLICY IF EXISTS webhook_tokens_tenant_policy ON webhook_tokens;
CREATE POLICY webhook_tokens_tenant_policy ON webhook_tokens USING (
    organization_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid
) WITH CHECK (
    organization_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid
);

GRANT SELECT, INSERT, UPDATE, DELETE ON builds, build_logs, build_secrets, webhook_tokens TO norix_app;
GRANT USAGE, SELECT ON SEQUENCE build_logs_id_seq TO norix_app;
