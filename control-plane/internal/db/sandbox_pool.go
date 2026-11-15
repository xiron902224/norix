package db

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const warmPoolNamePrefix = "warm-"

// SandboxWarmPoolSize / WarmPoolTarget — how many warm sandboxes to keep ready per org.
func SandboxWarmPoolSize() int { return WarmPoolTarget() }

func WarmPoolTarget() int {
	raw := strings.TrimSpace(firstNonEmpty(
		os.Getenv("NORIX_SANDBOX_WARM_POOL_SIZE"),
		os.Getenv("NORIX_SANDBOX_WARM_POOL"),
	))
	if raw == "" {
		return 2
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 {
		return 2
	}
	if n > 32 {
		return 32
	}
	return n
}

// EnsureSandboxWarmPool keeps N ready+pool_ready warm sandboxes and their containers.
func EnsureSandboxWarmPool(ctx context.Context, pool *pgxpool.Pool, tenantID string) error {
	target := WarmPoolTarget()
	if target == 0 {
		return nil
	}
	projectID, err := firstProjectID(ctx, pool, tenantID)
	if err != nil || projectID == "" {
		return nil
	}
	ready, err := countWarmPoolReady(ctx, pool, tenantID)
	if err != nil {
		return err
	}
	for ready < int64(target) {
		item, createErr := CreateSandbox(ctx, pool, tenantID, Sandbox{
			ProjectID:  projectID,
			Name:       fmt.Sprintf("%s%d-%d", warmPoolNamePrefix, time.Now().UTC().UnixNano(), ready),
			Template:   warmPoolTemplate(),
			TTLSeconds: 86400,
			CPU:        1,
			MemoryMB:   256,
			EgressMode: "restricted",
		})
		if createErr != nil {
			return createErr
		}
		_ = EnsureSandboxContainer(ctx, item)
		if !item.PoolReady {
			_ = MarkSandboxReady(ctx, pool, tenantID, item.ID, true)
		}
		ready++
	}
	items, listErr := listWarmPoolSandboxes(ctx, pool, tenantID)
	if listErr != nil {
		return listErr
	}
	for _, item := range items {
		if item.Status == "ready" {
			_ = EnsureSandboxContainer(ctx, item)
		}
	}
	return nil
}

func warmPoolTemplate() string {
	if v := strings.TrimSpace(os.Getenv("NORIX_SANDBOX_WARM_TEMPLATE")); v != "" {
		return v
	}
	return "blank"
}

func firstProjectID(ctx context.Context, pool *pgxpool.Pool, tenantID string) (string, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)
	if err := WithTenant(ctx, tx, tenantID); err != nil {
		return "", err
	}
	var id string
	err = tx.QueryRow(ctx, `SELECT id::text FROM projects WHERE organization_id = $1::uuid ORDER BY created_at LIMIT 1`, tenantID).Scan(&id)
	if err != nil {
		return "", err
	}
	return id, tx.Commit(ctx)
}

func countWarmPoolReady(ctx context.Context, pool *pgxpool.Pool, tenantID string) (int64, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	if err := WithTenant(ctx, tx, tenantID); err != nil {
		return 0, err
	}
	var n int64
	err = tx.QueryRow(ctx, `
		SELECT COUNT(*) FROM sandboxes
		WHERE organization_id = $1::uuid
		  AND name LIKE $2
		  AND status = 'ready'
		  AND pool_ready = TRUE
		  AND expires_at > NOW()`,
		tenantID, warmPoolNamePrefix+"%",
	).Scan(&n)
	if err != nil {
		return 0, err
	}
	return n, tx.Commit(ctx)
}

func listWarmPoolSandboxes(ctx context.Context, pool *pgxpool.Pool, tenantID string) ([]Sandbox, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	if err := WithTenant(ctx, tx, tenantID); err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `
		SELECT id::text, organization_id::text, project_id::text, name, template, status,
		       ttl_seconds, cpu, memory_mb, egress_mode, expires_at, pool_ready
		FROM sandboxes
		WHERE organization_id = $1::uuid AND name LIKE $2 AND status = 'ready'`,
		tenantID, warmPoolNamePrefix+"%",
	)
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

// ClaimWarmPoolSandbox renames a warm pool sandbox into a user sandbox when available.
func ClaimWarmPoolSandbox(ctx context.Context, pool *pgxpool.Pool, tenantID string, want Sandbox) (Sandbox, bool, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return Sandbox{}, false, err
	}
	defer tx.Rollback(ctx)
	if err := WithTenant(ctx, tx, tenantID); err != nil {
		return Sandbox{}, false, err
	}
	var item Sandbox
	err = tx.QueryRow(ctx, `
		SELECT id::text, organization_id::text, project_id::text, name, template, status,
		       ttl_seconds, cpu, memory_mb, egress_mode, expires_at, pool_ready
		FROM sandboxes
		WHERE organization_id = $1::uuid
		  AND name LIKE $2
		  AND status = 'ready'
		  AND pool_ready = TRUE
		  AND expires_at > NOW()
		ORDER BY created_at
		LIMIT 1
		FOR UPDATE SKIP LOCKED`,
		tenantID, warmPoolNamePrefix+"%",
	).Scan(
		&item.ID, &item.OrganizationID, &item.ProjectID, &item.Name, &item.Template, &item.Status,
		&item.TTLSeconds, &item.CPU, &item.MemoryMB, &item.EgressMode, &item.ExpiresAt, &item.PoolReady,
	)
	if err != nil {
		_ = tx.Rollback(ctx)
		return Sandbox{}, false, nil
	}
	ttl := want.TTLSeconds
	if ttl <= 0 {
		ttl = 3600
	}
	expires := time.Now().UTC().Add(time.Duration(ttl) * time.Second)
	template := want.Template
	if template == "" {
		template = item.Template
	}
	err = tx.QueryRow(ctx, `
		UPDATE sandboxes SET
			project_id = $2::uuid,
			name = $3,
			template = $4,
			ttl_seconds = $5,
			cpu = COALESCE(NULLIF($6, 0), cpu),
			memory_mb = COALESCE(NULLIF($7, 0), memory_mb),
			egress_mode = CASE WHEN $8 = '' THEN egress_mode ELSE $8 END,
			expires_at = $9,
			status = 'ready',
			pool_ready = TRUE,
			updated_at = NOW()
		WHERE id = $1::uuid
		RETURNING id::text, organization_id::text, project_id::text, name, template, status,
		          ttl_seconds, cpu, memory_mb, egress_mode, expires_at, pool_ready`,
		item.ID, want.ProjectID, want.Name, template, ttl, want.CPU, want.MemoryMB, want.EgressMode, expires,
	).Scan(
		&item.ID, &item.OrganizationID, &item.ProjectID, &item.Name, &item.Template, &item.Status,
		&item.TTLSeconds, &item.CPU, &item.MemoryMB, &item.EgressMode, &item.ExpiresAt, &item.PoolReady,
	)
	if err != nil {
		return Sandbox{}, false, err
	}
	return item, true, tx.Commit(ctx)
}

// MarkSandboxReady sets status/pool_ready after the runtime container is up.
func MarkSandboxReady(ctx context.Context, pool *pgxpool.Pool, tenantID, sandboxID string, poolReady bool) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := WithTenant(ctx, tx, tenantID); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		UPDATE sandboxes SET status = 'ready', pool_ready = $3, updated_at = NOW()
		WHERE id = $1::uuid AND organization_id = $2::uuid`,
		sandboxID, tenantID, poolReady)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// SandboxContainerName is the docker/nerdctl name for a sandbox.
func SandboxContainerName(sandboxID string) string {
	id := strings.ReplaceAll(sandboxID, "-", "")
	if len(id) > 12 {
		id = id[:12]
	}
	return "norix-sandbox-" + id
}

// EnsureSandboxContainer starts a long-lived warm container for the sandbox when docker is available.
func EnsureSandboxContainer(ctx context.Context, item Sandbox) error {
	engine, err := containerEngine()
	if err != nil {
		return err
	}
	name := SandboxContainerName(item.ID)
	inspect := exec.CommandContext(ctx, engine, "inspect", "-f", "{{.State.Running}}", name)
	if out, err := inspect.CombinedOutput(); err == nil && strings.TrimSpace(string(out)) == "true" {
		return nil
	}
	_ = exec.CommandContext(ctx, engine, "rm", "-f", name).Run()

	image := sandboxImageForTemplate(item.Template)
	mem := fmt.Sprintf("%dm", item.MemoryMB)
	if item.MemoryMB <= 0 {
		mem = "256m"
	}
	args := []string{
		"run", "-d",
		"--name", name,
		"--memory", mem,
		"--label", "norix.sandbox_id=" + item.ID,
		"--label", "norix.sandbox=1",
		"--label", "norix.warm_pool=1",
	}
	if item.EgressMode != "allow" && item.EgressMode != "open" {
		args = append(args, "--network", "none")
	}
	args = append(args, image, "sleep", "infinity")
	cmd := exec.CommandContext(ctx, engine, args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("start sandbox container: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func containerEngine() (string, error) {
	if _, err := exec.LookPath("docker"); err == nil {
		return "docker", nil
	}
	if _, err := exec.LookPath("nerdctl"); err == nil {
		return "nerdctl", nil
	}
	return "", fmt.Errorf("docker/nerdctl not available")
}

func sandboxImageForTemplate(template string) string {
	switch strings.ToLower(strings.TrimSpace(template)) {
	case "python", "python3":
		return "python:3.12-alpine"
	case "node", "nodejs":
		return "node:20-alpine"
	case "go", "golang":
		return "golang:1.22-alpine"
	default:
		return "alpine:3.20"
	}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
