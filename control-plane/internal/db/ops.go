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

	cmd := exec.CommandContext(ctx, "pg_dump", "--no-owner", "--no-acl", "--dbname="+dsn, "--file="+path)
	output, err := cmd.CombinedOutput()
	status := "completed"
	message := strings.TrimSpace(string(output))
	var size int64
	if err != nil {
		status = "failed"
		message = fmt.Sprintf("pg_dump failed: %v (%s)", err, message)
	} else if info, statErr := os.Stat(path); statErr == nil {
		size = info.Size()
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
	cmd := exec.CommandContext(ctx, "psql", dsn, "-v", "ON_ERROR_STOP=1", "-f", path)
	output, restoreErr := cmd.CombinedOutput()
	message := strings.TrimSpace(string(output))
	status := "restored"
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
			SELECT 1 FROM alert_events
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
		INSERT INTO alert_events (organization_id, kind, severity, message, resource_type, resource_id)
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
		FROM alert_events ORDER BY id DESC LIMIT $1`, limit)
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
	_, err = tx.Exec(ctx, `UPDATE alert_events SET delivered = TRUE WHERE id = ANY($1::bigint[])`, ids)
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
