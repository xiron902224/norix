DROP POLICY IF EXISTS user_totp_secrets_tenant_policy ON user_totp_secrets;
DROP POLICY IF EXISTS sso_providers_tenant_policy ON sso_providers;
DROP POLICY IF EXISTS usage_records_tenant_policy ON usage_records;
DROP POLICY IF EXISTS team_quotas_tenant_policy ON team_quotas;
DROP POLICY IF EXISTS team_members_tenant_policy ON team_members;
DROP POLICY IF EXISTS team_roles_tenant_policy ON team_roles;
DROP POLICY IF EXISTS teams_tenant_policy ON teams;

DROP TABLE IF EXISTS user_totp_secrets;
DROP TABLE IF EXISTS sso_providers;
DROP TABLE IF EXISTS usage_records;
DROP TABLE IF EXISTS team_quotas;
DROP TABLE IF EXISTS team_members;
DROP TABLE IF EXISTS team_roles;
DROP TABLE IF EXISTS teams;

ALTER TABLE projects DROP COLUMN IF EXISTS team_id;
ALTER TABLE organizations DROP COLUMN IF EXISTS mandatory_2fa;
