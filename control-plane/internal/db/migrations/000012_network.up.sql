-- S8 node networking: WireGuard mesh, DNS, project isolation, egress, shared certs.

ALTER TABLE nodes
    ADD COLUMN IF NOT EXISTS wg_public_key TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS wg_private_key_enc BYTEA,
    ADD COLUMN IF NOT EXISTS wg_private_key_nonce BYTEA,
    ADD COLUMN IF NOT EXISTS wg_address TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS wg_listen_port INTEGER NOT NULL DEFAULT 51820,
    ADD COLUMN IF NOT EXISTS network_status TEXT NOT NULL DEFAULT 'pending'
        CHECK (network_status IN ('pending', 'ready', 'degraded', 'error')),
    ADD COLUMN IF NOT EXISTS network_last_error TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS network_generation BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS network_observed_generation BIGINT NOT NULL DEFAULT 0;

CREATE TABLE IF NOT EXISTS project_network (
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    project_id UUID PRIMARY KEY REFERENCES projects(id) ON DELETE CASCADE,
    isolation_mark INTEGER NOT NULL,
    subnet_cidr TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (organization_id, isolation_mark)
);

CREATE TABLE IF NOT EXISTS workload_egress_policies (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    workload_id UUID NOT NULL REFERENCES workloads(id) ON DELETE CASCADE,
    mode TEXT NOT NULL DEFAULT 'allow'
        CHECK (mode IN ('allow', 'deny', 'allowlist')),
    allowlist JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (workload_id)
);

CREATE TABLE IF NOT EXISTS network_dns_records (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    project_id UUID REFERENCES projects(id) ON DELETE CASCADE,
    workload_id UUID REFERENCES workloads(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    record_type TEXT NOT NULL DEFAULT 'A' CHECK (record_type IN ('A', 'AAAA', 'CNAME')),
    value TEXT NOT NULL,
    ttl_seconds INTEGER NOT NULL DEFAULT 30,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (organization_id, name, record_type)
);

CREATE TABLE IF NOT EXISTS platform_certificates (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    hostname TEXT NOT NULL,
    cert_pem TEXT NOT NULL,
    key_pem_enc BYTEA NOT NULL,
    key_nonce BYTEA NOT NULL,
    not_before TIMESTAMPTZ NOT NULL,
    not_after TIMESTAMPTZ NOT NULL,
    issuer TEXT NOT NULL DEFAULT 'norix-internal',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (organization_id, hostname)
);

CREATE INDEX IF NOT EXISTS network_dns_org_name_idx ON network_dns_records (organization_id, name);
CREATE INDEX IF NOT EXISTS platform_certificates_org_idx ON platform_certificates (organization_id, not_after);

ALTER TABLE project_network ENABLE ROW LEVEL SECURITY;
ALTER TABLE project_network FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS project_network_tenant_policy ON project_network;
CREATE POLICY project_network_tenant_policy ON project_network USING (
    organization_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid
) WITH CHECK (
    organization_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid
);

ALTER TABLE workload_egress_policies ENABLE ROW LEVEL SECURITY;
ALTER TABLE workload_egress_policies FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS workload_egress_policies_tenant_policy ON workload_egress_policies;
CREATE POLICY workload_egress_policies_tenant_policy ON workload_egress_policies USING (
    organization_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid
) WITH CHECK (
    organization_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid
);

ALTER TABLE network_dns_records ENABLE ROW LEVEL SECURITY;
ALTER TABLE network_dns_records FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS network_dns_records_tenant_policy ON network_dns_records;
CREATE POLICY network_dns_records_tenant_policy ON network_dns_records USING (
    organization_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid
) WITH CHECK (
    organization_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid
);

ALTER TABLE platform_certificates ENABLE ROW LEVEL SECURITY;
ALTER TABLE platform_certificates FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS platform_certificates_tenant_policy ON platform_certificates;
CREATE POLICY platform_certificates_tenant_policy ON platform_certificates USING (
    organization_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid
) WITH CHECK (
    organization_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid
);

GRANT SELECT, INSERT, UPDATE, DELETE ON project_network TO norix_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON workload_egress_policies TO norix_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON network_dns_records TO norix_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON platform_certificates TO norix_app;
