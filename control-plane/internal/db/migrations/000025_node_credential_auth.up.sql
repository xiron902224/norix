-- #13: node token rotate/revoke + optional mTLS fingerprint binding
ALTER TABLE nodes
    ADD COLUMN IF NOT EXISTS previous_token_hash BYTEA,
    ADD COLUMN IF NOT EXISTS previous_token_expires_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS credential_rotate_requested_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS client_cert_sha256 TEXT,
    ADD COLUMN IF NOT EXISTS token_revoked_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS nodes_previous_token_hash_idx
    ON nodes (previous_token_hash)
    WHERE previous_token_hash IS NOT NULL;

CREATE INDEX IF NOT EXISTS nodes_client_cert_sha256_idx
    ON nodes (client_cert_sha256)
    WHERE client_cert_sha256 IS NOT NULL AND client_cert_sha256 <> '';
