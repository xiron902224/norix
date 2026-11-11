DROP TABLE IF EXISTS workload_logs;
DROP TABLE IF EXISTS deployments;
ALTER TABLE workloads
    DROP COLUMN IF EXISTS desired_generation,
    DROP COLUMN IF EXISTS observed_generation,
    DROP COLUMN IF EXISTS host_port,
    DROP COLUMN IF EXISTS container_port,
    DROP COLUMN IF EXISTS assigned_node_id,
    DROP COLUMN IF EXISTS container_id,
    DROP COLUMN IF EXISTS last_error,
    DROP COLUMN IF EXISTS desired_action;
