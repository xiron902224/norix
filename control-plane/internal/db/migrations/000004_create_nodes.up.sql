CREATE TABLE join_tokens (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    token_hash BYTEA NOT NULL UNIQUE,
    node_name TEXT,
    expires_at TIMESTAMPTZ NOT NULL,
    consumed_at TIMESTAMPTZ,
    created_by_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE nodes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'offline'
        CHECK (status IN ('online', 'offline', 'degraded')),
    token_hash BYTEA NOT NULL UNIQUE,
    last_seen_at TIMESTAMPTZ,
    cpu_cores INTEGER NOT NULL DEFAULT 0 CHECK (cpu_cores >= 0),
    memory_total_mb BIGINT NOT NULL DEFAULT 0 CHECK (memory_total_mb >= 0),
    docker_socket BOOLEAN NOT NULL DEFAULT FALSE,
    containerd_socket BOOLEAN NOT NULL DEFAULT FALSE,
    kvm_level INTEGER NOT NULL DEFAULT 0 CHECK (kvm_level BETWEEN 0 AND 2),
    hostname TEXT NOT NULL DEFAULT '',
    agent_version TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (organization_id, name),
    UNIQUE (organization_id, id)
);

ALTER TABLE join_tokens ENABLE ROW LEVEL SECURITY;
ALTER TABLE join_tokens FORCE ROW LEVEL SECURITY;
ALTER TABLE nodes ENABLE ROW LEVEL SECURITY;
ALTER TABLE nodes FORCE ROW LEVEL SECURITY;

CREATE POLICY join_tokens_tenant_policy ON join_tokens USING (
    organization_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid
) WITH CHECK (
    organization_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid
);

CREATE POLICY nodes_tenant_policy ON nodes USING (
    organization_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid
) WITH CHECK (
    organization_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid
);

CREATE INDEX join_tokens_org_expires_idx ON join_tokens (organization_id, expires_at);
CREATE INDEX nodes_org_last_seen_idx ON nodes (organization_id, last_seen_at DESC NULLS LAST);

GRANT SELECT, INSERT, UPDATE, DELETE ON join_tokens, nodes TO norix_app;
