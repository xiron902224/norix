package ha

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// AcquireLease tries to become leader for name until TTL (S17).
func AcquireLease(ctx context.Context, pool *pgxpool.Pool, name, holderID string, ttl time.Duration) (bool, error) {
	if name == "" || holderID == "" {
		return false, fmt.Errorf("name and holder_id required")
	}
	if ttl <= 0 {
		ttl = 15 * time.Second
	}
	expires := time.Now().UTC().Add(ttl)
	tag, err := pool.Exec(ctx, `
		INSERT INTO control_plane_leases (name, holder_id, expires_at, updated_at)
		VALUES ($1, $2, $3, NOW())
		ON CONFLICT (name) DO UPDATE SET
			holder_id = EXCLUDED.holder_id,
			expires_at = EXCLUDED.expires_at,
			updated_at = NOW()
		WHERE control_plane_leases.expires_at < NOW()
		   OR control_plane_leases.holder_id = EXCLUDED.holder_id`,
		name, holderID, expires,
	)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// RenewLease extends ownership if still holder.
func RenewLease(ctx context.Context, pool *pgxpool.Pool, name, holderID string, ttl time.Duration) (bool, error) {
	if ttl <= 0 {
		ttl = 15 * time.Second
	}
	expires := time.Now().UTC().Add(ttl)
	tag, err := pool.Exec(ctx, `
		UPDATE control_plane_leases
		SET expires_at = $3, updated_at = NOW()
		WHERE name = $1 AND holder_id = $2 AND expires_at >= NOW()`,
		name, holderID, expires,
	)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// DefaultLeaseTTL is the recommended leader heartbeat period.
func DefaultLeaseTTL() time.Duration {
	return 15 * time.Second
}
