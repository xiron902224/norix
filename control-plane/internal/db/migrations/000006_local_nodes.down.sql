DROP INDEX IF EXISTS nodes_one_local_per_org_idx;
ALTER TABLE nodes DROP COLUMN IF EXISTS is_local;
