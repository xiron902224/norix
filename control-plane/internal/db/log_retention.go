package db

import (
	"context"
	"os"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
)

const (
	defaultLogRetentionDays      = 14
	defaultWorkloadLogMaxLines   = 500
	defaultBuildLogMaxLines      = 800
	defaultLogSegmentMaxLines    = 5000
	defaultLogRetentionBytes     = 32 << 20
	defaultLogSegmentRetainBytes = 64 << 20
)

// LogRetention is the operator policy for stored logs (#25).
// Days or bytes set to 0 disable that dimension. Line caps stay on so a
// stream cannot grow without bound.
type LogRetention struct {
	Days          int
	WorkloadLines int
	BuildLines    int
	SegmentLines  int
	WorkloadBytes int64
	BuildBytes    int64
	SegmentBytes  int64
}

type sqlExec interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// LogPruneCounts is how many rows a prune pass deleted.
type LogPruneCounts struct {
	Workload int64
	Build    int64
	Segments int64
}

// Total is the sum of deleted rows.
func (c LogPruneCounts) Total() int64 {
	return c.Workload + c.Build + c.Segments
}

// LoadLogRetention reads NORIX_LOG_* . Empty or invalid values fall back to defaults.
func LoadLogRetention() LogRetention {
	return LogRetention{
		Days:          envInt("NORIX_LOG_RETENTION_DAYS", defaultLogRetentionDays, 0, 3650),
		WorkloadLines: envInt("NORIX_WORKLOAD_LOG_MAX_LINES", defaultWorkloadLogMaxLines, 50, 100_000),
		BuildLines:    envInt("NORIX_BUILD_LOG_MAX_LINES", defaultBuildLogMaxLines, 50, 100_000),
		SegmentLines:  envInt("NORIX_LOG_SEGMENT_MAX_LINES", defaultLogSegmentMaxLines, 100, 500_000),
		WorkloadBytes: envInt64("NORIX_LOG_RETENTION_BYTES", defaultLogRetentionBytes, 0, 10<<30),
		BuildBytes:    envInt64("NORIX_BUILD_LOG_RETENTION_BYTES", defaultLogRetentionBytes, 0, 10<<30),
		SegmentBytes:  envInt64("NORIX_LOG_SEGMENT_RETENTION_BYTES", defaultLogSegmentRetainBytes, 0, 10<<30),
	}
}

// WorkloadLogMaxLines is the per-workload line cap used on the ingest path.
func WorkloadLogMaxLines() int {
	return LoadLogRetention().WorkloadLines
}

// BuildLogMaxLines is the per-build line cap used on the ingest path.
func BuildLogMaxLines() int {
	return LoadLogRetention().BuildLines
}

// PruneStoredLogs deletes logs that are too old, too numerous, or too large.
// The caller must use a role that can see every tenant (migration/owner pool).
func PruneStoredLogs(ctx context.Context, exec sqlExec, policy LogRetention) (LogPruneCounts, error) {
	var counts LogPruneCounts
	var err error
	if counts.Workload, err = pruneTable(ctx, exec, policy.Days, policy.WorkloadLines, policy.WorkloadBytes, workloadLogPrune); err != nil {
		return counts, err
	}
	if counts.Build, err = pruneTable(ctx, exec, policy.Days, policy.BuildLines, policy.BuildBytes, buildLogPrune); err != nil {
		return counts, err
	}
	if counts.Segments, err = pruneTable(ctx, exec, policy.Days, policy.SegmentLines, policy.SegmentBytes, segmentLogPrune); err != nil {
		return counts, err
	}
	return counts, nil
}

type pruneSQL struct {
	ageSQL   string
	linesSQL string
	bytesSQL string
}

var (
	workloadLogPrune = pruneSQL{
		ageSQL: `
			DELETE FROM workload_logs
			WHERE created_at < NOW() - make_interval(days => $1::int)`,
		linesSQL: `
			DELETE FROM workload_logs wl
			USING (
				SELECT workload_id, id FROM (
					SELECT workload_id, id,
					       ROW_NUMBER() OVER (PARTITION BY workload_id ORDER BY id DESC) AS rn
					FROM workload_logs
				) ranked WHERE rn > $1
			) old
			WHERE wl.id = old.id`,
		bytesSQL: `
			DELETE FROM workload_logs wl
			USING (
				SELECT id FROM (
					SELECT id,
					       SUM(octet_length(line)) OVER (PARTITION BY workload_id ORDER BY id DESC) AS running
					FROM workload_logs
				) sized
				WHERE running > $1
			) old
			WHERE wl.id = old.id`,
	}
	buildLogPrune = pruneSQL{
		ageSQL: `
			DELETE FROM build_logs
			WHERE created_at < NOW() - make_interval(days => $1::int)`,
		linesSQL: `
			DELETE FROM build_logs bl
			USING (
				SELECT build_id, id FROM (
					SELECT build_id, id,
					       ROW_NUMBER() OVER (PARTITION BY build_id ORDER BY id DESC) AS rn
					FROM build_logs
				) ranked WHERE rn > $1
			) old
			WHERE bl.id = old.id`,
		bytesSQL: `
			DELETE FROM build_logs bl
			USING (
				SELECT id FROM (
					SELECT id,
					       SUM(octet_length(line)) OVER (PARTITION BY build_id ORDER BY id DESC) AS running
					FROM build_logs
				) sized
				WHERE running > $1
			) old
			WHERE bl.id = old.id`,
	}
	segmentLogPrune = pruneSQL{
		ageSQL: `
			DELETE FROM log_segments
			WHERE ts < NOW() - make_interval(days => $1::int)`,
		linesSQL: `
			DELETE FROM log_segments ls
			USING (
				SELECT organization_id, id FROM (
					SELECT organization_id, id,
					       ROW_NUMBER() OVER (PARTITION BY organization_id ORDER BY ts DESC, id DESC) AS rn
					FROM log_segments
				) ranked WHERE rn > $1
			) old
			WHERE ls.id = old.id`,
		bytesSQL: `
			DELETE FROM log_segments ls
			USING (
				SELECT id FROM (
					SELECT id,
					       SUM(octet_length(body)) OVER (PARTITION BY organization_id ORDER BY ts DESC, id DESC) AS running
					FROM log_segments
				) sized
				WHERE running > $1
			) old
			WHERE ls.id = old.id`,
	}
)

func pruneTable(ctx context.Context, exec sqlExec, days, lines int, maxBytes int64, query pruneSQL) (int64, error) {
	var removed int64
	if days > 0 {
		n, err := execAffected(ctx, exec, query.ageSQL, days)
		if err != nil {
			return removed, err
		}
		removed += n
	}
	if lines > 0 {
		n, err := execAffected(ctx, exec, query.linesSQL, lines)
		if err != nil {
			return removed, err
		}
		removed += n
	}
	if maxBytes > 0 {
		n, err := execAffected(ctx, exec, query.bytesSQL, maxBytes)
		if err != nil {
			return removed, err
		}
		removed += n
	}
	return removed, nil
}

func execAffected(ctx context.Context, exec sqlExec, sql string, arg any) (int64, error) {
	tag, err := exec.Exec(ctx, sql, arg)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func envInt(key string, fallback, min, max int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	if n < min {
		return min
	}
	if n > max {
		return max
	}
	return n
}

func envInt64(key string, fallback, min, max int64) int64 {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || n < min || n > max {
		return fallback
	}
	return n
}
