package db

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Sandbox struct {
	ID             string
	OrganizationID string
	ProjectID      string
	Name           string
	Template       string
	Status         string
	TTLSeconds     int32
	CPU            int32
	MemoryMB       int32
	EgressMode     string
	ExpiresAt      time.Time
	PoolReady      bool
}

func CreateSandbox(ctx context.Context, pool *pgxpool.Pool, tenantID string, sandbox Sandbox) (Sandbox, error) {
	sandbox.Name = strings.TrimSpace(sandbox.Name)
	if sandbox.ProjectID == "" || sandbox.Name == "" {
		return Sandbox{}, fmt.Errorf("project and name are required")
	}
	if sandbox.TTLSeconds <= 0 {
		sandbox.TTLSeconds = 3600
	}
	if sandbox.CPU <= 0 {
		sandbox.CPU = 1
	}
	if sandbox.MemoryMB <= 0 {
		sandbox.MemoryMB = 512
	}
	if sandbox.Template == "" {
		sandbox.Template = "blank"
	}
	if sandbox.EgressMode == "" {
		sandbox.EgressMode = "restricted"
	}

	// Prefer claiming a warm-pool sandbox (already pool_ready + container).
	if !strings.HasPrefix(sandbox.Name, warmPoolNamePrefix) {
		if claimed, ok, claimErr := ClaimWarmPoolSandbox(ctx, pool, tenantID, sandbox); claimErr == nil && ok {
			_ = EnsureSandboxContainer(ctx, claimed)
			return claimed, nil
		}
	}

	expires := time.Now().UTC().Add(time.Duration(sandbox.TTLSeconds) * time.Second)

	tx, err := pool.Begin(ctx)
	if err != nil {
		return Sandbox{}, err
	}
	defer tx.Rollback(ctx)
	if err := WithTenant(ctx, tx, tenantID); err != nil {
		return Sandbox{}, err
	}
	err = tx.QueryRow(ctx, `
		INSERT INTO sandboxes (organization_id, project_id, name, template, status, ttl_seconds, cpu, memory_mb, egress_mode, expires_at, pool_ready)
		VALUES ($1::uuid, $2::uuid, $3, $4, 'pending', $5, $6, $7, $8, $9, FALSE)
		RETURNING id::text, organization_id::text, project_id::text, name, template, status,
		          ttl_seconds, cpu, memory_mb, egress_mode, expires_at, pool_ready`,
		tenantID, sandbox.ProjectID, sandbox.Name, sandbox.Template,
		sandbox.TTLSeconds, sandbox.CPU, sandbox.MemoryMB, sandbox.EgressMode, expires,
	).Scan(
		&sandbox.ID, &sandbox.OrganizationID, &sandbox.ProjectID, &sandbox.Name, &sandbox.Template, &sandbox.Status,
		&sandbox.TTLSeconds, &sandbox.CPU, &sandbox.MemoryMB, &sandbox.EgressMode, &sandbox.ExpiresAt, &sandbox.PoolReady,
	)
	if err != nil {
		return Sandbox{}, fmt.Errorf("create sandbox: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Sandbox{}, err
	}

	poolReady := true
	if err := EnsureSandboxContainer(ctx, sandbox); err != nil {
		// Container runtime optional in CI; still mark ready so API stays usable.
		poolReady = false
	}
	if err := MarkSandboxReady(ctx, pool, tenantID, sandbox.ID, poolReady); err != nil {
		return Sandbox{}, err
	}
	sandbox.Status = "ready"
	sandbox.PoolReady = poolReady
	return sandbox, nil
}

func ListSandboxes(ctx context.Context, pool *pgxpool.Pool, tenantID, projectID string) ([]Sandbox, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	if err := WithTenant(ctx, tx, tenantID); err != nil {
		return nil, err
	}
	query := `
		SELECT id::text, organization_id::text, project_id::text, name, template, status,
		       ttl_seconds, cpu, memory_mb, egress_mode, expires_at, pool_ready
		FROM sandboxes WHERE organization_id = $1::uuid`
	args := []any{tenantID}
	if projectID != "" {
		query += ` AND project_id = $2::uuid`
		args = append(args, projectID)
	}
	query += ` ORDER BY created_at DESC`
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Sandbox, 0)
	for rows.Next() {
		var item Sandbox
		if err := rows.Scan(
			&item.ID, &item.OrganizationID, &item.ProjectID, &item.Name, &item.Template, &item.Status,
			&item.TTLSeconds, &item.CPU, &item.MemoryMB, &item.EgressMode, &item.ExpiresAt, &item.PoolReady,
		); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, tx.Commit(ctx)
}

func GetSandbox(ctx context.Context, pool *pgxpool.Pool, tenantID, sandboxID string) (Sandbox, error) {
	if strings.TrimSpace(sandboxID) == "" {
		return Sandbox{}, fmt.Errorf("sandbox id is required")
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return Sandbox{}, err
	}
	defer tx.Rollback(ctx)
	if err := WithTenant(ctx, tx, tenantID); err != nil {
		return Sandbox{}, err
	}
	var item Sandbox
	err = tx.QueryRow(ctx, `
		SELECT id::text, organization_id::text, project_id::text, name, template, status,
		       ttl_seconds, cpu, memory_mb, egress_mode, expires_at, pool_ready
		FROM sandboxes WHERE id = $1::uuid AND organization_id = $2::uuid`,
		sandboxID, tenantID,
	).Scan(
		&item.ID, &item.OrganizationID, &item.ProjectID, &item.Name, &item.Template, &item.Status,
		&item.TTLSeconds, &item.CPU, &item.MemoryMB, &item.EgressMode, &item.ExpiresAt, &item.PoolReady,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Sandbox{}, fmt.Errorf("sandbox not found: %w", err)
		}
		return Sandbox{}, fmt.Errorf("get sandbox: %w", err)
	}
	return item, tx.Commit(ctx)
}

func DestroySandbox(ctx context.Context, pool *pgxpool.Pool, tenantID, sandboxID string) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := WithTenant(ctx, tx, tenantID); err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `
		UPDATE sandboxes SET status = 'destroyed', pool_ready = FALSE, updated_at = NOW()
		WHERE id = $1::uuid`, sandboxID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("sandbox not found")
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	if engine, engErr := containerEngine(); engErr == nil {
		_ = exec.CommandContext(ctx, engine, "rm", "-f", SandboxContainerName(sandboxID)).Run()
	}
	return nil
}

func GCExpiredSandboxes(ctx context.Context, pool *pgxpool.Pool, tenantID string) (int64, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	if err := WithTenant(ctx, tx, tenantID); err != nil {
		return 0, err
	}
	tag, err := tx.Exec(ctx, `
		UPDATE sandboxes SET status = 'expired', pool_ready = FALSE, updated_at = NOW()
		WHERE status NOT IN ('destroyed', 'expired') AND expires_at < NOW()`)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), tx.Commit(ctx)
}
