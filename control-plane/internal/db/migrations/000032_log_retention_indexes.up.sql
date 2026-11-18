-- #25: age prune scans created_at / ts on every leader tick.
CREATE INDEX IF NOT EXISTS workload_logs_created_idx ON workload_logs (created_at);
CREATE INDEX IF NOT EXISTS build_logs_created_idx ON build_logs (created_at);
CREATE INDEX IF NOT EXISTS log_segments_ts_idx ON log_segments (ts);
