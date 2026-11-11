package db

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Workload struct {
	ID                 string
	OrganizationID     string
	ProjectID          string
	Name               string
	Runtime            string
	ImageOrSource      string
	Environment        map[string]string
	CPUShares          int32
	MemoryLimitMB      int64
	Domain             string
	Status             string
	DesiredGeneration  int64
	ObservedGeneration int64
	HostPort           int32
	ContainerPort      int32
	AssignedNodeID     string
	ContainerID        string
	LastError          string
	DesiredAction      string
}

func CreateWorkload(ctx context.Context, pool *pgxpool.Pool, tenantID string, workload Workload, actor AuditActor) (Workload, error) {
	workload.Name = strings.TrimSpace(workload.Name)
	if workload.ProjectID == "" || workload.Name == "" || workload.ImageOrSource == "" {
		return Workload{}, fmt.Errorf("project, name and image or source are required")
	}
	if workload.CPUShares <= 0 {
		workload.CPUShares = 1024
	}
	if workload.MemoryLimitMB <= 0 {
		workload.MemoryLimitMB = 512
	}
	if workload.ContainerPort <= 0 {
		workload.ContainerPort = 80
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return Workload{}, err
	}
	defer tx.Rollback(ctx)
	if err := WithTenant(ctx, tx, tenantID); err != nil {
		return Workload{}, err
	}
	if err := tx.QueryRow(ctx, `
		INSERT INTO workloads (organization_id, project_id, name, runtime, image_or_source, env_vars, cpu_shares, memory_limit_mb, domain, container_port)
		VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6, $7, $8, NULLIF($9, ''), $10)
		RETURNING id::text, organization_id::text, project_id::text, name, runtime, image_or_source, cpu_shares, memory_limit_mb, COALESCE(domain, ''), status,
		          desired_generation, observed_generation, COALESCE(host_port, 0), container_port,
		          COALESCE(assigned_node_id::text, ''), container_id, last_error, desired_action`,
		tenantID, workload.ProjectID, workload.Name, workload.Runtime, workload.ImageOrSource,
		mapToJSON(workload.Environment), workload.CPUShares, workload.MemoryLimitMB, workload.Domain, workload.ContainerPort,
	).Scan(
		&workload.ID, &workload.OrganizationID, &workload.ProjectID, &workload.Name, &workload.Runtime, &workload.ImageOrSource,
		&workload.CPUShares, &workload.MemoryLimitMB, &workload.Domain, &workload.Status,
		&workload.DesiredGeneration, &workload.ObservedGeneration, &workload.HostPort, &workload.ContainerPort,
		&workload.AssignedNodeID, &workload.ContainerID, &workload.LastError, &workload.DesiredAction,
	); err != nil {
		return Workload{}, fmt.Errorf("create workload: %w", err)
	}
	if err := writeAudit(ctx, tx, tenantID, "workload.created", "workload", workload.ID, actor, map[string]string{
		"name":    workload.Name,
		"project": workload.ProjectID,
	}); err != nil {
		return Workload{}, err
	}
	return workload, tx.Commit(ctx)
}

func mapToJSON(values map[string]string) []byte {
	if len(values) == 0 {
		return []byte(`{}`)
	}
	encoded, err := json.Marshal(values)
	if err != nil {
		return []byte(`{}`)
	}
	return encoded
}

func ListWorkloads(ctx context.Context, pool *pgxpool.Pool, tenantID, projectID string) ([]Workload, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	if err := WithTenant(ctx, tx, tenantID); err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `
		SELECT id::text, organization_id::text, project_id::text, name, runtime, image_or_source, env_vars::text,
		       cpu_shares, memory_limit_mb, COALESCE(domain, ''), status,
		       desired_generation, observed_generation, COALESCE(host_port, 0), container_port,
		       COALESCE(assigned_node_id::text, ''), container_id, last_error, desired_action
		FROM workloads WHERE project_id = $1::uuid ORDER BY created_at, name`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]Workload, 0)
	for rows.Next() {
		var workload Workload
		var environmentText string
		if err := rows.Scan(
			&workload.ID, &workload.OrganizationID, &workload.ProjectID, &workload.Name,
			&workload.Runtime, &workload.ImageOrSource, &environmentText, &workload.CPUShares,
			&workload.MemoryLimitMB, &workload.Domain, &workload.Status,
			&workload.DesiredGeneration, &workload.ObservedGeneration, &workload.HostPort, &workload.ContainerPort,
			&workload.AssignedNodeID, &workload.ContainerID, &workload.LastError, &workload.DesiredAction,
		); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(environmentText), &workload.Environment)
		result = append(result, workload)
	}
	return result, rows.Err()
}

func DeleteWorkload(ctx context.Context, pool *pgxpool.Pool, tenantID, workloadID string, actor AuditActor) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := WithTenant(ctx, tx, tenantID); err != nil {
		return err
	}
	// mark stop desired so agent can clean up if still assigned
	_, _ = tx.Exec(ctx, `
		UPDATE workloads SET desired_action = 'stop', desired_generation = desired_generation + 1, status = 'stopped', updated_at = NOW()
		WHERE id = $1::uuid AND container_id <> ''`, workloadID)
	command, err := tx.Exec(ctx, `DELETE FROM workloads WHERE id = $1::uuid`, workloadID)
	if err != nil {
		return err
	}
	if command.RowsAffected() == 0 {
		return fmt.Errorf("workload not found")
	}
	if err := writeAudit(ctx, tx, tenantID, "workload.deleted", "workload", workloadID, actor, nil); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func UpdateWorkload(ctx context.Context, pool *pgxpool.Pool, tenantID string, workload Workload, actor AuditActor) (Workload, error) {
	if workload.ID == "" || workload.Name == "" || workload.ImageOrSource == "" {
		return Workload{}, fmt.Errorf("workload id, name and image or source are required")
	}
	if workload.CPUShares <= 0 {
		workload.CPUShares = 1024
	}
	if workload.MemoryLimitMB <= 0 {
		workload.MemoryLimitMB = 512
	}
	if workload.ContainerPort <= 0 {
		workload.ContainerPort = 80
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return Workload{}, err
	}
	defer tx.Rollback(ctx)
	if err := WithTenant(ctx, tx, tenantID); err != nil {
		return Workload{}, err
	}
	err = tx.QueryRow(ctx, `
		UPDATE workloads SET name=$2, runtime=$3, image_or_source=$4, env_vars=$5,
		 cpu_shares=$6, memory_limit_mb=$7, domain=NULLIF($8, ''), container_port=$9, updated_at=NOW()
		WHERE id=$1::uuid AND organization_id=$10::uuid
		RETURNING id::text, organization_id::text, project_id::text, name, runtime, image_or_source,
		 cpu_shares, memory_limit_mb, COALESCE(domain, ''), status,
		 desired_generation, observed_generation, COALESCE(host_port, 0), container_port,
		 COALESCE(assigned_node_id::text, ''), container_id, last_error, desired_action`,
		workload.ID, workload.Name, workload.Runtime, workload.ImageOrSource, mapToJSON(workload.Environment),
		workload.CPUShares, workload.MemoryLimitMB, workload.Domain, workload.ContainerPort, tenantID).Scan(
		&workload.ID, &workload.OrganizationID, &workload.ProjectID, &workload.Name, &workload.Runtime,
		&workload.ImageOrSource, &workload.CPUShares, &workload.MemoryLimitMB, &workload.Domain, &workload.Status,
		&workload.DesiredGeneration, &workload.ObservedGeneration, &workload.HostPort, &workload.ContainerPort,
		&workload.AssignedNodeID, &workload.ContainerID, &workload.LastError, &workload.DesiredAction)
	if err != nil {
		return Workload{}, fmt.Errorf("update workload: %w", err)
	}
	if err := writeAudit(ctx, tx, tenantID, "workload.updated", "workload", workload.ID, actor, map[string]string{"name": workload.Name}); err != nil {
		return Workload{}, err
	}
	return workload, tx.Commit(ctx)
}
