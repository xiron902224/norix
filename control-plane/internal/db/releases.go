package db

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// WorkloadRelease is an immutable snapshot used for rollback (#14).
type WorkloadRelease struct {
	ID             string
	OrganizationID string
	WorkloadID     string
	DeploymentID   string
	Generation     int64
	ImageOrSource  string
	ImageDigest    string
	Environment    map[string]string
	ContainerPort  int32
	CPUShares      int32
	MemoryLimitMB  int64
	Source         string
	CreatedAt      time.Time
}

// ReleaseRetention returns how many releases to keep (default 10).
func ReleaseRetention() int {
	v := strings.TrimSpace(os.Getenv("NORIX_RELEASE_RETENTION"))
	if v == "" {
		return 10
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 {
		return 10
	}
	if n > 100 {
		return 100
	}
	return n
}

// AutoRollbackEnabled is NORIX_AUTO_ROLLBACK=1 (#14).
func AutoRollbackEnabled() bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv("NORIX_AUTO_ROLLBACK")))
	return v == "1" || v == "true" || v == "yes" || v == "on"
}

// RecordWorkloadRelease stores a deploy snapshot and prunes old rows.
func RecordWorkloadRelease(ctx context.Context, pool *pgxpool.Pool, tenantID string, workload Workload, deploymentID, source string) (WorkloadRelease, error) {
	if source == "" {
		source = "deploy"
	}
	envJSON, err := encodeEnvironmentJSON(workload.Environment)
	if err != nil {
		return WorkloadRelease{}, err
	}
	digest := extractImageDigest(workload.ImageOrSource)

	tx, err := pool.Begin(ctx)
	if err != nil {
		return WorkloadRelease{}, err
	}
	defer tx.Rollback(ctx)
	if err := WithTenant(ctx, tx, tenantID); err != nil {
		return WorkloadRelease{}, err
	}

	var rel WorkloadRelease
	var envText string
	err = tx.QueryRow(ctx, `
		INSERT INTO workload_releases (
			organization_id, workload_id, deployment_id, generation,
			image_or_source, image_digest, env_vars, container_port,
			cpu_shares, memory_limit_mb, source
		) VALUES (
			$1::uuid, $2::uuid, NULLIF($3, '')::uuid, $4,
			$5, $6, $7::jsonb, $8,
			$9, $10, $11
		)
		RETURNING id::text, organization_id::text, workload_id::text, COALESCE(deployment_id::text, ''),
		          generation, image_or_source, image_digest, env_vars::text,
		          container_port, cpu_shares, memory_limit_mb, source, created_at`,
		tenantID, workload.ID, deploymentID, workload.DesiredGeneration,
		workload.ImageOrSource, digest, string(envJSON),
		workload.ContainerPort, workload.CPUShares, workload.MemoryLimitMB, source,
	).Scan(
		&rel.ID, &rel.OrganizationID, &rel.WorkloadID, &rel.DeploymentID,
		&rel.Generation, &rel.ImageOrSource, &rel.ImageDigest, &envText,
		&rel.ContainerPort, &rel.CPUShares, &rel.MemoryLimitMB, &rel.Source, &rel.CreatedAt,
	)
	if err != nil {
		return WorkloadRelease{}, fmt.Errorf("insert workload release: %w", err)
	}
	env, envErr := decodeEnvironmentJSON(envText)
	if envErr != nil {
		return WorkloadRelease{}, envErr
	}
	rel.Environment = env

	keep := ReleaseRetention()
	_, _ = tx.Exec(ctx, `
		DELETE FROM workload_releases
		WHERE workload_id = $1::uuid
		  AND id NOT IN (
			SELECT id FROM workload_releases
			WHERE workload_id = $1::uuid
			ORDER BY created_at DESC
			LIMIT $2
		  )`, workload.ID, keep)

	if err := writeAudit(ctx, tx, tenantID, "workload.release_recorded", "workload_release", rel.ID, AuditActor{}, map[string]string{
		"workload_id": workload.ID,
		"generation":  fmt.Sprintf("%d", rel.Generation),
		"source":      source,
	}); err != nil {
		return WorkloadRelease{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return WorkloadRelease{}, err
	}
	return rel, nil
}

func extractImageDigest(image string) string {
	image = strings.TrimSpace(image)
	if i := strings.Index(image, "@sha256:"); i >= 0 {
		return image[i+1:]
	}
	return ""
}

// ListWorkloadReleases returns newest-first history.
func ListWorkloadReleases(ctx context.Context, pool *pgxpool.Pool, tenantID, workloadID string, limit int) ([]WorkloadRelease, error) {
	if limit <= 0 {
		limit = ReleaseRetention()
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
		SELECT id::text, organization_id::text, workload_id::text, COALESCE(deployment_id::text, ''),
		       generation, image_or_source, image_digest, env_vars::text,
		       container_port, cpu_shares, memory_limit_mb, source, created_at
		FROM workload_releases
		WHERE workload_id = $1::uuid
		ORDER BY created_at DESC
		LIMIT $2`, workloadID, limit)
	if err != nil {
		return nil, fmt.Errorf("list releases: %w", err)
	}
	defer rows.Close()
	out := make([]WorkloadRelease, 0)
	for rows.Next() {
		var rel WorkloadRelease
		var envText string
		if err := rows.Scan(
			&rel.ID, &rel.OrganizationID, &rel.WorkloadID, &rel.DeploymentID,
			&rel.Generation, &rel.ImageOrSource, &rel.ImageDigest, &envText,
			&rel.ContainerPort, &rel.CPUShares, &rel.MemoryLimitMB, &rel.Source, &rel.CreatedAt,
		); err != nil {
			return nil, err
		}
		env, envErr := decodeEnvironmentJSON(envText)
		if envErr != nil {
			return nil, envErr
		}
		rel.Environment = MaskEnvironment(env)
		out = append(out, rel)
	}
	return out, tx.Commit(ctx)
}

// GetWorkloadRelease loads one release (env decrypted for restore).
func GetWorkloadRelease(ctx context.Context, pool *pgxpool.Pool, tenantID, releaseID string) (WorkloadRelease, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return WorkloadRelease{}, err
	}
	defer tx.Rollback(ctx)
	if err := WithTenant(ctx, tx, tenantID); err != nil {
		return WorkloadRelease{}, err
	}
	var rel WorkloadRelease
	var envText string
	err = tx.QueryRow(ctx, `
		SELECT id::text, organization_id::text, workload_id::text, COALESCE(deployment_id::text, ''),
		       generation, image_or_source, image_digest, env_vars::text,
		       container_port, cpu_shares, memory_limit_mb, source, created_at
		FROM workload_releases WHERE id = $1::uuid`, releaseID).Scan(
		&rel.ID, &rel.OrganizationID, &rel.WorkloadID, &rel.DeploymentID,
		&rel.Generation, &rel.ImageOrSource, &rel.ImageDigest, &envText,
		&rel.ContainerPort, &rel.CPUShares, &rel.MemoryLimitMB, &rel.Source, &rel.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return WorkloadRelease{}, fmt.Errorf("release not found")
		}
		return WorkloadRelease{}, fmt.Errorf("get release: %w", err)
	}
	env, envErr := decodeEnvironmentJSON(envText)
	if envErr != nil {
		return WorkloadRelease{}, envErr
	}
	rel.Environment = env
	return rel, tx.Commit(ctx)
}

// RollbackWorkloadToRelease restores image/env/limits from a release and redeploys.
// recordSource should be "rollback" or "auto-rollback".
func RollbackWorkloadToRelease(ctx context.Context, pool *pgxpool.Pool, tenantID, releaseID string, actor AuditActor, recordSource string) (Deployment, Workload, WorkloadRelease, error) {
	if recordSource == "" {
		recordSource = "rollback"
	}
	rel, err := GetWorkloadRelease(ctx, pool, tenantID, releaseID)
	if err != nil {
		return Deployment{}, Workload{}, WorkloadRelease{}, err
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return Deployment{}, Workload{}, WorkloadRelease{}, err
	}
	defer tx.Rollback(ctx)
	if err := WithTenant(ctx, tx, tenantID); err != nil {
		return Deployment{}, Workload{}, WorkloadRelease{}, err
	}
	envJSON, err := encodeEnvironmentJSON(rel.Environment)
	if err != nil {
		return Deployment{}, Workload{}, WorkloadRelease{}, err
	}
	tag, err := tx.Exec(ctx, `
		UPDATE workloads SET
			image_or_source = $2,
			env_vars = $3::jsonb,
			container_port = $4,
			cpu_shares = $5,
			memory_limit_mb = $6,
			desired_generation = desired_generation + 1,
			desired_action = 'run',
			status = 'deploying',
			last_error = '',
			updated_at = NOW()
		WHERE id = $1::uuid`,
		rel.WorkloadID, rel.ImageOrSource, string(envJSON),
		rel.ContainerPort, rel.CPUShares, rel.MemoryLimitMB,
	)
	if err != nil {
		return Deployment{}, Workload{}, WorkloadRelease{}, fmt.Errorf("apply release: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return Deployment{}, Workload{}, WorkloadRelease{}, fmt.Errorf("workload not found")
	}
	if err := writeAudit(ctx, tx, tenantID, "workload.rollback", "workload", rel.WorkloadID, actor, map[string]string{
		"release_id": rel.ID,
		"generation": fmt.Sprintf("%d", rel.Generation),
	}); err != nil {
		return Deployment{}, Workload{}, WorkloadRelease{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Deployment{}, Workload{}, WorkloadRelease{}, err
	}

	dep, wl, err := deployWorkloadReplicas(ctx, pool, tenantID, rel.WorkloadID, rel.ContainerPort, actor, recordSource)
	if err != nil {
		return Deployment{}, Workload{}, WorkloadRelease{}, err
	}
	releases, listErr := ListWorkloadReleases(ctx, pool, tenantID, rel.WorkloadID, 1)
	if listErr != nil || len(releases) == 0 {
		return dep, wl, WorkloadRelease{}, nil
	}
	return dep, wl, releases[0], nil
}

// PreviousReleaseForAutoRollback returns the release before the latest deploy snapshot.
// Skips when the latest snapshot is already an auto-rollback (avoid loops).
func PreviousReleaseForAutoRollback(ctx context.Context, pool *pgxpool.Pool, tenantID, workloadID string) (WorkloadRelease, error) {
	list, err := ListWorkloadReleases(ctx, pool, tenantID, workloadID, 2)
	if err != nil {
		return WorkloadRelease{}, err
	}
	if len(list) < 2 {
		return WorkloadRelease{}, fmt.Errorf("no previous release to auto-rollback")
	}
	if list[0].Source == "auto-rollback" || list[0].Source == "rollback" {
		return WorkloadRelease{}, fmt.Errorf("latest release already a rollback; skip auto-rollback")
	}
	// List masks env — reload plaintext for restore.
	return GetWorkloadRelease(ctx, pool, tenantID, list[1].ID)
}

