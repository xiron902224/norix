-- #15: rolling / zero-downtime update knobs
ALTER TABLE workloads
    ADD COLUMN IF NOT EXISTS update_strategy TEXT NOT NULL DEFAULT 'rolling'
        CHECK (update_strategy IN ('rolling', 'recreate')),
    ADD COLUMN IF NOT EXISTS max_surge INTEGER NOT NULL DEFAULT 1
        CHECK (max_surge >= 0 AND max_surge <= 32),
    ADD COLUMN IF NOT EXISTS max_unavailable INTEGER NOT NULL DEFAULT 0
        CHECK (max_unavailable >= 0 AND max_unavailable <= 32);

-- At least one of max_surge / max_unavailable must allow progress (enforced in app).
ALTER TABLE workload_replicas
    ADD COLUMN IF NOT EXISTS surge_host_port INTEGER;

COMMENT ON COLUMN workloads.update_strategy IS 'rolling = start new before stop old; recreate = stop then start';
COMMENT ON COLUMN workload_replicas.surge_host_port IS 'Temporary publish port while rolling to a new generation';
