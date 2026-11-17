ALTER TABLE workloads
    DROP COLUMN IF EXISTS volume_backups,
    DROP COLUMN IF EXISTS volume_pending_op,
    DROP COLUMN IF EXISTS volume_mounts;
