package db

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
)

type AuditActor struct {
	UserID string
	Email  string
}

func writeAudit(ctx context.Context, tx pgx.Tx, organizationID, action, resourceType, resourceID string, actor AuditActor, metadata map[string]string) error {
	payload := []byte(`{}`)
	if len(metadata) > 0 {
		encoded, err := json.Marshal(metadata)
		if err != nil {
			return fmt.Errorf("encode audit metadata: %w", err)
		}
		payload = encoded
	}
	var actorID any
	if actor.UserID != "" {
		actorID = actor.UserID
	}
	var actorEmail any
	if actor.Email != "" {
		actorEmail = actor.Email
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO audit_events (organization_id, actor_user_id, actor_email, action, resource_type, resource_id, metadata)
		VALUES ($1::uuid, $2, $3, $4, $5, $6, $7::jsonb)`,
		organizationID, actorID, actorEmail, action, resourceType, resourceID, payload,
	); err != nil {
		return fmt.Errorf("write audit event: %w", err)
	}
	return nil
}
