DROP TABLE IF EXISTS workload_replicas;

ALTER TABLE workloads
    DROP COLUMN IF EXISTS replica_count,
    DROP COLUMN IF EXISTS schedule_strategy,
    DROP COLUMN IF EXISTS require_kvm,
    DROP COLUMN IF EXISTS preferred_zone,
    DROP COLUMN IF EXISTS affinity_labels,
    DROP COLUMN IF EXISTS anti_affinity_labels;

ALTER TABLE nodes
    DROP COLUMN IF EXISTS role,
    DROP COLUMN IF EXISTS cordoned,
    DROP COLUMN IF EXISTS draining,
    DROP COLUMN IF EXISTS zone,
    DROP COLUMN IF EXISTS labels;
