-- Workload env_vars are stored as an AES-GCM envelope when NORIX_BUILD_SECRETS_KEY is set:
--   {"__norix_env":1,"entries":{"NAME":{"c":"<b64>","n":"<b64>"}}}
-- Legacy plaintext JSONB maps are re-encrypted at control-plane startup (BackfillEncryptWorkloadEnvs).
COMMENT ON COLUMN workloads.env_vars IS 'Workload environment; AES-GCM envelope (__norix_env) or legacy plaintext map pending backfill';
