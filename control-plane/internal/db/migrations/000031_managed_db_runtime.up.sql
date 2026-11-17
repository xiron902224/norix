-- #20: track real managed DB containers
ALTER TABLE managed_databases
    ADD COLUMN IF NOT EXISTS host_port INT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS container_id TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS connection_uri TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS last_error TEXT NOT NULL DEFAULT '';

COMMENT ON COLUMN managed_databases.connection_uri IS
    'DSN with password redacted as ****** for UI; plaintext lives encrypted in connection_secret_*';
