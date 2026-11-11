DROP TABLE IF EXISTS webhook_tokens;
DROP TABLE IF EXISTS build_secrets;
DROP TABLE IF EXISTS build_logs;
DROP TABLE IF EXISTS builds;

ALTER TABLE workloads
    DROP COLUMN IF EXISTS latest_build_id,
    DROP COLUMN IF EXISTS dockerfile_path,
    DROP COLUMN IF EXISTS git_ref,
    DROP COLUMN IF EXISTS git_url,
    DROP COLUMN IF EXISTS source_kind;
