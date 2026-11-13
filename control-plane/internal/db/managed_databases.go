package db

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/xiron902224/norix/control-plane/internal/secrets"
)

type ManagedDatabase struct {
	ID             string
	OrganizationID string
	ProjectID      string
	Engine         string
	Name           string
	Status         string
	NodeID         string
	VolumePath     string
	BackupSchedule string
	LastBackupAt   *time.Time
}

func engineFromString(engine string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(engine)) {
	case "postgres", "mysql", "redis", "mongodb":
		return strings.ToLower(engine), nil
	default:
		return "", fmt.Errorf("unsupported database engine")
	}
}

func CreateManagedDatabase(ctx context.Context, pool *pgxpool.Pool, tenantID, projectID, engine, name, backupSchedule string) (ManagedDatabase, error) {
	engine, err := engineFromString(engine)
	if err != nil {
		return ManagedDatabase{}, err
	}
	name = strings.TrimSpace(name)
	if name == "" || projectID == "" {
		return ManagedDatabase{}, fmt.Errorf("project and name are required")
	}
	password := randomPassword(24)
	connJSON := fmt.Sprintf(`{"password":%q,"user":%q}`, password, "norix_"+name)

	key, keyErr := secrets.KeyFromEnv()
	var secretEnc, secretNonce []byte
	if keyErr == nil {
		secretEnc, secretNonce, err = secrets.Encrypt(key, []byte(connJSON))
		if err != nil {
			return ManagedDatabase{}, err
		}
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return ManagedDatabase{}, err
	}
	defer tx.Rollback(ctx)
	if err := WithTenant(ctx, tx, tenantID); err != nil {
		return ManagedDatabase{}, err
	}
	var item ManagedDatabase
	var lastBackup *time.Time
	err = tx.QueryRow(ctx, `
		INSERT INTO managed_databases (organization_id, project_id, engine, name, status, volume_path,
		                               connection_secret_enc, connection_secret_nonce, backup_schedule)
		VALUES ($1::uuid, $2::uuid, $3, $4, 'provisioning', $5, $6, $7, $8)
		RETURNING id::text, organization_id::text, project_id::text, engine, name, status,
		          COALESCE(node_id::text, ''), volume_path, backup_schedule, last_backup_at`,
		tenantID, projectID, engine, name, fmt.Sprintf("/var/lib/norix/dbs/%s", name),
		secretEnc, secretNonce, backupSchedule,
	).Scan(
		&item.ID, &item.OrganizationID, &item.ProjectID, &item.Engine, &item.Name, &item.Status,
		&item.NodeID, &item.VolumePath, &item.BackupSchedule, &lastBackup,
	)
	if err != nil {
		return ManagedDatabase{}, fmt.Errorf("create managed database: %w", err)
	}
	item.LastBackupAt = lastBackup
	_, _ = tx.Exec(ctx, `UPDATE managed_databases SET status = 'ready', updated_at = NOW() WHERE id = $1::uuid`, item.ID)
	item.Status = "ready"
	if err := tx.Commit(ctx); err != nil {
		return ManagedDatabase{}, err
	}
	return item, nil
}

func ListManagedDatabases(ctx context.Context, pool *pgxpool.Pool, tenantID, projectID string) ([]ManagedDatabase, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	if err := WithTenant(ctx, tx, tenantID); err != nil {
		return nil, err
	}
	query := `
		SELECT id::text, organization_id::text, project_id::text, engine, name, status,
		       COALESCE(node_id::text, ''), volume_path, backup_schedule, last_backup_at
		FROM managed_databases WHERE organization_id = $1::uuid`
	args := []any{tenantID}
	if projectID != "" {
		query += ` AND project_id = $2::uuid`
		args = append(args, projectID)
	}
	query += ` ORDER BY name`
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]ManagedDatabase, 0)
	for rows.Next() {
		var item ManagedDatabase
		var lastBackup *time.Time
		if err := rows.Scan(
			&item.ID, &item.OrganizationID, &item.ProjectID, &item.Engine, &item.Name, &item.Status,
			&item.NodeID, &item.VolumePath, &item.BackupSchedule, &lastBackup,
		); err != nil {
			return nil, err
		}
		item.LastBackupAt = lastBackup
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, tx.Commit(ctx)
}

func EnsureProvisioningDatabases(ctx context.Context, pool *pgxpool.Pool, tenantID, nodeID string) (int64, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	if err := WithTenant(ctx, tx, tenantID); err != nil {
		return 0, err
	}
	tag, err := tx.Exec(ctx, `
		UPDATE managed_databases SET status = 'ready', node_id = $2::uuid, updated_at = NOW()
		WHERE organization_id = $1::uuid
		  AND status = 'provisioning'
		  AND (node_id IS NULL OR node_id = $2::uuid)`,
		tenantID, nodeID)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), tx.Commit(ctx)
}

func BackupManagedDatabase(ctx context.Context, pool *pgxpool.Pool, tenantID, databaseID string) (ManagedDatabase, error) {
	return finishManagedDatabaseOperation(ctx, pool, tenantID, databaseID, "backing_up", true)
}

func RestoreManagedDatabase(ctx context.Context, pool *pgxpool.Pool, tenantID, databaseID string) (ManagedDatabase, error) {
	return finishManagedDatabaseOperation(ctx, pool, tenantID, databaseID, "restoring", false)
}

func finishManagedDatabaseOperation(ctx context.Context, pool *pgxpool.Pool, tenantID, databaseID, transientStatus string, touchBackup bool) (ManagedDatabase, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return ManagedDatabase{}, err
	}
	defer tx.Rollback(ctx)
	if err := WithTenant(ctx, tx, tenantID); err != nil {
		return ManagedDatabase{}, err
	}
	if touchBackup {
		tag, err := tx.Exec(ctx, `
			UPDATE managed_databases SET status = $3, updated_at = NOW(), last_backup_at = NOW()
			WHERE id = $1::uuid AND organization_id = $2::uuid`,
			databaseID, tenantID, transientStatus)
		if err != nil {
			return ManagedDatabase{}, fmt.Errorf("backup managed database: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return ManagedDatabase{}, fmt.Errorf("managed database not found")
		}
	} else {
		tag, err := tx.Exec(ctx, `
			UPDATE managed_databases SET status = $3, updated_at = NOW()
			WHERE id = $1::uuid AND organization_id = $2::uuid`,
			databaseID, tenantID, transientStatus)
		if err != nil {
			return ManagedDatabase{}, fmt.Errorf("restore managed database: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return ManagedDatabase{}, fmt.Errorf("managed database not found")
		}
	}
	var item ManagedDatabase
	var lastBackup *time.Time
	err = tx.QueryRow(ctx, `
		UPDATE managed_databases SET status = 'ready', updated_at = NOW()
		WHERE id = $1::uuid AND organization_id = $2::uuid
		RETURNING id::text, organization_id::text, project_id::text, engine, name, status,
		          COALESCE(node_id::text, ''), volume_path, backup_schedule, last_backup_at`,
		databaseID, tenantID,
	).Scan(
		&item.ID, &item.OrganizationID, &item.ProjectID, &item.Engine, &item.Name, &item.Status,
		&item.NodeID, &item.VolumePath, &item.BackupSchedule, &lastBackup,
	)
	if err != nil {
		return ManagedDatabase{}, fmt.Errorf("finish managed database operation: %w", err)
	}
	item.LastBackupAt = lastBackup
	return item, tx.Commit(ctx)
}

func randomPassword(length int) string {
	raw := make([]byte, length)
	if _, err := rand.Read(raw); err != nil {
		return fmt.Sprintf("norix-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(raw)[:length]
}
