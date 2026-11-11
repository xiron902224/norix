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
	SourceKind         string
	GitURL             string
	GitRef             string
	DockerfilePath     string
	LatestBuildID      string
	ReplicaCount       int32
	ScheduleStrategy   string
	RequireKVM         bool
	PreferredZone      string
	AffinityLabels     map[string]string
	AntiAffinityLabels map[string]string
}

func CreateWorkload(ctx context.Context, pool *pgxpool.Pool, tenantID string, workload Workload, actor AuditActor) (Workload, error) {
	workload.Name = strings.TrimSpace(workload.Name)
	workload.GitURL = strings.TrimSpace(workload.GitURL)
	if workload.SourceKind == "" {
		if workload.GitURL != "" {
			workload.SourceKind = "git"
		} else {
			workload.SourceKind = "image"
		}
	}
	if workload.ProjectID == "" || workload.Name == "" {
		return Workload{}, fmt.Errorf("project and name are required")
	}
	if workload.SourceKind == "git" {
		if workload.GitURL == "" {
			return Workload{}, fmt.Errorf("git url is required for git workloads")
		}
		if workload.ImageOrSource == "" {
			workload.ImageOrSource = "pending-build"
		}
		if workload.GitRef == "" {
			workload.GitRef = "main"
		}
		if workload.DockerfilePath == "" {
			workload.DockerfilePath = "Dockerfile"
		}
	} else if workload.ImageOrSource == "" {
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
	if workload.ReplicaCount <= 0 {
		workload.ReplicaCount = 1
	}
	if workload.ScheduleStrategy == "" {
		workload.ScheduleStrategy = "spread"
	}
	if RequireKVMForRuntime(workload.Runtime) {
		workload.RequireKVM = true
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return Workload{}, err
	}
	defer tx.Rollback(ctx)
	if err := WithTenant(ctx, tx, tenantID); err != nil {
		return Workload{}, err
	}
	var affinityOut, antiOut string
	if err := tx.QueryRow(ctx, `
		INSERT INTO workloads (organization_id, project_id, name, runtime, image_or_source, env_vars, cpu_shares, memory_limit_mb, domain, container_port,
		                       source_kind, git_url, git_ref, dockerfile_path,
		                       replica_count, schedule_strategy, require_kvm, preferred_zone, affinity_labels, anti_affinity_labels)
		VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6, $7, $8, NULLIF($9, ''), $10, $11, $12, $13, $14,
		        $15, $16, $17, $18, $19::jsonb, $20::jsonb)
		RETURNING id::text, organization_id::text, project_id::text, name, runtime, image_or_source, cpu_shares, memory_limit_mb, COALESCE(domain, ''), status,
		          desired_generation, observed_generation, COALESCE(host_port, 0), container_port,
		          COALESCE(assigned_node_id::text, ''), container_id, last_error, desired_action,
		          source_kind, git_url, git_ref, dockerfile_path, COALESCE(latest_build_id::text, ''),
		          replica_count, schedule_strategy, require_kvm, preferred_zone, affinity_labels::text, anti_affinity_labels::text`,
		tenantID, workload.ProjectID, workload.Name, workload.Runtime, workload.ImageOrSource,
		mapToJSON(workload.Environment), workload.CPUShares, workload.MemoryLimitMB, workload.Domain, workload.ContainerPort,
		workload.SourceKind, workload.GitURL, workload.GitRef, workload.DockerfilePath,
		workload.ReplicaCount, workload.ScheduleStrategy, workload.RequireKVM, workload.PreferredZone,
		encodeLabels(workload.AffinityLabels), encodeLabels(workload.AntiAffinityLabels),
	).Scan(
		&workload.ID, &workload.OrganizationID, &workload.ProjectID, &workload.Name, &workload.Runtime, &workload.ImageOrSource,
		&workload.CPUShares, &workload.MemoryLimitMB, &workload.Domain, &workload.Status,
		&workload.DesiredGeneration, &workload.ObservedGeneration, &workload.HostPort, &workload.ContainerPort,
		&workload.AssignedNodeID, &workload.ContainerID, &workload.LastError, &workload.DesiredAction,
		&workload.SourceKind, &workload.GitURL, &workload.GitRef, &workload.DockerfilePath, &workload.LatestBuildID,
		&workload.ReplicaCount, &workload.ScheduleStrategy, &workload.RequireKVM, &workload.PreferredZone,
		&affinityOut, &antiOut,
	); err != nil {
		return Workload{}, fmt.Errorf("create workload: %w", err)
	}
	workload.AffinityLabels = decodeLabels([]byte(affinityOut))
	workload.AntiAffinityLabels = decodeLabels([]byte(antiOut))
	if err := EnsureWorkloadReplicas(ctx, tx, tenantID, workload); err != nil {
		return Workload{}, err
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
		       COALESCE(assigned_node_id::text, ''), container_id, last_error, desired_action,
		       source_kind, git_url, git_ref, dockerfile_path, COALESCE(latest_build_id::text, ''),
		       replica_count, schedule_strategy, require_kvm, preferred_zone, affinity_labels::text, anti_affinity_labels::text
		FROM workloads WHERE project_id = $1::uuid AND status <> 'deleting' ORDER BY created_at, name`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]Workload, 0)
	for rows.Next() {
		var workload Workload
		var environmentText string
		var affinityText, antiText string
		if err := rows.Scan(
			&workload.ID, &workload.OrganizationID, &workload.ProjectID, &workload.Name,
			&workload.Runtime, &workload.ImageOrSource, &environmentText, &workload.CPUShares,
			&workload.MemoryLimitMB, &workload.Domain, &workload.Status,
			&workload.DesiredGeneration, &workload.ObservedGeneration, &workload.HostPort, &workload.ContainerPort,
			&workload.AssignedNodeID, &workload.ContainerID, &workload.LastError, &workload.DesiredAction,
			&workload.SourceKind, &workload.GitURL, &workload.GitRef, &workload.DockerfilePath, &workload.LatestBuildID,
			&workload.ReplicaCount, &workload.ScheduleStrategy, &workload.RequireKVM, &workload.PreferredZone,
			&affinityText, &antiText,
		); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(environmentText), &workload.Environment)
		workload.AffinityLabels = decodeLabels([]byte(affinityText))
		workload.AntiAffinityLabels = decodeLabels([]byte(antiText))
		result = append(result, workload)
	}
	return result, rows.Err()
}

// DeleteWorkload soft-deletes: asks agents to stop containers, then PurgeDeletingWorkloads
// removes the row once replicas report stopped (or never had a container).
func DeleteWorkload(ctx context.Context, pool *pgxpool.Pool, tenantID, workloadID string, actor AuditActor) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := WithTenant(ctx, tx, tenantID); err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `
		UPDATE workloads SET
			status = 'deleting',
			desired_action = 'stop',
			desired_generation = desired_generation + 1,
			updated_at = NOW()
		WHERE id = $1::uuid`, workloadID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("workload not found")
	}
	_, err = tx.Exec(ctx, `
		UPDATE workload_replicas SET
			desired_action = 'stop',
			desired_generation = desired_generation + 1,
			status = CASE
				WHEN container_id = '' AND status IN ('pending', 'stopped', 'failed') THEN 'stopped'
				ELSE 'deleting'
			END,
			updated_at = NOW()
		WHERE workload_id = $1::uuid`, workloadID)
	if err != nil {
		return err
	}
	if err := writeAudit(ctx, tx, tenantID, "workload.deleting", "workload", workloadID, actor, nil); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	_, _ = PurgeDeletingWorkloads(ctx, pool, tenantID)
	return nil
}

// PurgeDeletingWorkloads hard-deletes workloads whose containers have been stopped.
func PurgeDeletingWorkloads(ctx context.Context, pool *pgxpool.Pool, tenantID string) (int64, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	if err := WithTenant(ctx, tx, tenantID); err != nil {
		return 0, err
	}
	rows, err := tx.Query(ctx, `
		SELECT w.id::text
		FROM workloads w
		WHERE w.status = 'deleting'
		  AND NOT EXISTS (
			SELECT 1 FROM workload_replicas wr
			WHERE wr.workload_id = w.id
			  AND wr.desired_action = 'stop'
		  )
		  AND NOT EXISTS (
			SELECT 1 FROM workload_replicas wr
			WHERE wr.workload_id = w.id
			  AND wr.status NOT IN ('stopped', 'failed', 'pending')
		  )`)
	if err != nil {
		return 0, err
	}
	ids := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return 0, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	var purged int64
	for _, id := range ids {
		tag, err := tx.Exec(ctx, `DELETE FROM workloads WHERE id = $1::uuid AND status = 'deleting'`, id)
		if err != nil {
			return purged, err
		}
		purged += tag.RowsAffected()
	}
	if err := tx.Commit(ctx); err != nil {
		return purged, err
	}
	return purged, nil
}

func PurgeAllDeletingWorkloads(ctx context.Context, pool *pgxpool.Pool) (int64, error) {
	orgs, err := ListOrganizationIDs(ctx, pool)
	if err != nil {
		return 0, err
	}
	var total int64
	for _, orgID := range orgs {
		n, err := PurgeDeletingWorkloads(ctx, pool, orgID)
		if err != nil {
			continue
		}
		total += n
	}
	return total, nil
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
	if workload.ReplicaCount <= 0 {
		workload.ReplicaCount = 1
	}
	if workload.ReplicaCount > 32 {
		workload.ReplicaCount = 32
	}
	if workload.ScheduleStrategy == "" {
		workload.ScheduleStrategy = "spread"
	}
	if RequireKVMForRuntime(workload.Runtime) {
		workload.RequireKVM = true
	}
	if strings.TrimSpace(workload.PreferredZone) == "" {
		workload.PreferredZone = "default"
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return Workload{}, err
	}
	defer tx.Rollback(ctx)
	if err := WithTenant(ctx, tx, tenantID); err != nil {
		return Workload{}, err
	}
	var affinityOut, antiOut string
	err = tx.QueryRow(ctx, `
		UPDATE workloads SET name=$2, runtime=$3, image_or_source=$4, env_vars=$5,
		 cpu_shares=$6, memory_limit_mb=$7, domain=NULLIF($8, ''), container_port=$9,
		 replica_count=$10, schedule_strategy=$11, require_kvm=$12, preferred_zone=$13,
		 affinity_labels=$14::jsonb, anti_affinity_labels=$15::jsonb, updated_at=NOW()
		WHERE id=$1::uuid AND organization_id=$16::uuid
		RETURNING id::text, organization_id::text, project_id::text, name, runtime, image_or_source,
		 cpu_shares, memory_limit_mb, COALESCE(domain, ''), status,
		 desired_generation, observed_generation, COALESCE(host_port, 0), container_port,
		 COALESCE(assigned_node_id::text, ''), container_id, last_error, desired_action,
		 source_kind, git_url, git_ref, dockerfile_path, COALESCE(latest_build_id::text, ''),
		 replica_count, schedule_strategy, require_kvm, preferred_zone, affinity_labels::text, anti_affinity_labels::text`,
		workload.ID, workload.Name, workload.Runtime, workload.ImageOrSource, mapToJSON(workload.Environment),
		workload.CPUShares, workload.MemoryLimitMB, workload.Domain, workload.ContainerPort,
		workload.ReplicaCount, workload.ScheduleStrategy, workload.RequireKVM, workload.PreferredZone,
		encodeLabels(workload.AffinityLabels), encodeLabels(workload.AntiAffinityLabels), tenantID).Scan(
		&workload.ID, &workload.OrganizationID, &workload.ProjectID, &workload.Name, &workload.Runtime,
		&workload.ImageOrSource, &workload.CPUShares, &workload.MemoryLimitMB, &workload.Domain, &workload.Status,
		&workload.DesiredGeneration, &workload.ObservedGeneration, &workload.HostPort, &workload.ContainerPort,
		&workload.AssignedNodeID, &workload.ContainerID, &workload.LastError, &workload.DesiredAction,
		&workload.SourceKind, &workload.GitURL, &workload.GitRef, &workload.DockerfilePath, &workload.LatestBuildID,
		&workload.ReplicaCount, &workload.ScheduleStrategy, &workload.RequireKVM, &workload.PreferredZone,
		&affinityOut, &antiOut)
	if err != nil {
		return Workload{}, fmt.Errorf("update workload: %w", err)
	}
	workload.AffinityLabels = decodeLabels([]byte(affinityOut))
	workload.AntiAffinityLabels = decodeLabels([]byte(antiOut))
	if err := EnsureWorkloadReplicas(ctx, tx, tenantID, workload); err != nil {
		return Workload{}, err
	}
	if err := writeAudit(ctx, tx, tenantID, "workload.updated", "workload", workload.ID, actor, map[string]string{"name": workload.Name}); err != nil {
		return Workload{}, err
	}
	return workload, tx.Commit(ctx)
}
