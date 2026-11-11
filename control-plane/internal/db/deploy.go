package db

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
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
		       COALESCE(assigned_node_id::text, ''), container_id, last_error, desired_action
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
	err := tx.QueryRow(ctx, query, args...).Scan(
		&workload.ID, &workload.OrganizationID, &workload.ProjectID, &workload.Name, &workload.Runtime,
		&workload.ImageOrSource, &environmentText, &workload.CPUShares, &workload.MemoryLimitMB,
		&workload.Domain, &workload.Status, &workload.DesiredGeneration, &workload.ObservedGeneration,
		&workload.HostPort, &workload.ContainerPort, &assignedNode, &workload.ContainerID,
		&workload.LastError, &workload.DesiredAction,
	)
	if err != nil {
		return Workload{}, fmt.Errorf("get workload: %w", err)
	}
	_ = json.Unmarshal([]byte(environmentText), &workload.Environment)
	workload.AssignedNodeID = assignedNode
	return workload, nil
}

func PickOnlineNode(ctx context.Context, pool *pgxpool.Pool, tenantID string) (Node, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return Node{}, err
	}
	defer tx.Rollback(ctx)
	if err := WithTenant(ctx, tx, tenantID); err != nil {
		return Node{}, err
	}
	var node Node
	err = tx.QueryRow(ctx, `
		SELECT id::text, organization_id::text, name, status, last_seen_at, is_local
		FROM nodes
		WHERE status = 'online'
		ORDER BY is_local ASC, last_seen_at DESC NULLS LAST
		LIMIT 1`).Scan(&node.ID, &node.OrganizationID, &node.Name, &node.Status, &node.LastSeenAt, &node.IsLocal)
	if err == nil {
		return node, tx.Commit(ctx)
	}
	if err != pgx.ErrNoRows {
		return Node{}, fmt.Errorf("pick node: %w", err)
	}
	if err := tx.Rollback(ctx); err != nil {
		return Node{}, err
	}

	// Prefer a remote agent when online; otherwise use the embedded control-plane node.
	local, ensureErr := EnsureLocalNode(ctx, pool, tenantID, DetectHostCapabilities())
	if ensureErr != nil {
		return Node{}, fmt.Errorf("no online agent and could not use control-plane host: %w", ensureErr)
	}
	if !local.Capabilities.DockerSocket && !local.Capabilities.ContainerdSocket {
		return Node{}, fmt.Errorf("no online agent; control-plane host has no docker/containerd socket")
	}
	return local, nil
}

// ReclaimDesiredWorkloads moves stuck run/stop intents from offline/unassigned nodes onto an online node.
func ReclaimDesiredWorkloads(ctx context.Context, pool *pgxpool.Pool, organizationID, onlineNodeID string) (int64, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	if err := WithTenant(ctx, tx, organizationID); err != nil {
		return 0, err
	}
	tag, err := tx.Exec(ctx, `
		UPDATE workloads
		SET assigned_node_id = $1::uuid, updated_at = NOW()
		WHERE desired_action IN ('run', 'stop')
		  AND (
			assigned_node_id IS NULL
			OR assigned_node_id IN (
				SELECT id FROM nodes
				WHERE organization_id = $2::uuid
				  AND status <> 'online'
			)
		)`, onlineNodeID, organizationID)
	if err != nil {
		return 0, fmt.Errorf("reclaim desired workloads: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
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
	var port int32
	err = tx.QueryRow(ctx, `
		WITH bounds AS (SELECT 18080 AS start_port)
		SELECT COALESCE(
			(SELECT MIN(candidate)
			 FROM generate_series(18080, 18200) candidate
			 WHERE candidate NOT IN (SELECT host_port FROM workloads WHERE host_port IS NOT NULL)
			), 18080)`).Scan(&port)
	if err != nil {
		return 0, fmt.Errorf("allocate host port: %w", err)
	}
	return port, tx.Commit(ctx)
}

func DeployWorkload(ctx context.Context, pool *pgxpool.Pool, tenantID, workloadID, nodeID string, hostPort, containerPort int32, actor AuditActor) (Deployment, Workload, error) {
	if containerPort <= 0 {
		containerPort = 80
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return Deployment{}, Workload{}, err
	}
	defer tx.Rollback(ctx)
	if err := WithTenant(ctx, tx, tenantID); err != nil {
		return Deployment{}, Workload{}, err
	}

	var workload Workload
	var environmentText string
	err = tx.QueryRow(ctx, `
		UPDATE workloads SET
			desired_generation = desired_generation + 1,
			desired_action = 'run',
			status = 'deploying',
			assigned_node_id = $2::uuid,
			host_port = $3,
			container_port = $4,
			last_error = '',
			updated_at = NOW()
		WHERE id = $1::uuid AND organization_id = $5::uuid
		RETURNING id::text, organization_id::text, project_id::text, name, runtime, image_or_source, env_vars::text,
		          cpu_shares, memory_limit_mb, COALESCE(domain, ''), status,
		          desired_generation, observed_generation, COALESCE(host_port, 0), container_port,
		          COALESCE(assigned_node_id::text, ''), container_id, last_error, desired_action`,
		workloadID, nodeID, hostPort, containerPort, tenantID,
	).Scan(
		&workload.ID, &workload.OrganizationID, &workload.ProjectID, &workload.Name, &workload.Runtime,
		&workload.ImageOrSource, &environmentText, &workload.CPUShares, &workload.MemoryLimitMB,
		&workload.Domain, &workload.Status, &workload.DesiredGeneration, &workload.ObservedGeneration,
		&workload.HostPort, &workload.ContainerPort, &workload.AssignedNodeID, &workload.ContainerID,
		&workload.LastError, &workload.DesiredAction,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return Deployment{}, Workload{}, fmt.Errorf("workload not found")
		}
		return Deployment{}, Workload{}, fmt.Errorf("deploy workload: %w", err)
	}
	_ = json.Unmarshal([]byte(environmentText), &workload.Environment)

	var deployment Deployment
	err = tx.QueryRow(ctx, `
		INSERT INTO deployments (organization_id, workload_id, node_id, status, host_port)
		VALUES ($1::uuid, $2::uuid, $3::uuid, 'deploying', $4)
		RETURNING id::text, status, COALESCE(host_port, 0), COALESCE(node_id::text, '')`,
		tenantID, workloadID, nodeID, hostPort,
	).Scan(&deployment.ID, &deployment.Status, &deployment.HostPort, &deployment.NodeID)
	if err != nil {
		return Deployment{}, Workload{}, fmt.Errorf("create deployment: %w", err)
	}
	if err := writeAudit(ctx, tx, tenantID, "workload.deployed", "workload", workloadID, actor, map[string]string{
		"deployment_id": deployment.ID,
		"node_id":       nodeID,
	}); err != nil {
		return Deployment{}, Workload{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Deployment{}, Workload{}, err
	}
	return deployment, workload, nil
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
		SELECT id::text, project_id::text, name, image_or_source, env_vars::text,
		       cpu_shares, memory_limit_mb, COALESCE(host_port, 0), container_port,
		       desired_generation, desired_action
		FROM workloads
		WHERE assigned_node_id = $1::uuid
		  AND desired_action IN ('run', 'stop')
		ORDER BY updated_at`, nodeID)
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
		_, err := tx.Exec(ctx, `
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
		if status == "running" || status == "failed" || status == "stopped" {
			_, _ = tx.Exec(ctx, `
				UPDATE deployments SET status = $2, updated_at = NOW()
				WHERE id = (
					SELECT id FROM deployments WHERE workload_id = $1::uuid ORDER BY created_at DESC LIMIT 1
				)`, item.ID, status)
		}
		for _, line := range item.LogLines {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO workload_logs (organization_id, workload_id, line)
				VALUES ($1::uuid, $2::uuid, $3)`, organizationID, item.ID, truncate(line, 4000)); err != nil {
				return fmt.Errorf("insert log: %w", err)
			}
		}
		// keep last 500 lines
		_, _ = tx.Exec(ctx, `
			DELETE FROM workload_logs
			WHERE workload_id = $1::uuid
			  AND id < COALESCE((
				SELECT id FROM workload_logs WHERE workload_id = $1::uuid ORDER BY id DESC OFFSET 500 LIMIT 1
			  ), 0)`, item.ID)
	}
	return tx.Commit(ctx)
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
