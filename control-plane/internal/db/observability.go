package db

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type MetricSample struct {
	ID       int64
	NodeID   string
	Name     string
	Labels   map[string]string
	Value    float64
	TS       time.Time
}

type LogSegment struct {
	ID         int64
	ProjectID  string
	WorkloadID string
	Body       string
	Level      string
	TS         time.Time
}

type ObservabilityAlertRule struct {
	ID        string
	Name      string
	Kind      string
	Threshold float64
	Channel   string
	Enabled   bool
}

type ObservabilityAlertEvent struct {
	ID        string
	RuleID    string
	Message   string
	Status    string
	CreatedAt time.Time
}

func InsertMetricSample(ctx context.Context, pool *pgxpool.Pool, tenantID, nodeID, name string, labels map[string]string, value float64) error {
	if labels == nil {
		labels = map[string]string{}
	}
	raw, _ := json.Marshal(labels)
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := WithTenant(ctx, tx, tenantID); err != nil {
		return err
	}
	var nodeArg any = nil
	if strings.TrimSpace(nodeID) != "" {
		nodeArg = nodeID
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO metric_samples (organization_id, node_id, name, labels, value, ts)
		VALUES ($1::uuid, $2::uuid, $3, $4::jsonb, $5, NOW())`,
		tenantID, nodeArg, name, raw, value)
	if err != nil {
		return fmt.Errorf("insert metric sample: %w", err)
	}
	return tx.Commit(ctx)
}

func ListMetricSamples(ctx context.Context, pool *pgxpool.Pool, tenantID, nodeID, namePrefix string, limit int32) ([]MetricSample, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	if err := WithTenant(ctx, tx, tenantID); err != nil {
		return nil, err
	}
	query := `
		SELECT id, COALESCE(node_id::text, ''), name, labels::text, value, ts
		FROM metric_samples
		WHERE organization_id = $1::uuid`
	args := []any{tenantID}
	argN := 2
	if nodeID != "" {
		query += fmt.Sprintf(" AND node_id = $%d::uuid", argN)
		args = append(args, nodeID)
		argN++
	}
	if namePrefix != "" {
		query += fmt.Sprintf(" AND name LIKE $%d", argN)
		args = append(args, namePrefix+"%")
		argN++
	}
	query += fmt.Sprintf(" ORDER BY ts DESC LIMIT $%d", argN)
	args = append(args, limit)

	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]MetricSample, 0)
	for rows.Next() {
		var item MetricSample
		var labelsText string
		if err := rows.Scan(&item.ID, &item.NodeID, &item.Name, &labelsText, &item.Value, &item.TS); err != nil {
			return nil, err
		}
		item.Labels = decodeLabels([]byte(labelsText))
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, tx.Commit(ctx)
}

func InsertLogSegment(ctx context.Context, pool *pgxpool.Pool, tenantID, projectID, workloadID, body, level string) error {
	return insertLogSegments(ctx, pool, tenantID, projectID, workloadID, []string{body}, level)
}

func InsertLogSegmentsBatch(ctx context.Context, pool *pgxpool.Pool, tenantID, projectID, workloadID string, lines []string, level string) (int, error) {
	if len(lines) == 0 {
		return 0, nil
	}
	if err := insertLogSegments(ctx, pool, tenantID, projectID, workloadID, lines, level); err != nil {
		return 0, err
	}
	if workloadID != "" {
		_ = insertWorkloadLogLines(ctx, pool, tenantID, workloadID, lines)
	}
	return len(lines), nil
}

func insertLogSegments(ctx context.Context, pool *pgxpool.Pool, tenantID, projectID, workloadID string, lines []string, level string) error {
	level = strings.ToLower(strings.TrimSpace(level))
	if level == "" {
		level = "info"
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := WithTenant(ctx, tx, tenantID); err != nil {
		return err
	}
	var projectArg, workloadArg any
	if projectID != "" {
		projectArg = projectID
	}
	if workloadID != "" {
		workloadArg = workloadID
	}
	for _, body := range lines {
		body = strings.TrimSpace(body)
		if body == "" {
			continue
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO log_segments (organization_id, project_id, workload_id, body, level, ts)
			VALUES ($1::uuid, $2::uuid, $3::uuid, $4, $5, NOW())`,
			tenantID, projectArg, workloadArg, truncate(body, 8000), level)
		if err != nil {
			return fmt.Errorf("insert log segment: %w", err)
		}
	}
	return tx.Commit(ctx)
}

func insertWorkloadLogLines(ctx context.Context, pool *pgxpool.Pool, tenantID, workloadID string, lines []string) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := WithTenant(ctx, tx, tenantID); err != nil {
		return err
	}
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO workload_logs (organization_id, workload_id, line)
			VALUES ($1::uuid, $2::uuid, $3)`, tenantID, workloadID, truncate(line, 4000)); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func QueryLogSegments(ctx context.Context, pool *pgxpool.Pool, tenantID, projectID, workloadID, level string, limit int32) ([]LogSegment, error) {
	return QueryLogSegmentsFiltered(ctx, pool, tenantID, projectID, workloadID, level, "", limit)
}

// QueryLogSegmentsFiltered merges log_segments + workload/build logs and optionally
// filters by case-insensitive substring (S9 log search).
func QueryLogSegmentsFiltered(ctx context.Context, pool *pgxpool.Pool, tenantID, projectID, workloadID, level, contains string, limit int32) ([]LogSegment, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	fetch := limit
	if fetch < 500 {
		fetch = 500
	}
	segments, err := queryLogSegmentTable(ctx, pool, tenantID, projectID, workloadID, level, fetch)
	if err != nil {
		return nil, err
	}
	workloadLines, err := queryWorkloadLogSegments(ctx, pool, tenantID, projectID, workloadID, fetch)
	if err != nil {
		return nil, err
	}
	buildLines, err := queryBuildLogSegments(ctx, pool, tenantID, projectID, workloadID, fetch)
	if err != nil {
		return nil, err
	}
	merged := append(append(segments, workloadLines...), buildLines...)
	merged = FilterLogSegmentsContains(merged, contains)
	sort.Slice(merged, func(i, j int) bool {
		if merged[i].TS.Equal(merged[j].TS) {
			return merged[i].ID > merged[j].ID
		}
		return merged[i].TS.After(merged[j].TS)
	})
	if int32(len(merged)) > limit {
		merged = merged[:limit]
	}
	return merged, nil
}

// FilterLogSegmentsContains keeps segments whose body matches query (case-insensitive).
func FilterLogSegmentsContains(segments []LogSegment, contains string) []LogSegment {
	contains = strings.TrimSpace(contains)
	if contains == "" {
		return segments
	}
	needle := strings.ToLower(contains)
	out := make([]LogSegment, 0, len(segments))
	for _, segment := range segments {
		if strings.Contains(strings.ToLower(segment.Body), needle) {
			out = append(out, segment)
		}
	}
	return out
}

// RetainLogSegments keeps the newest N rows globally (ops retention).
func RetainLogSegments(ctx context.Context, pool *pgxpool.Pool, keep int) error {
	if keep < 100 {
		keep = 100
	}
	_, err := pool.Exec(ctx, `
		DELETE FROM log_segments
		WHERE id NOT IN (
			SELECT id FROM log_segments ORDER BY ts DESC, id DESC LIMIT $1
		)`, keep)
	return err
}

// RetainMetricSamples keeps the newest N metric samples.
func RetainMetricSamples(ctx context.Context, pool *pgxpool.Pool, keep int) error {
	if keep < 100 {
		keep = 100
	}
	_, err := pool.Exec(ctx, `
		DELETE FROM metric_samples
		WHERE id NOT IN (
			SELECT id FROM metric_samples ORDER BY ts DESC, id DESC LIMIT $1
		)`, keep)
	return err
}

func queryLogSegmentTable(ctx context.Context, pool *pgxpool.Pool, tenantID, projectID, workloadID, level string, limit int32) ([]LogSegment, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	if err := WithTenant(ctx, tx, tenantID); err != nil {
		return nil, err
	}
	query := `SELECT id, COALESCE(project_id::text, ''), COALESCE(workload_id::text, ''), body, level, ts
		FROM log_segments WHERE organization_id = $1::uuid`
	args := []any{tenantID}
	argN := 2
	if projectID != "" {
		query += fmt.Sprintf(" AND project_id = $%d::uuid", argN)
		args = append(args, projectID)
		argN++
	}
	if workloadID != "" {
		query += fmt.Sprintf(" AND workload_id = $%d::uuid", argN)
		args = append(args, workloadID)
		argN++
	}
	if level != "" {
		query += fmt.Sprintf(" AND level = $%d", argN)
		args = append(args, level)
		argN++
	}
	query += fmt.Sprintf(" ORDER BY ts DESC LIMIT $%d", argN)
	args = append(args, limit)
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]LogSegment, 0)
	for rows.Next() {
		var item LogSegment
		if err := rows.Scan(&item.ID, &item.ProjectID, &item.WorkloadID, &item.Body, &item.Level, &item.TS); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, tx.Commit(ctx)
}

func queryWorkloadLogSegments(ctx context.Context, pool *pgxpool.Pool, tenantID, projectID, workloadID string, limit int32) ([]LogSegment, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	if err := WithTenant(ctx, tx, tenantID); err != nil {
		return nil, err
	}
	query := `
		SELECT wl.id, COALESCE(w.project_id::text, ''), wl.workload_id::text, wl.line, 'info',
		       COALESCE(w.updated_at, NOW())
		FROM workload_logs wl
		JOIN workloads w ON w.id = wl.workload_id AND w.organization_id = wl.organization_id
		WHERE wl.organization_id = $1::uuid`
	args := []any{tenantID}
	argN := 2
	if projectID != "" {
		query += fmt.Sprintf(" AND w.project_id = $%d::uuid", argN)
		args = append(args, projectID)
		argN++
	}
	if workloadID != "" {
		query += fmt.Sprintf(" AND wl.workload_id = $%d::uuid", argN)
		args = append(args, workloadID)
		argN++
	}
	query += fmt.Sprintf(" ORDER BY wl.id DESC LIMIT $%d", argN)
	args = append(args, limit)
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]LogSegment, 0)
	for rows.Next() {
		var item LogSegment
		if err := rows.Scan(&item.ID, &item.ProjectID, &item.WorkloadID, &item.Body, &item.Level, &item.TS); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func queryBuildLogSegments(ctx context.Context, pool *pgxpool.Pool, tenantID, projectID, workloadID string, limit int32) ([]LogSegment, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	if err := WithTenant(ctx, tx, tenantID); err != nil {
		return nil, err
	}
	query := `
		SELECT bl.id, COALESCE(w.project_id::text, ''), w.id::text, bl.line, 'info', bl.created_at
		FROM build_logs bl
		JOIN builds b ON b.id = bl.build_id AND b.organization_id = bl.organization_id
		JOIN workloads w ON w.id = b.workload_id AND w.organization_id = b.organization_id
		WHERE bl.organization_id = $1::uuid`
	args := []any{tenantID}
	argN := 2
	if projectID != "" {
		query += fmt.Sprintf(" AND w.project_id = $%d::uuid", argN)
		args = append(args, projectID)
		argN++
	}
	if workloadID != "" {
		query += fmt.Sprintf(" AND w.id = $%d::uuid", argN)
		args = append(args, workloadID)
		argN++
	}
	query += fmt.Sprintf(" ORDER BY bl.id DESC LIMIT $%d", argN)
	args = append(args, limit)
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]LogSegment, 0)
	for rows.Next() {
		var item LogSegment
		if err := rows.Scan(&item.ID, &item.ProjectID, &item.WorkloadID, &item.Body, &item.Level, &item.TS); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func ListOpsAlertEventsAsObservability(ctx context.Context, pool *pgxpool.Pool, tenantID string, limit int32) ([]ObservabilityAlertEvent, error) {
	if limit <= 0 {
		limit = 50
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	if err := WithTenant(ctx, tx, tenantID); err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `
		SELECT id, kind, message, severity, created_at
		FROM ops_alert_events ORDER BY id DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]ObservabilityAlertEvent, 0)
	for rows.Next() {
		var id int64
		var item ObservabilityAlertEvent
		if err := rows.Scan(&id, &item.RuleID, &item.Message, &item.Status, &item.CreatedAt); err != nil {
			return nil, err
		}
		item.ID = strconv.FormatInt(id, 10)
		if item.Status == "" {
			item.Status = "open"
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, tx.Commit(ctx)
}

func ListAlertRules(ctx context.Context, pool *pgxpool.Pool, tenantID string) ([]ObservabilityAlertRule, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	if err := WithTenant(ctx, tx, tenantID); err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `
		SELECT id::text, name, kind, threshold, channel, enabled
		FROM alert_rules ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]ObservabilityAlertRule, 0)
	for rows.Next() {
		var item ObservabilityAlertRule
		if err := rows.Scan(&item.ID, &item.Name, &item.Kind, &item.Threshold, &item.Channel, &item.Enabled); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, tx.Commit(ctx)
}

func ListObservabilityAlertEvents(ctx context.Context, pool *pgxpool.Pool, tenantID string, limit int32) ([]ObservabilityAlertEvent, error) {
	if limit <= 0 {
		limit = 50
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	if err := WithTenant(ctx, tx, tenantID); err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `
		SELECT id::text, rule_id::text, message, status, created_at
		FROM alert_events ORDER BY created_at DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]ObservabilityAlertEvent, 0)
	for rows.Next() {
		var item ObservabilityAlertEvent
		if err := rows.Scan(&item.ID, &item.RuleID, &item.Message, &item.Status, &item.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, tx.Commit(ctx)
}

func UpsertAlertRule(ctx context.Context, pool *pgxpool.Pool, tenantID string, rule ObservabilityAlertRule) (ObservabilityAlertRule, error) {
	rule.Name = strings.TrimSpace(rule.Name)
	if rule.Name == "" {
		return ObservabilityAlertRule{}, fmt.Errorf("alert rule name is required")
	}
	if rule.Kind == "" {
		rule.Kind = "threshold"
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return ObservabilityAlertRule{}, err
	}
	defer tx.Rollback(ctx)
	if err := WithTenant(ctx, tx, tenantID); err != nil {
		return ObservabilityAlertRule{}, err
	}
	if rule.ID == "" {
		err = tx.QueryRow(ctx, `
			INSERT INTO alert_rules (organization_id, name, kind, threshold, channel, enabled)
			VALUES ($1::uuid, $2, $3, $4, $5, $6)
			RETURNING id::text, name, kind, threshold, channel, enabled`,
			tenantID, rule.Name, rule.Kind, rule.Threshold, rule.Channel, rule.Enabled,
		).Scan(&rule.ID, &rule.Name, &rule.Kind, &rule.Threshold, &rule.Channel, &rule.Enabled)
	} else {
		err = tx.QueryRow(ctx, `
			INSERT INTO alert_rules (id, organization_id, name, kind, threshold, channel, enabled)
			VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6, $7)
			ON CONFLICT (organization_id, name) DO UPDATE SET
				kind = EXCLUDED.kind,
				threshold = EXCLUDED.threshold,
				channel = EXCLUDED.channel,
				enabled = EXCLUDED.enabled,
				updated_at = NOW()
			RETURNING id::text, name, kind, threshold, channel, enabled`,
			rule.ID, tenantID, rule.Name, rule.Kind, rule.Threshold, rule.Channel, rule.Enabled,
		).Scan(&rule.ID, &rule.Name, &rule.Kind, &rule.Threshold, &rule.Channel, &rule.Enabled)
	}
	if err != nil {
		return ObservabilityAlertRule{}, fmt.Errorf("upsert alert rule: %w", err)
	}
	return rule, tx.Commit(ctx)
}

// SampleNodeHostMetrics records coarse host metrics for the local node.
func SampleNodeHostMetrics(ctx context.Context, pool *pgxpool.Pool, tenantID, nodeID string) error {
	cpu, memFree, diskFree := SampleHostMetrics()
	_ = InsertMetricSample(ctx, pool, tenantID, nodeID, "node_cpu_usage_percent", nil, cpu)
	_ = InsertMetricSample(ctx, pool, tenantID, nodeID, "node_memory_free_mb", nil, float64(memFree))
	_ = InsertMetricSample(ctx, pool, tenantID, nodeID, "node_disk_free_mb", nil, float64(diskFree))
	return nil
}
