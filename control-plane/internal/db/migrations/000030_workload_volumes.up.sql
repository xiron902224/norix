-- #16: persistent named volumes for workloads
ALTER TABLE workloads
    ADD COLUMN IF NOT EXISTS volume_mounts JSONB NOT NULL DEFAULT '[]'::jsonb,
    ADD COLUMN IF NOT EXISTS volume_pending_op JSONB,
    ADD COLUMN IF NOT EXISTS volume_backups JSONB NOT NULL DEFAULT '[]'::jsonb;

COMMENT ON COLUMN workloads.volume_mounts IS
    'JSON array: [{name,mount_path,read_only}] — Docker named volumes, node-local';
COMMENT ON COLUMN workloads.volume_pending_op IS
    'Optional one-shot agent op: {id,op,name} where op is backup|delete';
COMMENT ON COLUMN workloads.volume_backups IS
    'JSON array of completed backups: [{id,name,host_path,created_at,status,error}]';
