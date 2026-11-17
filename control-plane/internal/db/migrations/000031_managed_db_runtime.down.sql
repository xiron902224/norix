ALTER TABLE managed_databases
    DROP COLUMN IF EXISTS host_port,
    DROP COLUMN IF EXISTS container_id,
    DROP COLUMN IF EXISTS connection_uri,
    DROP COLUMN IF EXISTS last_error;
