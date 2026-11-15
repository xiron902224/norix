DROP TABLE IF EXISTS sandboxes;

ALTER TABLE workloads DROP COLUMN IF EXISTS execution_runtime;
ALTER TABLE workloads DROP COLUMN IF EXISTS isolation_mode;
