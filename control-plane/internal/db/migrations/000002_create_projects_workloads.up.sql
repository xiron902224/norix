CREATE TABLE projects (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (organization_id, name),
    UNIQUE (organization_id, id)
);

CREATE TABLE workloads (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    project_id UUID NOT NULL,
    name TEXT NOT NULL,
    runtime TEXT NOT NULL CHECK (runtime IN ('container','docker-compose','microvm','wasm','static-site')),
    image_or_source TEXT NOT NULL,
    env_vars JSONB NOT NULL DEFAULT '{}'::jsonb,
    cpu_shares INTEGER NOT NULL DEFAULT 1024 CHECK (cpu_shares > 0),
    memory_limit_mb BIGINT NOT NULL DEFAULT 512 CHECK (memory_limit_mb > 0),
    domain TEXT,
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','building','deploying','running','stopped','failed')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    FOREIGN KEY (organization_id, project_id) REFERENCES projects(organization_id, id) ON DELETE CASCADE,
    UNIQUE (organization_id, project_id, name)
);

ALTER TABLE projects ENABLE ROW LEVEL SECURITY;
ALTER TABLE projects FORCE ROW LEVEL SECURITY;
ALTER TABLE workloads ENABLE ROW LEVEL SECURITY;
ALTER TABLE workloads FORCE ROW LEVEL SECURITY;

CREATE POLICY projects_tenant_policy ON projects USING (
    organization_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid
) WITH CHECK (
    organization_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid
);
CREATE POLICY workloads_tenant_policy ON workloads USING (
    organization_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid
) WITH CHECK (
    organization_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid
);

CREATE INDEX workloads_project_idx ON workloads(project_id);
GRANT SELECT, INSERT, UPDATE, DELETE ON projects, workloads TO norix_app;
