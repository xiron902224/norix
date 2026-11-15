-- Agent inventory + docker-compose stacks (v1 Coolify-style multi-service)

ALTER TABLE nodes
    ADD COLUMN IF NOT EXISTS reported_workload_ids TEXT[] NOT NULL DEFAULT '{}';

ALTER TABLE workloads
    ADD COLUMN IF NOT EXISTS stack_id UUID,
    ADD COLUMN IF NOT EXISTS compose_service TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS compose_spec JSONB NOT NULL DEFAULT '{}'::jsonb;

CREATE INDEX IF NOT EXISTS workloads_stack_idx ON workloads (organization_id, stack_id)
    WHERE stack_id IS NOT NULL;

COMMENT ON COLUMN nodes.reported_workload_ids IS 'Workload IDs reported by agent heartbeat (norix.workload_id labels)';
COMMENT ON COLUMN workloads.stack_id IS 'Shared id for compose stack sibling services';
COMMENT ON COLUMN workloads.compose_service IS 'Original compose service name (alias on stack network)';
COMMENT ON COLUMN workloads.compose_spec IS 'Parsed compose service overrides / stack metadata JSON';
