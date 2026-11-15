-- S16 teams, quotas, usage metering, SSO, and 2FA storage.
ALTER TABLE organizations
    ADD COLUMN IF NOT EXISTS mandatory_2fa BOOLEAN NOT NULL DEFAULT FALSE;

CREATE TABLE IF NOT EXISTS teams (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    slug TEXT NOT NULL,
    is_default BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (organization_id, slug),
    UNIQUE (organization_id, id)
);

CREATE TABLE IF NOT EXISTS team_roles (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    permissions JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (organization_id, name)
);

CREATE TABLE IF NOT EXISTS team_members (
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    team_id UUID NOT NULL,
    user_id UUID NOT NULL,
    role TEXT NOT NULL DEFAULT 'member',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (team_id, user_id),
    FOREIGN KEY (organization_id, team_id) REFERENCES teams(organization_id, id) ON DELETE CASCADE,
    FOREIGN KEY (organization_id, user_id) REFERENCES users(organization_id, id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS team_quotas (
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    team_id UUID PRIMARY KEY REFERENCES teams(id) ON DELETE CASCADE,
    max_cpu INTEGER NOT NULL DEFAULT 0 CHECK (max_cpu >= 0),
    max_memory_mb BIGINT NOT NULL DEFAULT 0 CHECK (max_memory_mb >= 0),
    max_disk_gb BIGINT NOT NULL DEFAULT 0 CHECK (max_disk_gb >= 0),
    max_sandboxes INTEGER NOT NULL DEFAULT 0 CHECK (max_sandboxes >= 0),
    max_previews INTEGER NOT NULL DEFAULT 0 CHECK (max_previews >= 0),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    FOREIGN KEY (organization_id, team_id) REFERENCES teams(organization_id, id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS usage_records (
    id BIGSERIAL PRIMARY KEY,
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    team_id UUID NOT NULL,
    metric TEXT NOT NULL,
    quantity NUMERIC NOT NULL DEFAULT 0,
    unit TEXT NOT NULL DEFAULT '',
    recorded_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    FOREIGN KEY (organization_id, team_id) REFERENCES teams(organization_id, id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS usage_records_team_metric_idx
    ON usage_records (organization_id, team_id, metric, recorded_at DESC);

CREATE TABLE IF NOT EXISTS sso_providers (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    kind TEXT NOT NULL CHECK (kind IN ('oidc', 'saml')),
    issuer_url TEXT NOT NULL DEFAULT '',
    client_id TEXT NOT NULL DEFAULT '',
    client_secret_enc TEXT NOT NULL DEFAULT '',
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    enabled BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (organization_id, name)
);

CREATE TABLE IF NOT EXISTS user_totp_secrets (
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    user_id UUID PRIMARY KEY,
    secret_enc TEXT NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT FALSE,
    verified_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    FOREIGN KEY (organization_id, user_id) REFERENCES users(organization_id, id) ON DELETE CASCADE
);

ALTER TABLE projects
    ADD COLUMN IF NOT EXISTS team_id UUID;

DO $$
DECLARE
    org RECORD;
    default_team UUID;
BEGIN
    FOR org IN SELECT id FROM organizations LOOP
        INSERT INTO teams (organization_id, name, slug, is_default)
        VALUES (org.id, 'Default', 'default', TRUE)
        ON CONFLICT (organization_id, slug) DO NOTHING;
        SELECT id INTO default_team
        FROM teams
        WHERE organization_id = org.id AND is_default
        ORDER BY created_at
        LIMIT 1;
        IF default_team IS NOT NULL THEN
            UPDATE projects SET team_id = default_team
            WHERE organization_id = org.id AND team_id IS NULL;
            INSERT INTO team_quotas (organization_id, team_id)
            VALUES (org.id, default_team)
            ON CONFLICT (team_id) DO NOTHING;
            INSERT INTO team_members (organization_id, team_id, user_id, role)
            SELECT u.organization_id, default_team, u.id,
                   CASE WHEN u.role = 'owner' THEN 'owner' WHEN u.role = 'admin' THEN 'admin' ELSE 'member' END
            FROM users u
            WHERE u.organization_id = org.id
            ON CONFLICT DO NOTHING;
        END IF;
    END LOOP;
END $$;

INSERT INTO team_roles (organization_id, name, permissions)
SELECT o.id, 'owner', '["*"]'::jsonb FROM organizations o
ON CONFLICT (organization_id, name) DO NOTHING;
INSERT INTO team_roles (organization_id, name, permissions)
SELECT o.id, 'admin', '["mutate","read"]'::jsonb FROM organizations o
ON CONFLICT (organization_id, name) DO NOTHING;
INSERT INTO team_roles (organization_id, name, permissions)
SELECT o.id, 'member', '["read"]'::jsonb FROM organizations o
ON CONFLICT (organization_id, name) DO NOTHING;

ALTER TABLE teams ENABLE ROW LEVEL SECURITY;
ALTER TABLE teams FORCE ROW LEVEL SECURITY;
ALTER TABLE team_roles ENABLE ROW LEVEL SECURITY;
ALTER TABLE team_roles FORCE ROW LEVEL SECURITY;
ALTER TABLE team_members ENABLE ROW LEVEL SECURITY;
ALTER TABLE team_members FORCE ROW LEVEL SECURITY;
ALTER TABLE team_quotas ENABLE ROW LEVEL SECURITY;
ALTER TABLE team_quotas FORCE ROW LEVEL SECURITY;
ALTER TABLE usage_records ENABLE ROW LEVEL SECURITY;
ALTER TABLE usage_records FORCE ROW LEVEL SECURITY;
ALTER TABLE sso_providers ENABLE ROW LEVEL SECURITY;
ALTER TABLE sso_providers FORCE ROW LEVEL SECURITY;
ALTER TABLE user_totp_secrets ENABLE ROW LEVEL SECURITY;
ALTER TABLE user_totp_secrets FORCE ROW LEVEL SECURITY;

CREATE POLICY teams_tenant_policy ON teams USING (
    organization_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid
) WITH CHECK (
    organization_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid
);
CREATE POLICY team_roles_tenant_policy ON team_roles USING (
    organization_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid
) WITH CHECK (
    organization_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid
);
CREATE POLICY team_members_tenant_policy ON team_members USING (
    organization_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid
) WITH CHECK (
    organization_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid
);
CREATE POLICY team_quotas_tenant_policy ON team_quotas USING (
    organization_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid
) WITH CHECK (
    organization_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid
);
CREATE POLICY usage_records_tenant_policy ON usage_records USING (
    organization_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid
) WITH CHECK (
    organization_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid
);
CREATE POLICY sso_providers_tenant_policy ON sso_providers USING (
    organization_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid
) WITH CHECK (
    organization_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid
);
CREATE POLICY user_totp_secrets_tenant_policy ON user_totp_secrets USING (
    organization_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid
) WITH CHECK (
    organization_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid
);

GRANT SELECT, INSERT, UPDATE, DELETE ON teams, team_roles, team_members, team_quotas, sso_providers, user_totp_secrets TO norix_app;
GRANT SELECT, INSERT ON usage_records TO norix_app;
GRANT USAGE, SELECT ON SEQUENCE usage_records_id_seq TO norix_app;
