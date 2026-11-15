package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

func WithTenant(ctx context.Context, tx pgx.Tx, organizationID string) error {
	if _, err := tx.Exec(ctx, "SELECT set_config('app.current_tenant_id', $1, true)", organizationID); err != nil {
		return fmt.Errorf("set tenant: %w", err)
	}
	return nil
}
