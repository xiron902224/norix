ALTER TABLE workload_replicas DROP COLUMN IF EXISTS surge_host_port;
ALTER TABLE workloads DROP COLUMN IF EXISTS max_unavailable;
ALTER TABLE workloads DROP COLUMN IF EXISTS max_surge;
ALTER TABLE workloads DROP COLUMN IF EXISTS update_strategy;
