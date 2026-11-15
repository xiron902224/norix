ALTER TABLE nodes
    ADD COLUMN IF NOT EXISTS is_local BOOLEAN NOT NULL DEFAULT FALSE;

CREATE UNIQUE INDEX IF NOT EXISTS nodes_one_local_per_org_idx
    ON nodes (organization_id)
    WHERE is_local;
