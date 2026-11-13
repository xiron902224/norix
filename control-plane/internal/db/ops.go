package db

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Backup struct {
	ID             string
	OrganizationID string
	Filename       string
	SizeBytes      int64
	Status         string
	Message        string
	CreatedBy      string
	CreatedAt      time.Time
}

type AlertEvent struct {
	ID           int64
	Kind         string
	Severity     string
	Message      string
	ResourceType string
	ResourceID   string
	Delivered    bool
	CreatedAt    time.Time
}

func BackupDir() string {
	if value := strings.TrimSpace(os.Getenv("NORIX_BACKUP_DIR")); value != "" {
		return value
	}
	return filepath.Join(os.TempDir(), "norix-backups")
}

func pgTool(envKey, fallback string) (string, error) {
	if value := strings.TrimSpace(os.Getenv(envKey)); value != "" {
		return value, nil
	}
	if path, err := exec.LookPath(fallback); err == nil {
		return path, nil
	}
	return "", fmt.Errorf("%s not found in PATH (install postgresql-client or set %s)", fallback, envKey)
}

func postgresDockerContainer() string {
	if value := strings.TrimSpace(os.Getenv("NORIX_PG_DOCKER")); value != "" {
		return value
	}
	// Local compose default.
	return "norix-postgres-1"
}

func CreatePostgresBackup(ctx context.Context, pool *pgxpool.Pool, tenantID, createdBy string) (Backup, error) {
	dir := BackupDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return Backup{}, err
	}
	stamp := time.Now().UTC().Format("20060102-150405")
	filename := fmt.Sprintf("norix-%s-%s.sql", tenantID[:8], stamp)
	path := filepath.Join(dir, filename)

	dsn := os.Getenv("MIGRATION_DATABASE_URL")
	if dsn == "" {
		dsn = os.Getenv("DATABASE_URL")
	}
	if dsn == "" {
		return Backup{}, fmt.Errorf("DATABASE_URL required for backups")
	}

	status := "completed"
	message := ""
	var size int64

	if bin, err := pgTool("NORIX_PG_DUMP", "pg_dump"); err == nil {
		cmd := exec.CommandContext(ctx, bin, "--no-owner", "--no-acl", "--dbname="+dsn, "--file="+path)
		output, runErr := cmd.CombinedOutput()
		message = strings.TrimSpace(string(output))
		if runErr != nil {
			status = "failed"
			message = fmt.Sprintf("pg_dump failed: %v (%s)", runErr, message)
		}
	} else {
		// Fallback: stream pg_dump from the Postgres docker container onto the host file.
		outFile, createErr := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
		if createErr != nil {
			return Backup{}, createErr
		}
		container := postgresDockerContainer()
		cmd := exec.CommandContext(ctx, "docker", "exec", "-i", container, "pg_dump", "-U", "norix", "--no-owner", "--no-acl", "norix")
		cmd.Stdout = outFile
		var stderr strings.Builder
		cmd.Stderr = &stderr
		runErr := cmd.Run()
		_ = outFile.Close()
		message = strings.TrimSpace(stderr.String())
		if runErr != nil {
			status = "failed"
			message = fmt.Sprintf("pg_dump via docker (%s) failed: %v (%s); also: %v", container, runErr, message, err)
		} else if message == "" {
			message = fmt.Sprintf("pg_dump via docker container %s", container)
		}
	}

	if status == "completed" {
		if info, statErr := os.Stat(path); statErr == nil {
			size = info.Size()
		}
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return Backup{}, err
	}
	defer tx.Rollback(ctx)
	if err := WithTenant(ctx, tx, tenantID); err != nil {
		return Backup{}, err
	}
	var backup Backup
	err = tx.QueryRow(ctx, `
		INSERT INTO backups (organization_id, filename, size_bytes, status, message, created_by)
		VALUES ($1::uuid, $2, $3, $4, $5, $6)
		RETURNING id::text, organization_id::text, filename, size_bytes, status, message, created_by, created_at`,
		tenantID, filename, size, status, message, createdBy,
	).Scan(&backup.ID, &backup.OrganizationID, &backup.Filename, &backup.SizeBytes, &backup.Status, &backup.Message, &backup.CreatedBy, &backup.CreatedAt)
	if err != nil {
		return Backup{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Backup{}, err
	}
	if status == "failed" {
		return backup, fmt.Errorf("%s", message)
	}
	return backup, nil
}

func GetBackup(ctx context.Context, pool *pgxpool.Pool, tenantID, backupID string) (Backup, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return Backup{}, err
	}
	defer tx.Rollback(ctx)
	if err := WithTenant(ctx, tx, tenantID); err != nil {
		return Backup{}, err
	}
	var backup Backup
	err = tx.QueryRow(ctx, `
		SELECT id::text, organization_id::text, filename, size_bytes, status, message, created_by, created_at
		FROM backups WHERE id = $1::uuid`, backupID).Scan(
		&backup.ID, &backup.OrganizationID, &backup.Filename, &backup.SizeBytes, &backup.Status, &backup.Message, &backup.CreatedBy, &backup.CreatedAt,
	)
	if err != nil {
		return Backup{}, fmt.Errorf("backup not found")
	}
	return backup, tx.Commit(ctx)
}

func ListBackups(ctx context.Context, pool *pgxpool.Pool, tenantID string) ([]Backup, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	if err := WithTenant(ctx, tx, tenantID); err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `
		SELECT id::text, organization_id::text, filename, size_bytes, status, message, created_by, created_at
		FROM backups ORDER BY created_at DESC LIMIT 50`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]Backup, 0)
	for rows.Next() {
		var item Backup
		if err := rows.Scan(&item.ID, &item.OrganizationID, &item.Filename, &item.SizeBytes, &item.Status, &item.Message, &item.CreatedBy, &item.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func RestorePostgresBackup(ctx context.Context, pool *pgxpool.Pool, tenantID, backupID string) (Backup, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return Backup{}, err
	}
	defer tx.Rollback(ctx)
	if err := WithTenant(ctx, tx, tenantID); err != nil {
		return Backup{}, err
	}
	var backup Backup
	err = tx.QueryRow(ctx, `
		SELECT id::text, organization_id::text, filename, size_bytes, status, message, created_by, created_at
		FROM backups WHERE id = $1::uuid`, backupID).Scan(
		&backup.ID, &backup.OrganizationID, &backup.Filename, &backup.SizeBytes, &backup.Status, &backup.Message, &backup.CreatedBy, &backup.CreatedAt,
	)
	if err != nil {
		return Backup{}, fmt.Errorf("backup not found")
	}
	if err := tx.Commit(ctx); err != nil {
		return Backup{}, err
	}

	path := filepath.Join(BackupDir(), backup.Filename)
	dsn := os.Getenv("MIGRATION_DATABASE_URL")
	if dsn == "" {
		dsn = os.Getenv("DATABASE_URL")
	}

	status := "restored"
	message := ""
	var restoreErr error

	if bin, err := pgTool("NORIX_PSQL", "psql"); err == nil {
		cmd := exec.CommandContext(ctx, bin, dsn, "-v", "ON_ERROR_STOP=1", "-f", path)
		output, err := cmd.CombinedOutput()
		message = strings.TrimSpace(string(output))
		restoreErr = err
	} else {
		container := postgresDockerContainer()
		// Copy dump into the container then restore.
		remote := "/tmp/" + filepath.Base(path)
		copyCmd := exec.CommandContext(ctx, "docker", "cp", path, container+":"+remote)
		if out, copyErr := copyCmd.CombinedOutput(); copyErr != nil {
			restoreErr = copyErr
			message = strings.TrimSpace(string(out))
		} else {
			cmd := exec.CommandContext(ctx, "docker", "exec", "-i", container, "psql", "-U", "norix", "-d", "norix", "-v", "ON_ERROR_STOP=1", "-f", remote)
			output, err := cmd.CombinedOutput()
			message = strings.TrimSpace(string(output))
			restoreErr = err
			if restoreErr == nil && message == "" {
				message = fmt.Sprintf("restored via docker container %s", container)
			}
		}
		if restoreErr != nil && message == "" {
			message = err.Error()
		}
	}

	if restoreErr != nil {
		status = "failed"
		message = fmt.Sprintf("restore failed: %v (%s)", restoreErr, message)
	}

	tx2, err := pool.Begin(ctx)
	if err != nil {
		return Backup{}, err
	}
	defer tx2.Rollback(ctx)
	if err := WithTenant(ctx, tx2, tenantID); err != nil {
		return Backup{}, err
	}
	_, _ = tx2.Exec(ctx, `UPDATE backups SET status = $2, message = $3 WHERE id = $1::uuid`, backupID, status, message)
	_ = tx2.Commit(ctx)
	backup.Status = status
	backup.Message = message
	if restoreErr != nil {
		return backup, restoreErr
	}
	return backup, nil
}

func HasRecentAlert(ctx context.Context, pool *pgxpool.Pool, tenantID, kind, resourceType, resourceID string, within time.Duration) (bool, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	if err := WithTenant(ctx, tx, tenantID); err != nil {
		return false, err
	}
	var exists bool
	err = tx.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM ops_alert_events
			WHERE kind = $1 AND resource_type = $2 AND resource_id = $3
			  AND created_at > NOW() - make_interval(secs => $4::double precision)
		)`, kind, resourceType, resourceID, within.Seconds(),
	).Scan(&exists)
	if err != nil {
		return false, err
	}
	return exists, nil
}

func InsertAlert(ctx context.Context, pool *pgxpool.Pool, tenantID, kind, severity, message, resourceType, resourceID string) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := WithTenant(ctx, tx, tenantID); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO ops_alert_events (organization_id, kind, severity, message, resource_type, resource_id)
		VALUES ($1::uuid, $2, $3, $4, $5, $6)`,
		tenantID, kind, severity, message, resourceType, resourceID)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func ListAlerts(ctx context.Context, pool *pgxpool.Pool, tenantID string, limit int32) ([]AlertEvent, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
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
		SELECT id, kind, severity, message, resource_type, resource_id, delivered, created_at
		FROM ops_alert_events ORDER BY id DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]AlertEvent, 0)
	for rows.Next() {
		var item AlertEvent
		if err := rows.Scan(&item.ID, &item.Kind, &item.Severity, &item.Message, &item.ResourceType, &item.ResourceID, &item.Delivered, &item.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func MarkAlertsDelivered(ctx context.Context, pool *pgxpool.Pool, tenantID string, ids []int64) error {
	if len(ids) == 0 {
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
	_, err = tx.Exec(ctx, `UPDATE ops_alert_events SET delivered = TRUE WHERE id = ANY($1::bigint[])`, ids)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func RetainWorkloadLogs(ctx context.Context, pool *pgxpool.Pool, keep int) error {
	if keep <= 0 {
		keep = 500
	}
	_, err := pool.Exec(ctx, `
		DELETE FROM workload_logs wl
		USING (
			SELECT workload_id, id FROM (
				SELECT workload_id, id,
				       ROW_NUMBER() OVER (PARTITION BY workload_id ORDER BY id DESC) AS rn
				FROM workload_logs
			) ranked WHERE rn > $1
		) old
		WHERE wl.id = old.id`, keep)
	return err
}

func RetainBuildLogs(ctx context.Context, pool *pgxpool.Pool, keep int) error {
	if keep <= 0 {
		keep = 800
	}
	_, err := pool.Exec(ctx, `
		DELETE FROM build_logs bl
		USING (
			SELECT build_id, id FROM (
				SELECT build_id, id,
				       ROW_NUMBER() OVER (PARTITION BY build_id ORDER BY id DESC) AS rn
				FROM build_logs
			) ranked WHERE rn > $1
		) old
		WHERE bl.id = old.id`, keep)
	return err
}
