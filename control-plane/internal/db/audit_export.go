package db

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type AuditExportEvent struct {
	ID           int64
	ActorUserID  string
	ActorEmail   string
	Action       string
	ResourceType string
	ResourceID   string
	Metadata     map[string]string
	CreatedAt    time.Time
}

func ListAuditEvents(ctx context.Context, pool *pgxpool.Pool, organizationID string, limit int) ([]AuditExportEvent, error) {
	if limit <= 0 || limit > 100000 {
		limit = 10000
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin audit export: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := WithTenant(ctx, tx, organizationID); err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `
		SELECT id, COALESCE(actor_user_id::text, ''), COALESCE(actor_email, ''),
		       action, resource_type, COALESCE(resource_id, ''), metadata::text, created_at
		FROM audit_events
		WHERE organization_id = $1::uuid
		ORDER BY created_at DESC
		LIMIT $2`, organizationID, limit)
	if err != nil {
		return nil, fmt.Errorf("list audit events: %w", err)
	}
	defer rows.Close()
	out := make([]AuditExportEvent, 0)
	for rows.Next() {
		var item AuditExportEvent
		var metaJSON string
		if err := rows.Scan(&item.ID, &item.ActorUserID, &item.ActorEmail, &item.Action, &item.ResourceType, &item.ResourceID, &metaJSON, &item.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan audit export: %w", err)
		}
		item.Metadata = map[string]string{}
		_ = json.Unmarshal([]byte(metaJSON), &item.Metadata)
		out = append(out, item)
	}
	return out, rows.Err()
}
