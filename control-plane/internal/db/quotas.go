package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

type TeamQuota struct {
	TeamID       string
	MaxCPU       int32
	MaxMemoryMB  int64
	MaxDiskGB    int64
	MaxSandboxes int32
	MaxPreviews  int32
}

type TeamUsageSnapshot struct {
	TeamID        string
	CPUUsed       int32
	MemoryMBUsed  int64
	DiskGBUsed    int64
	SandboxesUsed int32
	PreviewsUsed  int32
}

type QuotaDelta struct {
	AdditionalCPU       int32
	AdditionalMemoryMB  int64
	AdditionalDiskGB    int64
	AdditionalSandboxes int32
	AdditionalPreviews  int32
}

func GetTeamQuota(ctx context.Context, pool *pgxpool.Pool, organizationID, teamID string) (TeamQuota, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return TeamQuota{}, fmt.Errorf("begin get quota: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := WithTenant(ctx, tx, organizationID); err != nil {
		return TeamQuota{}, err
	}
	var quota TeamQuota
	quota.TeamID = teamID
	err = tx.QueryRow(ctx, `
		SELECT max_cpu, max_memory_mb, max_disk_gb, max_sandboxes, max_previews
		FROM team_quotas WHERE organization_id = $1::uuid AND team_id = $2::uuid`,
		organizationID, teamID).Scan(
		&quota.MaxCPU, &quota.MaxMemoryMB, &quota.MaxDiskGB, &quota.MaxSandboxes, &quota.MaxPreviews,
	)
	if err != nil {
		return TeamQuota{}, fmt.Errorf("get team quota: %w", err)
	}
	return quota, nil
}

func SetTeamQuota(ctx context.Context, pool *pgxpool.Pool, organizationID string, quota TeamQuota) (TeamQuota, error) {
	if quota.TeamID == "" {
		return TeamQuota{}, fmt.Errorf("team_id is required")
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return TeamQuota{}, fmt.Errorf("begin set quota: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := WithTenant(ctx, tx, organizationID); err != nil {
		return TeamQuota{}, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO team_quotas (organization_id, team_id, max_cpu, max_memory_mb, max_disk_gb, max_sandboxes, max_previews)
		VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6, $7)
		ON CONFLICT (team_id) DO UPDATE SET
			max_cpu = EXCLUDED.max_cpu,
			max_memory_mb = EXCLUDED.max_memory_mb,
			max_disk_gb = EXCLUDED.max_disk_gb,
			max_sandboxes = EXCLUDED.max_sandboxes,
			max_previews = EXCLUDED.max_previews,
			updated_at = NOW()`,
		organizationID, quota.TeamID, quota.MaxCPU, quota.MaxMemoryMB, quota.MaxDiskGB, quota.MaxSandboxes, quota.MaxPreviews,
	); err != nil {
		return TeamQuota{}, fmt.Errorf("set team quota: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return TeamQuota{}, fmt.Errorf("commit quota: %w", err)
	}
	return quota, nil
}

func ComputeTeamUsage(ctx context.Context, pool *pgxpool.Pool, organizationID, teamID string) (TeamUsageSnapshot, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return TeamUsageSnapshot{}, fmt.Errorf("begin team usage: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := WithTenant(ctx, tx, organizationID); err != nil {
		return TeamUsageSnapshot{}, err
	}
	usage := TeamUsageSnapshot{TeamID: teamID}

	// Workloads on projects assigned to this team (cpu_shares / 1024 approximates vCPU).
	if err := tx.QueryRow(ctx, `
		SELECT COALESCE(SUM(GREATEST(w.cpu_shares / 1024, 1)), 0)::int,
		       COALESCE(SUM(w.memory_limit_mb), 0)::bigint
		FROM workloads w
		JOIN projects p ON p.id = w.project_id AND p.organization_id = w.organization_id
		WHERE w.organization_id = $1::uuid AND p.team_id = $2::uuid
		  AND w.status NOT IN ('failed')`,
		organizationID, teamID).Scan(&usage.CPUUsed, &usage.MemoryMBUsed); err != nil {
		return TeamUsageSnapshot{}, fmt.Errorf("workload usage: %w", err)
	}

	var sandboxCPU int32
	var sandboxMem int64
	var sandboxCount int32
	if err := tx.QueryRow(ctx, `
		SELECT COALESCE(SUM(s.cpu), 0)::int,
		       COALESCE(SUM(s.memory_mb), 0)::bigint,
		       COUNT(*)::int
		FROM sandboxes s
		JOIN projects p ON p.id = s.project_id AND p.organization_id = s.organization_id
		WHERE s.organization_id = $1::uuid AND p.team_id = $2::uuid
		  AND s.status NOT IN ('destroyed', 'expired')`,
		organizationID, teamID).Scan(&sandboxCPU, &sandboxMem, &sandboxCount); err != nil {
		return TeamUsageSnapshot{}, fmt.Errorf("sandbox usage: %w", err)
	}
	usage.CPUUsed += sandboxCPU
	usage.MemoryMBUsed += sandboxMem
	usage.SandboxesUsed = sandboxCount

	if err := tx.QueryRow(ctx, `
		SELECT COUNT(*)::int FROM pr_previews pr
		JOIN projects p ON p.id = pr.project_id AND p.organization_id = pr.organization_id
		WHERE pr.organization_id = $1::uuid AND p.team_id = $2::uuid
		  AND pr.status <> 'destroyed'`,
		organizationID, teamID).Scan(&usage.PreviewsUsed); err != nil {
		return TeamUsageSnapshot{}, fmt.Errorf("preview usage: %w", err)
	}

	if err := tx.QueryRow(ctx, `
		SELECT COALESCE(SUM(quantity), 0)::bigint
		FROM usage_records
		WHERE organization_id = $1::uuid AND team_id = $2::uuid AND metric = 'disk_gb'`,
		organizationID, teamID).Scan(&usage.DiskGBUsed); err != nil {
		return TeamUsageSnapshot{}, fmt.Errorf("disk usage: %w", err)
	}

	var dbDisk int64
	if err := tx.QueryRow(ctx, `
		SELECT COALESCE(COUNT(*) * 10, 0)::bigint
		FROM managed_databases md
		JOIN projects p ON p.id = md.project_id AND p.organization_id = md.organization_id
		WHERE md.organization_id = $1::uuid AND p.team_id = $2::uuid
		  AND md.status NOT IN ('failed')`,
		organizationID, teamID).Scan(&dbDisk); err != nil {
		return TeamUsageSnapshot{}, fmt.Errorf("database disk usage: %w", err)
	}
	usage.DiskGBUsed += dbDisk

	return usage, nil
}

func CheckTeamQuota(ctx context.Context, pool *pgxpool.Pool, organizationID, teamID string, delta QuotaDelta) error {
	quota, err := GetTeamQuota(ctx, pool, organizationID, teamID)
	if err != nil {
		return err
	}
	usage, err := ComputeTeamUsage(ctx, pool, organizationID, teamID)
	if err != nil {
		return err
	}

	projectedCPU := usage.CPUUsed + delta.AdditionalCPU
	projectedMem := usage.MemoryMBUsed + delta.AdditionalMemoryMB
	projectedDisk := usage.DiskGBUsed + delta.AdditionalDiskGB
	projectedSandboxes := usage.SandboxesUsed + delta.AdditionalSandboxes
	projectedPreviews := usage.PreviewsUsed + delta.AdditionalPreviews

	if quota.MaxCPU > 0 && projectedCPU > quota.MaxCPU {
		return fmt.Errorf("team %s exceeded CPU quota: using %d vCPU would exceed limit %d", teamID, projectedCPU, quota.MaxCPU)
	}
	if quota.MaxMemoryMB > 0 && projectedMem > quota.MaxMemoryMB {
		return fmt.Errorf("team %s exceeded memory quota: using %d MB would exceed limit %d", teamID, projectedMem, quota.MaxMemoryMB)
	}
	if quota.MaxDiskGB > 0 && projectedDisk > quota.MaxDiskGB {
		return fmt.Errorf("team %s exceeded disk quota: using %d GB would exceed limit %d", teamID, projectedDisk, quota.MaxDiskGB)
	}
	if quota.MaxSandboxes > 0 && projectedSandboxes > quota.MaxSandboxes {
		return fmt.Errorf("team %s exceeded sandbox quota: %d active sandboxes would exceed limit %d", teamID, projectedSandboxes, quota.MaxSandboxes)
	}
	if quota.MaxPreviews > 0 && projectedPreviews > quota.MaxPreviews {
		return fmt.Errorf("team %s exceeded preview quota: %d active previews would exceed limit %d", teamID, projectedPreviews, quota.MaxPreviews)
	}
	return nil
}

func CheckProjectQuota(ctx context.Context, pool *pgxpool.Pool, organizationID, projectID string, delta QuotaDelta) error {
	teamID, err := TeamIDForProject(ctx, pool, organizationID, projectID)
	if err != nil {
		return err
	}
	return CheckTeamQuota(ctx, pool, organizationID, teamID, delta)
}
