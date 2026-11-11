CREATE TABLE IF NOT EXISTS domain_routes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    workload_id UUID NOT NULL,
    hostname TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'provisioning', 'active', 'failed', 'deleting')),
    tls_mode TEXT NOT NULL DEFAULT 'internal'
        CHECK (tls_mode IN ('internal', 'acme')),
    edge_generation BIGINT NOT NULL DEFAULT 0,
    last_error TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (organization_id, hostname),
    FOREIGN KEY (organization_id, workload_id) REFERENCES workloads(organization_id, id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS edge_snapshots (
    id BIGSERIAL PRIMARY KEY,
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    generation BIGINT NOT NULL,
    config_json TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'active', 'rolled_back', 'failed')),
    message TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (organization_id, generation)
);

CREATE INDEX IF NOT EXISTS domain_routes_workload_idx ON domain_routes (workload_id);
CREATE INDEX IF NOT EXISTS domain_routes_status_idx ON domain_routes (organization_id, status);
CREATE INDEX IF NOT EXISTS edge_snapshots_org_gen_idx ON edge_snapshots (organization_id, generation DESC);

ALTER TABLE domain_routes ENABLE ROW LEVEL SECURITY;
ALTER TABLE domain_routes FORCE ROW LEVEL SECURITY;
ALTER TABLE edge_snapshots ENABLE ROW LEVEL SECURITY;
ALTER TABLE edge_snapshots FORCE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS domain_routes_tenant_policy ON domain_routes;
CREATE POLICY domain_routes_tenant_policy ON domain_routes USING (
    organization_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid
) WITH CHECK (
    organization_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid
);

DROP POLICY IF EXISTS edge_snapshots_tenant_policy ON edge_snapshots;
CREATE POLICY edge_snapshots_tenant_policy ON edge_snapshots USING (
    organization_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid
) WITH CHECK (
    organization_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid
);

GRANT SELECT, INSERT, UPDATE, DELETE ON domain_routes, edge_snapshots TO norix_app;
GRANT USAGE, SELECT ON SEQUENCE edge_snapshots_id_seq TO norix_app;
