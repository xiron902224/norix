DROP TABLE IF EXISTS alert_events;
DROP TABLE IF EXISTS backups;
ALTER TABLE nodes
    DROP COLUMN IF EXISTS disk_free_mb,
    DROP COLUMN IF EXISTS memory_free_mb,
    DROP COLUMN IF EXISTS cpu_usage_percent;
