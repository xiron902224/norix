ALTER TABLE workload_replicas DROP COLUMN IF EXISTS probe_status;
ALTER TABLE workload_replicas DROP COLUMN IF EXISTS ready;
ALTER TABLE workloads DROP COLUMN IF EXISTS probe_spec;
