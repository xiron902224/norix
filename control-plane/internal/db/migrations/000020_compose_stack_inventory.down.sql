DROP INDEX IF EXISTS workloads_stack_idx;
ALTER TABLE workloads
    DROP COLUMN IF EXISTS compose_spec,
    DROP COLUMN IF EXISTS compose_service,
    DROP COLUMN IF EXISTS stack_id;
ALTER TABLE nodes
    DROP COLUMN IF EXISTS reported_workload_ids;
