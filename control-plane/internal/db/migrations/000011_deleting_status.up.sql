-- Soft-delete workloads: mark deleting so agents can stop containers before purge.

ALTER TABLE workloads DROP CONSTRAINT IF EXISTS workloads_status_check;
ALTER TABLE workloads
    ADD CONSTRAINT workloads_status_check
    CHECK (status IN ('pending', 'building', 'deploying', 'running', 'stopped', 'failed', 'deleting'));

ALTER TABLE workload_replicas DROP CONSTRAINT IF EXISTS workload_replicas_status_check;
ALTER TABLE workload_replicas
    ADD CONSTRAINT workload_replicas_status_check
    CHECK (status IN ('pending', 'building', 'deploying', 'running', 'stopped', 'failed', 'deleting'));
