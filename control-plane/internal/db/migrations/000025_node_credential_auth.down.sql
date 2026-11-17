DROP INDEX IF EXISTS nodes_client_cert_sha256_idx;
DROP INDEX IF EXISTS nodes_previous_token_hash_idx;

ALTER TABLE nodes
    DROP COLUMN IF EXISTS token_revoked_at,
    DROP COLUMN IF EXISTS client_cert_sha256,
    DROP COLUMN IF EXISTS credential_rotate_requested_at,
    DROP COLUMN IF EXISTS previous_token_expires_at,
    DROP COLUMN IF EXISTS previous_token_hash;
