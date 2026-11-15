package db

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ScalableWorkload is a running/deploying workload considered for HPA (S18).
type ScalableWorkload struct {
	Workload
	NodeCPUUsagePercent float64
	HasNodeCPU          bool
}

// ListScalableWorkloads returns non-deleting workloads that are running or deploying.
func ListScalableWorkloads(ctx context.Context, pool *pgxpool.Pool, tenantID string) ([]ScalableWorkload, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	if err := WithTenant(ctx, tx, tenantID); err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `
		SELECT w.id::text, w.organization_id::text, w.project_id::text, w.name, w.runtime, w.image_or_source, w.env_vars::text,
		       w.cpu_shares, w.memory_limit_mb, COALESCE(w.domain, ''), w.status,
		       w.desired_generation, w.observed_generation, COALESCE(w.host_port, 0), w.container_port,
		       COALESCE(w.assigned_node_id::text, ''), w.container_id, w.last_error, w.desired_action,
		       w.source_kind, w.git_url, w.git_ref, w.dockerfile_path, COALESCE(w.latest_build_id::text, ''),
		       w.replica_count, w.schedule_strategy, w.require_kvm, w.preferred_zone,
		       w.affinity_labels::text, w.anti_affinity_labels::text,
		       COALESCE(w.isolation_mode, 'container'), COALESCE(w.execution_runtime, 'docker'),
		       COALESCE(w.stack_id::text, ''), COALESCE(w.compose_service, ''), COALESCE(w.compose_spec::text, '{}'),
		       COALESCE(n.cpu_usage_percent, 0), (n.id IS NOT NULL)
		FROM workloads w
		LEFT JOIN nodes n ON n.id = w.assigned_node_id
		WHERE w.organization_id = $1::uuid
		  AND w.status IN ('running', 'deploying')
		  AND w.replica_count >= 1
		ORDER BY w.created_at, w.name`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list scalable workloads: %w", err)
	}
	defer rows.Close()
	out := make([]ScalableWorkload, 0)
	for rows.Next() {
		var item ScalableWorkload
		var environmentText, affinityText, antiText string
		var hasNode bool
		if err := rows.Scan(
			&item.ID, &item.OrganizationID, &item.ProjectID, &item.Name,
			&item.Runtime, &item.ImageOrSource, &environmentText, &item.CPUShares,
			&item.MemoryLimitMB, &item.Domain, &item.Status,
			&item.DesiredGeneration, &item.ObservedGeneration, &item.HostPort, &item.ContainerPort,
			&item.AssignedNodeID, &item.ContainerID, &item.LastError, &item.DesiredAction,
			&item.SourceKind, &item.GitURL, &item.GitRef, &item.DockerfilePath, &item.LatestBuildID,
			&item.ReplicaCount, &item.ScheduleStrategy, &item.RequireKVM, &item.PreferredZone,
			&affinityText, &antiText, &item.IsolationMode, &item.ExecutionRuntime,
			&item.StackID, &item.ComposeService, &item.ComposeSpecJSON,
			&item.NodeCPUUsagePercent, &hasNode,
		); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(environmentText), &item.Environment)
		item.AffinityLabels = decodeLabels([]byte(affinityText))
		item.AntiAffinityLabels = decodeLabels([]byte(antiText))
		item.HasNodeCPU = hasNode
		if item.ReplicaCount <= 0 {
			item.ReplicaCount = 1
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, tx.Commit(ctx)
}

// LatestMetricValue returns the newest metric_samples value for name (exact) optionally filtered by node/label.
func LatestMetricValue(ctx context.Context, pool *pgxpool.Pool, tenantID, nodeID, name, labelKey, labelValue string) (float64, bool, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return 0, false, err
	}
	defer tx.Rollback(ctx)
	if err := WithTenant(ctx, tx, tenantID); err != nil {
		return 0, false, err
	}
	query := `
		SELECT value FROM metric_samples
		WHERE organization_id = $1::uuid AND name = $2`
	args := []any{tenantID, name}
	argN := 3
	if strings.TrimSpace(nodeID) != "" {
		query += fmt.Sprintf(" AND node_id = $%d::uuid", argN)
		args = append(args, nodeID)
		argN++
	}
	if labelKey != "" {
		query += fmt.Sprintf(" AND labels->>$%d = $%d", argN, argN+1)
		args = append(args, labelKey, labelValue)
		argN += 2
	}
	query += " ORDER BY ts DESC LIMIT 1"
	var value float64
	err = tx.QueryRow(ctx, query, args...).Scan(&value)
	if err == pgx.ErrNoRows {
		return 0, false, tx.Commit(ctx)
	}
	if err != nil {
		return 0, false, err
	}
	return value, true, tx.Commit(ctx)
}

// SetWorkloadReplicaCount updates replica_count and ensures replica rows exist.
// Unassigned replicas are picked up by RescheduleOrphanReplicas (leader cleanup).
func SetWorkloadReplicaCount(ctx context.Context, pool *pgxpool.Pool, tenantID, workloadID string, replicas int32, actor AuditActor) (Workload, error) {
	if replicas < 1 {
		replicas = 1
	}
	if replicas > 32 {
		replicas = 32
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return Workload{}, err
	}
	defer tx.Rollback(ctx)
	if err := WithTenant(ctx, tx, tenantID); err != nil {
		return Workload{}, err
	}
	workload, err := scanWorkload(ctx, tx, `
		SELECT id::text, organization_id::text, project_id::text, name, runtime, image_or_source, env_vars::text,
		       cpu_shares, memory_limit_mb, COALESCE(domain, ''), status,
		       desired_generation, observed_generation, COALESCE(host_port, 0), container_port,
		       COALESCE(assigned_node_id::text, ''), container_id, last_error, desired_action,
		       source_kind, git_url, git_ref, dockerfile_path, COALESCE(latest_build_id::text, ''),
		       replica_count, schedule_strategy, require_kvm, preferred_zone, affinity_labels::text, anti_affinity_labels::text,
		       COALESCE(isolation_mode, 'container'), COALESCE(execution_runtime, 'docker'),
		       COALESCE(stack_id::text, ''), COALESCE(compose_service, ''), COALESCE(compose_spec::text, '{}')
		FROM workloads WHERE id = $1::uuid FOR UPDATE`, workloadID)
	if err != nil {
		return Workload{}, err
	}
	if workload.ReplicaCount == replicas {
		return workload, tx.Commit(ctx)
	}
	_, err = tx.Exec(ctx, `
		UPDATE workloads SET replica_count = $2, updated_at = NOW()
		WHERE id = $1::uuid`, workloadID, replicas)
	if err != nil {
		return Workload{}, fmt.Errorf("set replica_count: %w", err)
	}
	workload.ReplicaCount = replicas
	if err := EnsureWorkloadReplicas(ctx, tx, tenantID, workload); err != nil {
		return Workload{}, err
	}
	if err := writeAudit(ctx, tx, tenantID, "workload.autoscale", "workload", workloadID, actor, map[string]string{
		"replica_count": fmt.Sprintf("%d", replicas),
	}); err != nil {
		return Workload{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Workload{}, err
	}
	return workload, nil
}
