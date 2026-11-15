-- Real TOTP (AES nonce) + OIDC login state for authorization-code flow.
ALTER TABLE user_totp_secrets
    ADD COLUMN IF NOT EXISTS secret_nonce BYTEA NOT NULL DEFAULT ''::bytea;

-- Previous stub stored bcrypt hashes which cannot verify TOTP; force re-enrollment.
UPDATE user_totp_secrets SET enabled = FALSE, verified_at = NULL, secret_enc = '', secret_nonce = ''::bytea
WHERE secret_nonce = ''::bytea OR octet_length(secret_nonce) = 0;

CREATE TABLE IF NOT EXISTS oidc_login_states (
    state TEXT PRIMARY KEY,
    nonce TEXT NOT NULL,
    provider_id UUID NOT NULL REFERENCES sso_providers(id) ON DELETE CASCADE,
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS oidc_login_states_expires_idx ON oidc_login_states (expires_at);

GRANT SELECT, INSERT, DELETE ON oidc_login_states TO norix_app;
