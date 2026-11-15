REVOKE SELECT, INSERT, DELETE ON oidc_login_states FROM norix_app;
DROP TABLE IF EXISTS oidc_login_states;
ALTER TABLE user_totp_secrets DROP COLUMN IF EXISTS secret_nonce;
