-- S7 multi-node scheduler: node roles, cordon/drain, zones, replicas.

ALTER TABLE nodes
    ADD COLUMN IF NOT EXISTS role TEXT NOT NULL DEFAULT 'worker'
        CHECK (role IN ('worker', 'edge')),
    ADD COLUMN IF NOT EXISTS cordoned BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS draining BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS zone TEXT NOT NULL DEFAULT 'default',
    ADD COLUMN IF NOT EXISTS labels JSONB NOT NULL DEFAULT '{}'::jsonb;

ALTER TABLE workloads
    ADD COLUMN IF NOT EXISTS replica_count INTEGER NOT NULL DEFAULT 1
        CHECK (replica_count >= 1 AND replica_count <= 32),
    ADD COLUMN IF NOT EXISTS schedule_strategy TEXT NOT NULL DEFAULT 'spread'
        CHECK (schedule_strategy IN ('spread', 'binpack')),
    ADD COLUMN IF NOT EXISTS require_kvm BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS preferred_zone TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS affinity_labels JSONB NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN IF NOT EXISTS anti_affinity_labels JSONB NOT NULL DEFAULT '{}'::jsonb;

-- Auto-require KVM for microvm runtime rows already present.
UPDATE workloads SET require_kvm = TRUE
WHERE lower(runtime) IN ('microvm', 'runtime_type_microvm', '3');

CREATE TABLE IF NOT EXISTS workload_replicas (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    workload_id UUID NOT NULL REFERENCES workloads(id) ON DELETE CASCADE,
    replica_index INTEGER NOT NULL CHECK (replica_index >= 0),
    assigned_node_id UUID REFERENCES nodes(id) ON DELETE SET NULL,
    host_port INTEGER,
    container_port INTEGER NOT NULL DEFAULT 80,
    status TEXT NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'building', 'deploying', 'running', 'stopped', 'failed')),
    desired_generation BIGINT NOT NULL DEFAULT 0,
    observed_generation BIGINT NOT NULL DEFAULT 0,
    desired_action TEXT NOT NULL DEFAULT 'none'
        CHECK (desired_action IN ('none', 'run', 'stop')),
    container_id TEXT NOT NULL DEFAULT '',
    last_error TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (workload_id, replica_index),
    UNIQUE (organization_id, id)
);

CREATE INDEX IF NOT EXISTS workload_replicas_node_action_idx
    ON workload_replicas (assigned_node_id, desired_action);
CREATE INDEX IF NOT EXISTS workload_replicas_workload_idx
    ON workload_replicas (workload_id);

-- Backfill one primary replica for existing workloads that already have placement intent.
INSERT INTO workload_replicas (
    organization_id, workload_id, replica_index, assigned_node_id, host_port, container_port,
    status, desired_generation, observed_generation, desired_action, container_id, last_error
)
SELECT
    organization_id, id, 0, assigned_node_id, host_port, container_port,
    status, desired_generation, observed_generation, desired_action, container_id, last_error
FROM workloads w
WHERE NOT EXISTS (
    SELECT 1 FROM workload_replicas wr WHERE wr.workload_id = w.id AND wr.replica_index = 0
)
ON CONFLICT (workload_id, replica_index) DO NOTHING;

ALTER TABLE workload_replicas ENABLE ROW LEVEL SECURITY;
ALTER TABLE workload_replicas FORCE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS workload_replicas_tenant_policy ON workload_replicas;
CREATE POLICY workload_replicas_tenant_policy ON workload_replicas USING (
    organization_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid
) WITH CHECK (
    organization_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid
);

GRANT SELECT, INSERT, UPDATE, DELETE ON workload_replicas TO norix_app;
