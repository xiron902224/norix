-- #22: user personal access tokens (hashed at rest)
CREATE TABLE IF NOT EXISTS user_api_tokens (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    user_id UUID NOT NULL,
    name TEXT NOT NULL,
    token_prefix TEXT NOT NULL,
    token_hash BYTEA NOT NULL UNIQUE,
    scopes TEXT[] NOT NULL DEFAULT ARRAY['*']::text[],
    expires_at TIMESTAMPTZ,
    revoked_at TIMESTAMPTZ,
    last_used_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    FOREIGN KEY (organization_id, user_id) REFERENCES users(organization_id, id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS user_api_tokens_user_idx
    ON user_api_tokens (organization_id, user_id, created_at DESC);

ALTER TABLE user_api_tokens ENABLE ROW LEVEL SECURITY;
ALTER TABLE user_api_tokens FORCE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS user_api_tokens_tenant_policy ON user_api_tokens;
CREATE POLICY user_api_tokens_tenant_policy ON user_api_tokens USING (
    organization_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid
) WITH CHECK (
    organization_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid
);

GRANT SELECT, INSERT, UPDATE, DELETE ON user_api_tokens TO norix_app;
