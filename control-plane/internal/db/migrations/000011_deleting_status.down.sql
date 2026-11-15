ALTER TABLE workloads DROP CONSTRAINT IF EXISTS workloads_status_check;
ALTER TABLE workloads
    ADD CONSTRAINT workloads_status_check
    CHECK (status IN ('pending', 'building', 'deploying', 'running', 'stopped', 'failed'));

ALTER TABLE workload_replicas DROP CONSTRAINT IF EXISTS workload_replicas_status_check;
ALTER TABLE workload_replicas
    ADD CONSTRAINT workload_replicas_status_check
    CHECK (status IN ('pending', 'building', 'deploying', 'running', 'stopped', 'failed'));

UPDATE workloads SET status = 'stopped' WHERE status = 'deleting';
UPDATE workload_replicas SET status = 'stopped' WHERE status = 'deleting';
