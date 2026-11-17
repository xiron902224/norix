-- #17: configurable liveness / readiness probes
ALTER TABLE workloads
    ADD COLUMN IF NOT EXISTS probe_spec JSONB NOT NULL DEFAULT '{}'::jsonb;

ALTER TABLE workload_replicas
    ADD COLUMN IF NOT EXISTS ready BOOLEAN NOT NULL DEFAULT TRUE,
    ADD COLUMN IF NOT EXISTS probe_status JSONB NOT NULL DEFAULT '{}'::jsonb;

COMMENT ON COLUMN workloads.probe_spec IS 'JSON: {readiness,liveness} probe configs (http|tcp|exec)';
COMMENT ON COLUMN workload_replicas.ready IS 'False when readiness probe is failing; edge should skip';
