-- S14 pull-request preview environments.

CREATE TABLE IF NOT EXISTS pr_previews (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    repo TEXT NOT NULL,
    pr_number INTEGER NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'building', 'ready', 'failed', 'destroyed')),
    preview_url TEXT NOT NULL DEFAULT '',
    github_comment_id BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (organization_id, repo, pr_number)
);

CREATE INDEX IF NOT EXISTS pr_previews_org_project_idx ON pr_previews (organization_id, project_id);

ALTER TABLE pr_previews ENABLE ROW LEVEL SECURITY;
ALTER TABLE pr_previews FORCE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS pr_previews_tenant_policy ON pr_previews;
CREATE POLICY pr_previews_tenant_policy ON pr_previews USING (
    organization_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid
) WITH CHECK (
    organization_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid
);

GRANT SELECT, INSERT, UPDATE, DELETE ON pr_previews TO norix_app;
