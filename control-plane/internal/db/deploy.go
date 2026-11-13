package db

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/xiron902224/norix/control-plane/internal/scheduler"
)

type DesiredWorkload struct {
	ID                string
	ProjectID         string
	Name              string
	Image             string
	Environment       map[string]string
	CPUShares         int32
	MemoryLimitMB     int64
	HostPort          int32
	ContainerPort     int32
	DesiredGeneration int64
	Action            string
}

type ObservedWorkload struct {
	ID                  string
	Status              string
	ContainerID         string
	ObservedGeneration  int64
	Message             string
	HostPort            int32
	LogLines            []string
}

type Deployment struct {
	ID       string
	Status   string
	HostPort int32
	NodeID   string
}

func GetWorkload(ctx context.Context, pool *pgxpool.Pool, tenantID, workloadID string) (Workload, error) {
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
		       COALESCE(isolation_mode, 'container'), COALESCE(execution_runtime, 'docker')
		FROM workloads WHERE id = $1::uuid`, workloadID)
	if err != nil {
		return Workload{}, err
	}
	return workload, tx.Commit(ctx)
}

func scanWorkload(ctx context.Context, tx pgx.Tx, query string, args ...any) (Workload, error) {
	var workload Workload
	var environmentText string
	var assignedNode string
	var affinityText, antiAffinityText string
	err := tx.QueryRow(ctx, query, args...).Scan(
		&workload.ID, &workload.OrganizationID, &workload.ProjectID, &workload.Name, &workload.Runtime,
		&workload.ImageOrSource, &environmentText, &workload.CPUShares, &workload.MemoryLimitMB,
		&workload.Domain, &workload.Status, &workload.DesiredGeneration, &workload.ObservedGeneration,
		&workload.HostPort, &workload.ContainerPort, &assignedNode, &workload.ContainerID,
		&workload.LastError, &workload.DesiredAction,
		&workload.SourceKind, &workload.GitURL, &workload.GitRef, &workload.DockerfilePath, &workload.LatestBuildID,
		&workload.ReplicaCount, &workload.ScheduleStrategy, &workload.RequireKVM, &workload.PreferredZone,
		&affinityText, &antiAffinityText, &workload.IsolationMode, &workload.ExecutionRuntime,
	)
	if err != nil {
		return Workload{}, fmt.Errorf("get workload: %w", err)
	}
	_ = json.Unmarshal([]byte(environmentText), &workload.Environment)
	workload.AssignedNodeID = assignedNode
	workload.AffinityLabels = decodeLabels([]byte(affinityText))
	workload.AntiAffinityLabels = decodeLabels([]byte(antiAffinityText))
	if workload.ReplicaCount <= 0 {
		workload.ReplicaCount = 1
	}
	if workload.ScheduleStrategy == "" {
		workload.ScheduleStrategy = "spread"
	}
	if RequireKVMForIsolation(workload.IsolationMode) {
		workload.RequireKVM = true
	}
	return workload, nil
}

func PickOnlineNode(ctx context.Context, pool *pgxpool.Pool, tenantID string) (Node, error) {
	return PickScheduledNode(ctx, pool, tenantID, scheduler.Request{
		MemoryLimitMB: 512,
		Strategy:      "spread",
		PreferRemote:  true,
	})
}

// ReclaimDesiredWorkloads reschedules replicas stuck on offline/draining nodes onto healthy ones.
func ReclaimDesiredWorkloads(ctx context.Context, pool *pgxpool.Pool, organizationID, onlineNodeID string) (int64, error) {
	_ = onlineNodeID
	return RescheduleOrphanReplicas(ctx, pool, organizationID)
}

func AllocateHostPort(ctx context.Context, pool *pgxpool.Pool, tenantID string) (int32, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	if err := WithTenant(ctx, tx, tenantID); err != nil {
		return 0, err
	}
	port, err := allocateHostPortTx(ctx, tx)
	if err != nil {
		return 0, err
	}
	return port, tx.Commit(ctx)
}

func DeployWorkload(ctx context.Context, pool *pgxpool.Pool, tenantID, workloadID, nodeID string, hostPort, containerPort int32, actor AuditActor) (Deployment, Workload, error) {
	_ = nodeID
	_ = hostPort
	return DeployWorkloadReplicas(ctx, pool, tenantID, workloadID, containerPort, actor)
}

func ListDesiredWorkloadsForNode(ctx context.Context, pool *pgxpool.Pool, organizationID, nodeID string) ([]DesiredWorkload, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	if err := WithTenant(ctx, tx, organizationID); err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `
		SELECT wr.id::text, w.project_id::text,
		       CASE WHEN wr.replica_index = 0 THEN w.name ELSE w.name || '-r' || wr.replica_index::text END,
		       w.image_or_source, w.env_vars::text,
		       w.cpu_shares, w.memory_limit_mb, COALESCE(wr.host_port, 0), wr.container_port,
		       wr.desired_generation, wr.desired_action
		FROM workload_replicas wr
		JOIN workloads w ON w.id = wr.workload_id
		WHERE wr.assigned_node_id = $1::uuid
		  AND (
			-- Explicit mutate intents.
			wr.desired_action IN ('run', 'stop')
			-- Self-heal: keep ensuring containers that should already be up
			-- (covers host restarts / accidental docker rm while action=none).
			OR (wr.desired_action = 'none' AND wr.status IN ('running', 'deploying'))
		  )
		  AND (
			wr.desired_action = 'stop'
			OR (
				w.image_or_source <> ''
				AND w.image_or_source <> 'pending-build'
				AND w.status NOT IN ('building', 'pending')
			)
		  )
		ORDER BY wr.updated_at`, nodeID)
	if err != nil {
		return nil, fmt.Errorf("list desired workloads: %w", err)
	}
	defer rows.Close()
	result := make([]DesiredWorkload, 0)
	for rows.Next() {
		var item DesiredWorkload
		var envText string
		if err := rows.Scan(
			&item.ID, &item.ProjectID, &item.Name, &item.Image, &envText,
			&item.CPUShares, &item.MemoryLimitMB, &item.HostPort, &item.ContainerPort,
			&item.DesiredGeneration, &item.Action,
		); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(envText), &item.Environment)
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return result, nil
}

func ApplyObservedWorkloads(ctx context.Context, pool *pgxpool.Pool, organizationID string, observed []ObservedWorkload) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := WithTenant(ctx, tx, organizationID); err != nil {
		return err
	}
	for _, item := range observed {
		status := strings.TrimSpace(item.Status)
		if status == "" {
			status = "deploying"
		}
		var workloadID string
		err := tx.QueryRow(ctx, `
			UPDATE workload_replicas SET
				status = CASE WHEN status = 'deleting' AND $2 = 'stopped' THEN 'stopped' ELSE $2 END,
				observed_generation = $3,
				container_id = CASE
					WHEN $2 = 'stopped' THEN COALESCE($4, '')
					ELSE COALESCE(NULLIF($4, ''), container_id)
				END,
				host_port = CASE WHEN $5 > 0 THEN $5 ELSE host_port END,
				last_error = COALESCE($6, ''),
				updated_at = NOW(),
				desired_action = CASE
					WHEN $2 IN ('running', 'stopped', 'failed') AND desired_action = 'run' AND $3 >= desired_generation THEN 'none'
					WHEN $2 = 'stopped' AND desired_action = 'stop' THEN 'none'
					ELSE desired_action
				END
			WHERE id = $1::uuid
			RETURNING workload_id::text`,
			item.ID, status, item.ObservedGeneration, item.ContainerID, item.HostPort, item.Message,
		).Scan(&workloadID)
		if err == pgx.ErrNoRows {
			// Back-compat: observed id may still be the parent workload id.
			_, err = tx.Exec(ctx, `
				UPDATE workloads SET
					status = $2,
					observed_generation = $3,
					container_id = COALESCE(NULLIF($4, ''), container_id),
					host_port = CASE WHEN $5 > 0 THEN $5 ELSE host_port END,
					last_error = COALESCE($6, ''),
					updated_at = NOW(),
					desired_action = CASE
						WHEN $2 IN ('running', 'stopped', 'failed') AND desired_action = 'run' AND $3 >= desired_generation THEN 'none'
						WHEN $2 = 'stopped' AND desired_action = 'stop' THEN 'none'
						ELSE desired_action
					END
				WHERE id = $1::uuid`,
				item.ID, status, item.ObservedGeneration, item.ContainerID, item.HostPort, item.Message,
			)
			if err != nil {
				return fmt.Errorf("apply observed %s: %w", item.ID, err)
			}
			workloadID = item.ID
			_, _ = tx.Exec(ctx, `
				UPDATE workload_replicas SET
					status = $2,
					observed_generation = $3,
					container_id = COALESCE(NULLIF($4, ''), container_id),
					host_port = CASE WHEN $5 > 0 THEN $5 ELSE host_port END,
					last_error = COALESCE($6, ''),
					desired_action = CASE
						WHEN $2 IN ('running', 'stopped', 'failed') AND desired_action = 'run' AND $3 >= desired_generation THEN 'none'
						WHEN $2 = 'stopped' AND desired_action = 'stop' THEN 'none'
						ELSE desired_action
					END,
					updated_at = NOW()
				WHERE workload_id = $1::uuid AND replica_index = 0`,
				item.ID, status, item.ObservedGeneration, item.ContainerID, item.HostPort, item.Message,
			)
		} else if err != nil {
			return fmt.Errorf("apply observed replica %s: %w", item.ID, err)
		} else if err := syncParentFromPrimary(ctx, tx, workloadID); err != nil {
			return err
		}
		if status == "running" || status == "failed" || status == "stopped" {
			_, _ = tx.Exec(ctx, `
				UPDATE deployments SET status = $2, updated_at = NOW()
				WHERE id = (
					SELECT id FROM deployments WHERE workload_id = $1::uuid ORDER BY created_at DESC LIMIT 1
				)`, workloadID, status)
		}
		for _, line := range item.LogLines {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO workload_logs (organization_id, workload_id, line)
				VALUES ($1::uuid, $2::uuid, $3)`, organizationID, workloadID, truncate(line, 4000)); err != nil {
				return fmt.Errorf("insert log: %w", err)
			}
		}
		_, _ = tx.Exec(ctx, `
			DELETE FROM workload_logs
			WHERE workload_id = $1::uuid
			  AND id < COALESCE((
				SELECT id FROM workload_logs WHERE workload_id = $1::uuid ORDER BY id DESC OFFSET 500 LIMIT 1
			  ), 0)`, workloadID)
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	_, _ = PurgeDeletingWorkloads(ctx, pool, organizationID)
	return nil
}

func ListWorkloadLogs(ctx context.Context, pool *pgxpool.Pool, tenantID, workloadID string, tail int32) ([]string, error) {
	if tail <= 0 || tail > 500 {
		tail = 100
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
		SELECT line FROM (
			SELECT line, id FROM workload_logs
			WHERE workload_id = $1::uuid
			ORDER BY id DESC
			LIMIT $2
		) recent ORDER BY id`, workloadID, tail)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	lines := make([]string, 0)
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			return nil, err
		}
		lines = append(lines, line)
	}
	return lines, rows.Err()
}

func truncate(value string, max int) string {
	if len(value) <= max {
		return value
	}
	return value[:max]
}
