package db

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/xiron902224/norix/control-plane/internal/secrets"
)

type Build struct {
	ID             string
	OrganizationID string
	WorkloadID     string
	GitURL         string
	GitRef         string
	DockerfilePath string
	Status         string
	Image          string
	AssignedNodeID string
	LastError      string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type DesiredBuild struct {
	ID             string
	WorkloadID     string
	GitURL         string
	GitRef         string
	DockerfilePath string
	ImageTag       string
	BuildArgs      map[string]string
	Registry       string
}

type ObservedBuild struct {
	ID       string
	Status   string
	Image    string
	Message  string
	LogLines []string
}

func RegistryHost() string {
	if value := strings.TrimSpace(os.Getenv("NORIX_REGISTRY")); value != "" {
		if strings.EqualFold(value, "none") || value == "-" {
			return ""
		}
		return strings.TrimSuffix(value, "/")
	}
	// Empty = local image tags only (no registry required).
	return ""
}

func ImageTagFor(workloadName, buildID string) string {
	short := buildID
	if len(short) > 8 {
		short = short[:8]
	}
	safeName := sanitizeImageName(workloadName)
	if registry := RegistryHost(); registry != "" {
		return fmt.Sprintf("%s/norix/%s:%s", registry, safeName, short)
	}
	return fmt.Sprintf("norix/%s:%s", safeName, short)
}

func CreateBuild(ctx context.Context, pool *pgxpool.Pool, tenantID, workloadID, gitURL, gitRef, dockerfilePath string, containerPort int32, plainSecrets map[string]string, actor AuditActor) (Build, error) {
	gitURL = strings.TrimSpace(gitURL)
	if gitURL == "" {
		return Build{}, fmt.Errorf("git url is required")
	}
	if gitRef == "" {
		gitRef = "main"
	}
	if dockerfilePath == "" {
		dockerfilePath = "Dockerfile"
	}
	if containerPort <= 0 {
		containerPort = 80
	}

	node, err := PickOnlineNode(ctx, pool, tenantID)
	if err != nil {
		return Build{}, err
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return Build{}, err
	}
	defer tx.Rollback(ctx)
	if err := WithTenant(ctx, tx, tenantID); err != nil {
		return Build{}, err
	}

	var workload Workload
	var environmentText string
	err = tx.QueryRow(ctx, `
		UPDATE workloads SET
			source_kind = 'git',
			git_url = $2,
			git_ref = $3,
			dockerfile_path = $4,
			container_port = $5,
			status = 'building',
			last_error = '',
			image_or_source = CASE WHEN image_or_source = '' OR image_or_source = 'pending-build' THEN 'pending-build' ELSE image_or_source END,
			updated_at = NOW()
		WHERE id = $1::uuid AND organization_id = $6::uuid
		RETURNING id::text, organization_id::text, project_id::text, name, runtime, image_or_source, env_vars::text,
		          cpu_shares, memory_limit_mb, COALESCE(domain, ''), status,
		          desired_generation, observed_generation, COALESCE(host_port, 0), container_port,
		          COALESCE(assigned_node_id::text, ''), container_id, last_error, desired_action,
		          source_kind, git_url, git_ref, dockerfile_path, COALESCE(latest_build_id::text, '')`,
		workloadID, gitURL, gitRef, dockerfilePath, containerPort, tenantID,
	).Scan(
		&workload.ID, &workload.OrganizationID, &workload.ProjectID, &workload.Name, &workload.Runtime,
		&workload.ImageOrSource, &environmentText, &workload.CPUShares, &workload.MemoryLimitMB,
		&workload.Domain, &workload.Status, &workload.DesiredGeneration, &workload.ObservedGeneration,
		&workload.HostPort, &workload.ContainerPort, &workload.AssignedNodeID, &workload.ContainerID,
		&workload.LastError, &workload.DesiredAction,
		&workload.SourceKind, &workload.GitURL, &workload.GitRef, &workload.DockerfilePath, &workload.LatestBuildID,
	)
	_ = environmentText
	if err != nil {
		if err == pgx.ErrNoRows {
			return Build{}, fmt.Errorf("workload not found")
		}
		return Build{}, fmt.Errorf("prepare workload for build: %w", err)
	}

	var build Build
	err = tx.QueryRow(ctx, `
		INSERT INTO builds (organization_id, workload_id, git_url, git_ref, dockerfile_path, status, assigned_node_id)
		VALUES ($1::uuid, $2::uuid, $3, $4, $5, 'pending', $6::uuid)
		RETURNING id::text, organization_id::text, workload_id::text, git_url, git_ref, dockerfile_path, status, image,
		          COALESCE(assigned_node_id::text, ''), last_error, created_at, updated_at`,
		tenantID, workloadID, gitURL, gitRef, dockerfilePath, node.ID,
	).Scan(
		&build.ID, &build.OrganizationID, &build.WorkloadID, &build.GitURL, &build.GitRef, &build.DockerfilePath,
		&build.Status, &build.Image, &build.AssignedNodeID, &build.LastError, &build.CreatedAt, &build.UpdatedAt,
	)
	if err != nil {
		return Build{}, fmt.Errorf("create build: %w", err)
	}

	_, err = tx.Exec(ctx, `UPDATE workloads SET latest_build_id = $2::uuid, updated_at = NOW() WHERE id = $1::uuid`,
		workloadID, build.ID)
	if err != nil {
		return Build{}, err
	}

	if len(plainSecrets) > 0 {
		key, keyErr := secrets.KeyFromEnv()
		if keyErr != nil {
			return Build{}, fmt.Errorf("build secrets require NORIX_BUILD_SECRETS_KEY: %w", keyErr)
		}
		for name, value := range plainSecrets {
			name = strings.TrimSpace(name)
			if name == "" || value == "" {
				continue
			}
			ciphertext, nonce, encErr := secrets.Encrypt(key, []byte(value))
			if encErr != nil {
				return Build{}, encErr
			}
			_, err = tx.Exec(ctx, `
				INSERT INTO build_secrets (organization_id, workload_id, name, ciphertext, nonce)
				VALUES ($1::uuid, $2::uuid, $3, $4, $5)
				ON CONFLICT (organization_id, workload_id, name)
				DO UPDATE SET ciphertext = EXCLUDED.ciphertext, nonce = EXCLUDED.nonce, updated_at = NOW()`,
				tenantID, workloadID, name, ciphertext, nonce)
			if err != nil {
				return Build{}, fmt.Errorf("store build secret: %w", err)
			}
		}
	}

	if err := writeAudit(ctx, tx, tenantID, "build.created", "build", build.ID, actor, map[string]string{
		"workload_id": workloadID,
		"git_url":     gitURL,
		"git_ref":     gitRef,
	}); err != nil {
		return Build{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Build{}, err
	}
	return build, nil
}

func ListBuilds(ctx context.Context, pool *pgxpool.Pool, tenantID, workloadID string) ([]Build, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	if err := WithTenant(ctx, tx, tenantID); err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `
		SELECT id::text, organization_id::text, workload_id::text, git_url, git_ref, dockerfile_path, status, image,
		       COALESCE(assigned_node_id::text, ''), last_error, created_at, updated_at
		FROM builds WHERE workload_id = $1::uuid ORDER BY created_at DESC LIMIT 50`, workloadID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]Build, 0)
	for rows.Next() {
		var build Build
		if err := rows.Scan(
			&build.ID, &build.OrganizationID, &build.WorkloadID, &build.GitURL, &build.GitRef, &build.DockerfilePath,
			&build.Status, &build.Image, &build.AssignedNodeID, &build.LastError, &build.CreatedAt, &build.UpdatedAt,
		); err != nil {
			return nil, err
		}
		result = append(result, build)
	}
	return result, rows.Err()
}

func GetBuild(ctx context.Context, pool *pgxpool.Pool, tenantID, buildID string) (Build, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return Build{}, err
	}
	defer tx.Rollback(ctx)
	if err := WithTenant(ctx, tx, tenantID); err != nil {
		return Build{}, err
	}
	var build Build
	err = tx.QueryRow(ctx, `
		SELECT id::text, organization_id::text, workload_id::text, git_url, git_ref, dockerfile_path, status, image,
		       COALESCE(assigned_node_id::text, ''), last_error, created_at, updated_at
		FROM builds WHERE id = $1::uuid`, buildID).Scan(
		&build.ID, &build.OrganizationID, &build.WorkloadID, &build.GitURL, &build.GitRef, &build.DockerfilePath,
		&build.Status, &build.Image, &build.AssignedNodeID, &build.LastError, &build.CreatedAt, &build.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return Build{}, fmt.Errorf("build not found")
		}
		return Build{}, err
	}
	return build, tx.Commit(ctx)
}

// AppendBuildLogLines streams live builder output so the UI can poll mid-build.
func AppendBuildLogLines(ctx context.Context, pool *pgxpool.Pool, tenantID, buildID string, lines []string) error {
	if len(lines) == 0 {
		return nil
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := WithTenant(ctx, tx, tenantID); err != nil {
		return err
	}
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO build_logs (organization_id, build_id, line)
			VALUES ($1::uuid, $2::uuid, $3)`, tenantID, buildID, truncate(line, 4000)); err != nil {
			return err
		}
	}
	_, _ = tx.Exec(ctx, `
		DELETE FROM build_logs
		WHERE build_id = $1::uuid
		  AND id < COALESCE((
			SELECT id FROM build_logs WHERE build_id = $1::uuid ORDER BY id DESC OFFSET 800 LIMIT 1
		  ), 0)`, buildID)
	return tx.Commit(ctx)
}

func ListBuildLogs(ctx context.Context, pool *pgxpool.Pool, tenantID, buildID string, tail int32) ([]string, error) {
	if tail <= 0 || tail > 500 {
		tail = 200
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
			SELECT line, id FROM build_logs WHERE build_id = $1::uuid ORDER BY id DESC LIMIT $2
		) recent ORDER BY id`, buildID, tail)
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

func ListDesiredBuildsForNode(ctx context.Context, pool *pgxpool.Pool, organizationID, nodeID string) ([]DesiredBuild, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	if err := WithTenant(ctx, tx, organizationID); err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `
		SELECT b.id::text, b.workload_id::text, b.git_url, b.git_ref, b.dockerfile_path, w.name
		FROM builds b
		JOIN workloads w ON w.id = b.workload_id
		WHERE b.assigned_node_id = $1::uuid AND b.status = 'pending'
		ORDER BY b.created_at
		LIMIT 5`, nodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	registry := RegistryHost()
	result := make([]DesiredBuild, 0)
	for rows.Next() {
		var item DesiredBuild
		var workloadName string
		if err := rows.Scan(&item.ID, &item.WorkloadID, &item.GitURL, &item.GitRef, &item.DockerfilePath, &workloadName); err != nil {
			return nil, err
		}
		item.Registry = registry
		item.ImageTag = ImageTagFor(workloadName, item.ID)
		item.BuildArgs = map[string]string{}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for i := range result {
		args, argErr := loadBuildArgs(ctx, tx, organizationID, result[i].WorkloadID)
		if argErr != nil {
			return nil, argErr
		}
		result[i].BuildArgs = args
		_, _ = tx.Exec(ctx, `UPDATE builds SET status = 'building', updated_at = NOW() WHERE id = $1::uuid AND status = 'pending'`, result[i].ID)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return result, nil
}

func loadBuildArgs(ctx context.Context, tx pgx.Tx, tenantID, workloadID string) (map[string]string, error) {
	rows, err := tx.Query(ctx, `
		SELECT name, ciphertext, nonce FROM build_secrets
		WHERE organization_id = $1::uuid AND workload_id = $2::uuid`, tenantID, workloadID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := map[string]string{}
	var key []byte
	for rows.Next() {
		var name string
		var ciphertext, nonce []byte
		if err := rows.Scan(&name, &ciphertext, &nonce); err != nil {
			return nil, err
		}
		if key == nil {
			var keyErr error
			key, keyErr = secrets.KeyFromEnv()
			if keyErr != nil {
				return nil, keyErr
			}
		}
		plain, decErr := secrets.Decrypt(key, ciphertext, nonce)
		if decErr != nil {
			return nil, decErr
		}
		result[name] = string(plain)
	}
	return result, rows.Err()
}

func ApplyObservedBuild(ctx context.Context, pool *pgxpool.Pool, organizationID string, observed ObservedBuild, actor AuditActor) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := WithTenant(ctx, tx, organizationID); err != nil {
		return err
	}

	status := strings.TrimSpace(observed.Status)
	if status == "" {
		status = "building"
	}
	var workloadID string
	err = tx.QueryRow(ctx, `
		UPDATE builds SET
			status = $2,
			image = COALESCE(NULLIF($3, ''), image),
			last_error = COALESCE($4, ''),
			updated_at = NOW()
		WHERE id = $1::uuid
		RETURNING workload_id::text`,
		observed.ID, status, observed.Image, observed.Message,
	).Scan(&workloadID)
	if err != nil {
		if err == pgx.ErrNoRows {
			// Workload/build may have been deleted while the builder was still running.
			return nil
		}
		return err
	}

	for _, line := range observed.LogLines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO build_logs (organization_id, build_id, line)
			VALUES ($1::uuid, $2::uuid, $3)`, organizationID, observed.ID, truncate(line, 4000)); err != nil {
			return err
		}
	}
	_, _ = tx.Exec(ctx, `
		DELETE FROM build_logs
		WHERE build_id = $1::uuid
		  AND id < COALESCE((
			SELECT id FROM build_logs WHERE build_id = $1::uuid ORDER BY id DESC OFFSET 800 LIMIT 1
		  ), 0)`, observed.ID)

	if status == "succeeded" && strings.TrimSpace(observed.Image) != "" {
		_, err = tx.Exec(ctx, `
			UPDATE workloads SET
				image_or_source = $2,
				source_kind = 'git',
				status = 'deploying',
				last_error = '',
				updated_at = NOW()
			WHERE id = $1::uuid`, workloadID, observed.Image)
		if err != nil {
			return err
		}
		if err := writeAudit(ctx, tx, organizationID, "build.succeeded", "build", observed.ID, actor, map[string]string{
			"image": observed.Image,
		}); err != nil {
			return err
		}
		if err := tx.Commit(ctx); err != nil {
			return err
		}
		// Deploy outside the build tx so port allocation / deployment row stay consistent.
		node, pickErr := PickOnlineNode(ctx, pool, organizationID)
		if pickErr != nil {
			return pickErr
		}
		workload, getErr := GetWorkload(ctx, pool, organizationID, workloadID)
		if getErr != nil {
			return getErr
		}
		hostPort := workload.HostPort
		if hostPort <= 0 {
			hostPort, getErr = AllocateHostPort(ctx, pool, organizationID)
			if getErr != nil {
				return getErr
			}
		}
		_, _, deployErr := DeployWorkload(ctx, pool, organizationID, workloadID, node.ID, hostPort, workload.ContainerPort, actor)
		return deployErr
	}

	if status == "failed" {
		_, _ = tx.Exec(ctx, `
			UPDATE workloads SET status = 'failed', last_error = $2, desired_action = 'none', updated_at = NOW()
			WHERE id = $1::uuid`, workloadID, observed.Message)
		_, _ = tx.Exec(ctx, `
			UPDATE workload_replicas SET
				desired_action = 'none',
				status = 'failed',
				last_error = $2,
				updated_at = NOW()
			WHERE workload_id = $1::uuid AND desired_action IN ('run', 'stop')`, workloadID, observed.Message)
		_ = writeAudit(ctx, tx, organizationID, "build.failed", "build", observed.ID, actor, map[string]string{
			"error": observed.Message,
		})
	}
	return tx.Commit(ctx)
}

func FindWorkloadByGitURL(ctx context.Context, pool *pgxpool.Pool, tenantID, gitURL string) (Workload, error) {
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
		FROM workloads
		WHERE source_kind = 'git' AND git_url = $1 AND status <> 'deleting'
		ORDER BY updated_at DESC
		LIMIT 1`, gitURL)
	if err != nil {
		return Workload{}, err
	}
	return workload, tx.Commit(ctx)
}

func FindWebhookToken(ctx context.Context, pool *pgxpool.Pool, tokenHash string) (orgID, workloadID string, err error) {
	err = pool.QueryRow(ctx, `
		SELECT organization_id::text, workload_id::text
		FROM webhook_tokens WHERE token_hash = $1`, tokenHash).Scan(&orgID, &workloadID)
	if err == pgx.ErrNoRows {
		return "", "", fmt.Errorf("webhook token not found")
	}
	return orgID, workloadID, err
}

func EnsureWebhookToken(ctx context.Context, pool *pgxpool.Pool, tenantID, workloadID, tokenHash string) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := WithTenant(ctx, tx, tenantID); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO webhook_tokens (organization_id, workload_id, token_hash)
		VALUES ($1::uuid, $2::uuid, $3)
		ON CONFLICT (token_hash) DO NOTHING`, tenantID, workloadID, tokenHash)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func sanitizeImageName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	var b strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "app"
	}
	if len(out) > 40 {
		return out[:40]
	}
	return out
}

func ReclaimPendingBuilds(ctx context.Context, pool *pgxpool.Pool, organizationID, onlineNodeID string) (int64, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	if err := WithTenant(ctx, tx, organizationID); err != nil {
		return 0, err
	}
	tag, err := tx.Exec(ctx, `
		UPDATE builds
		SET assigned_node_id = $1::uuid,
		    status = 'pending',
		    updated_at = NOW()
		WHERE status IN ('pending', 'building')
		  AND (
			assigned_node_id IS NULL
			OR assigned_node_id IN (
				SELECT id FROM nodes WHERE organization_id = $2::uuid AND status <> 'online'
			)
		)`, onlineNodeID, organizationID)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), tx.Commit(ctx)
}
