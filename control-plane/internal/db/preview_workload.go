package db

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PreviewWorkloadName is the stable workload name for a PR preview (#21).
func PreviewWorkloadName(prNumber int32) string {
	if prNumber < 1 {
		prNumber = 1
	}
	return fmt.Sprintf("pr-%d", prNumber)
}

// FindWorkloadByName returns a non-deleting workload in a project by exact name.
func FindWorkloadByName(ctx context.Context, pool *pgxpool.Pool, tenantID, projectID, name string) (Workload, error) {
	name = strings.TrimSpace(name)
	if projectID == "" || name == "" {
		return Workload{}, fmt.Errorf("project and name are required")
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return Workload{}, err
	}
	defer tx.Rollback(ctx)
	if err := WithTenant(ctx, tx, tenantID); err != nil {
		return Workload{}, err
	}
	var workload Workload
	var environmentText string
	var affinityText, antiText string
	var probeRaw, mountsRaw string
	err = tx.QueryRow(ctx, `
		SELECT id::text, organization_id::text, project_id::text, name, runtime, image_or_source, env_vars::text,
		       cpu_shares, memory_limit_mb, COALESCE(domain, ''), status,
		       desired_generation, observed_generation, COALESCE(host_port, 0), container_port,
		       COALESCE(assigned_node_id::text, ''), container_id, last_error, desired_action,
		       source_kind, git_url, git_ref, dockerfile_path, COALESCE(latest_build_id::text, ''),
		       replica_count, schedule_strategy, require_kvm, preferred_zone, affinity_labels::text, anti_affinity_labels::text,
		       COALESCE(isolation_mode, 'container'), COALESCE(execution_runtime, 'docker'),
		       COALESCE(stack_id::text, ''), COALESCE(compose_service, ''), COALESCE(compose_spec::text, '{}'),
		       COALESCE(NULLIF(update_strategy, ''), 'rolling'), COALESCE(max_surge, 1), COALESCE(max_unavailable, 0),
		       COALESCE(probe_spec::text, '{}'),
		       COALESCE(volume_mounts::text, '[]'), COALESCE(volume_pending_op::text, ''), COALESCE(volume_backups::text, '[]')
		FROM workloads
		WHERE project_id = $1::uuid AND name = $2 AND status <> 'deleting'
		LIMIT 1`, projectID, name,
	).Scan(
		&workload.ID, &workload.OrganizationID, &workload.ProjectID, &workload.Name,
		&workload.Runtime, &workload.ImageOrSource, &environmentText, &workload.CPUShares,
		&workload.MemoryLimitMB, &workload.Domain, &workload.Status,
		&workload.DesiredGeneration, &workload.ObservedGeneration, &workload.HostPort, &workload.ContainerPort,
		&workload.AssignedNodeID, &workload.ContainerID, &workload.LastError, &workload.DesiredAction,
		&workload.SourceKind, &workload.GitURL, &workload.GitRef, &workload.DockerfilePath, &workload.LatestBuildID,
		&workload.ReplicaCount, &workload.ScheduleStrategy, &workload.RequireKVM, &workload.PreferredZone,
		&affinityText, &antiText, &workload.IsolationMode, &workload.ExecutionRuntime,
		&workload.StackID, &workload.ComposeService, &workload.ComposeSpecJSON,
		&workload.UpdateStrategy, &workload.MaxSurge, &workload.MaxUnavailable, &probeRaw,
		&mountsRaw, &workload.VolumePendingOpJSON, &workload.VolumeBackupsJSON,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Workload{}, fmt.Errorf("workload not found")
	}
	if err != nil {
		return Workload{}, err
	}
	env, envErr := decodeEnvironmentJSON(environmentText)
	if envErr != nil {
		return Workload{}, envErr
	}
	workload.Environment = env
	workload.AffinityLabels = decodeLabels([]byte(affinityText))
	workload.AntiAffinityLabels = decodeLabels([]byte(antiText))
	workload.UpdateStrategy, workload.MaxSurge, workload.MaxUnavailable =
		NormalizeUpdateStrategy(workload.UpdateStrategy, workload.MaxSurge, workload.MaxUnavailable)
	if decoded, err := DecodeProbeSpec(probeRaw); err == nil {
		workload.ProbeSpec = decoded
	}
	if mounts, err := DecodeVolumeMounts(mountsRaw); err == nil {
		workload.VolumeMounts = mounts
	}
	workload.Ready = workload.Status == "running"
	return workload, tx.Commit(ctx)
}
