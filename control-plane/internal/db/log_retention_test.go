package db

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func TestLoadLogRetentionDefaults(t *testing.T) {
	t.Setenv("NORIX_LOG_RETENTION_DAYS", "")
	t.Setenv("NORIX_WORKLOAD_LOG_MAX_LINES", "")
	t.Setenv("NORIX_BUILD_LOG_MAX_LINES", "")
	t.Setenv("NORIX_LOG_SEGMENT_MAX_LINES", "")
	t.Setenv("NORIX_LOG_RETENTION_BYTES", "")
	t.Setenv("NORIX_BUILD_LOG_RETENTION_BYTES", "")
	t.Setenv("NORIX_LOG_SEGMENT_RETENTION_BYTES", "")

	got := LoadLogRetention()
	if got.Days != 14 || got.WorkloadLines != 500 || got.BuildLines != 800 || got.SegmentLines != 5000 {
		t.Fatalf("defaults = %+v", got)
	}
	if got.WorkloadBytes != 32<<20 || got.BuildBytes != 32<<20 || got.SegmentBytes != 64<<20 {
		t.Fatalf("byte defaults = %+v", got)
	}
}

func TestLoadLogRetentionClamps(t *testing.T) {
	t.Setenv("NORIX_LOG_RETENTION_DAYS", "nope")
	t.Setenv("NORIX_WORKLOAD_LOG_MAX_LINES", "1")
	t.Setenv("NORIX_BUILD_LOG_MAX_LINES", "999999999")
	t.Setenv("NORIX_LOG_SEGMENT_MAX_LINES", "1000")
	t.Setenv("NORIX_LOG_RETENTION_BYTES", "0")
	t.Setenv("NORIX_BUILD_LOG_RETENTION_BYTES", "-5")
	t.Setenv("NORIX_LOG_SEGMENT_RETENTION_BYTES", "4096")

	got := LoadLogRetention()
	if got.Days != 14 {
		t.Fatalf("invalid days should fall back to 14, got %d", got.Days)
	}
	if got.WorkloadLines != 50 {
		t.Fatalf("line floor = %d", got.WorkloadLines)
	}
	if got.BuildLines != 100_000 {
		t.Fatalf("line ceiling = %d", got.BuildLines)
	}
	if got.SegmentLines != 1000 {
		t.Fatalf("segment lines = %d", got.SegmentLines)
	}
	if got.WorkloadBytes != 0 {
		t.Fatalf("zero bytes should disable the byte cap, got %d", got.WorkloadBytes)
	}
	if got.BuildBytes != 32<<20 {
		t.Fatalf("invalid bytes should fall back, got %d", got.BuildBytes)
	}
	if got.SegmentBytes != 4096 {
		t.Fatalf("segment bytes = %d", got.SegmentBytes)
	}
}

func TestPruneStoredLogsAgeLinesAndBytes(t *testing.T) {
	dsn := os.Getenv("NORIX_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("NORIX_TEST_DATABASE_URL required")
	}
	ctx := context.Background()
	if err := Migrate(dsn); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	pool, err := Open(ctx, dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(pool.Close)

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	t.Cleanup(func() { _ = tx.Rollback(context.Background()) })

	stamp := time.Now().UTC().Format("150405.000000")
	orgID, workloadID, buildID := seedLogRetentionFixture(t, ctx, tx, stamp)

	old := time.Now().UTC().Add(-30 * 24 * time.Hour)
	for i := 0; i < 3; i++ {
		if _, err := tx.Exec(ctx, `
			INSERT INTO workload_logs (organization_id, workload_id, line, created_at)
			VALUES ($1::uuid, $2::uuid, $3, $4)`,
			orgID, workloadID, fmt.Sprintf("old-%d", i), old); err != nil {
			t.Fatalf("insert old workload log: %v", err)
		}
	}
	for i := 0; i < 4; i++ {
		if _, err := tx.Exec(ctx, `
			INSERT INTO workload_logs (organization_id, workload_id, line)
			VALUES ($1::uuid, $2::uuid, $3)`,
			orgID, workloadID, fmt.Sprintf("new-%d-%s", i, strings.Repeat("x", 1000))); err != nil {
			t.Fatalf("insert new workload log: %v", err)
		}
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO build_logs (organization_id, build_id, line, created_at)
		VALUES ($1::uuid, $2::uuid, 'stale-build', $3)`,
		orgID, buildID, old); err != nil {
		t.Fatalf("insert old build log: %v", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO build_logs (organization_id, build_id, line)
		VALUES ($1::uuid, $2::uuid, 'fresh-build')`, orgID, buildID); err != nil {
		t.Fatalf("insert build log: %v", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO log_segments (organization_id, body, ts)
		VALUES ($1::uuid, 'old-segment', $2)`, orgID, old); err != nil {
		t.Fatalf("insert old segment: %v", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO log_segments (organization_id, body)
		VALUES ($1::uuid, 'fresh-segment')`, orgID); err != nil {
		t.Fatalf("insert segment: %v", err)
	}

	policy := LogRetention{
		Days:          14,
		WorkloadLines: 10,
		BuildLines:    10,
		SegmentLines:  10,
		WorkloadBytes: 2500,
		BuildBytes:    1 << 20,
		SegmentBytes:  1 << 20,
	}
	if _, err := PruneStoredLogs(ctx, tx, policy); err != nil {
		t.Fatalf("prune: %v", err)
	}

	var workloadLeft, buildLeft, segmentLeft int
	if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM workload_logs WHERE workload_id = $1::uuid`, workloadID).Scan(&workloadLeft); err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM build_logs WHERE build_id = $1::uuid`, buildID).Scan(&buildLeft); err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM log_segments WHERE organization_id = $1::uuid AND body LIKE '%segment'`, orgID).Scan(&segmentLeft); err != nil {
		t.Fatal(err)
	}
	// 3 aged out, 4 new of 1000 bytes, cap 2500 → two newest survive.
	if workloadLeft != 2 {
		t.Fatalf("workload logs left = %d, want 2", workloadLeft)
	}
	if buildLeft != 1 {
		t.Fatalf("build logs left = %d, want 1", buildLeft)
	}
	if segmentLeft != 1 {
		t.Fatalf("segments left = %d, want 1", segmentLeft)
	}
}

type logRetentionQuerier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func seedLogRetentionFixture(t *testing.T, ctx context.Context, q logRetentionQuerier, stamp string) (orgID, workloadID, buildID string) {
	t.Helper()
	if err := q.QueryRow(ctx, `
		INSERT INTO organizations (name, slug) VALUES ($1, $2) RETURNING id::text`,
		"log-retention-"+stamp, "log-retention-"+stamp).Scan(&orgID); err != nil {
		t.Fatalf("org: %v", err)
	}
	var projectID string
	if err := q.QueryRow(ctx, `
		INSERT INTO projects (organization_id, name) VALUES ($1::uuid, 'logs') RETURNING id::text`,
		orgID).Scan(&projectID); err != nil {
		t.Fatalf("project: %v", err)
	}
	if err := q.QueryRow(ctx, `
		INSERT INTO workloads (organization_id, project_id, name, runtime, image_or_source)
		VALUES ($1::uuid, $2::uuid, 'app', 'container', 'nginx:alpine')
		RETURNING id::text`, orgID, projectID).Scan(&workloadID); err != nil {
		t.Fatalf("workload: %v", err)
	}
	if err := q.QueryRow(ctx, `
		INSERT INTO builds (organization_id, workload_id, git_url)
		VALUES ($1::uuid, $2::uuid, 'https://example.com/app.git')
		RETURNING id::text`, orgID, workloadID).Scan(&buildID); err != nil {
		t.Fatalf("build: %v", err)
	}
	return orgID, workloadID, buildID
}
