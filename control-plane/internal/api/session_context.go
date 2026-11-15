package api

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/xiron902224/norix/control-plane/internal/auth"
)

func contextWithSession(ctx context.Context, identity auth.SessionIdentity) context.Context {
	return context.WithValue(ctx, sessionContextKey{}, identity)
}

func SessionFromContext(ctx context.Context) (auth.SessionIdentity, bool) {
	identity, ok := ctx.Value(sessionContextKey{}).(auth.SessionIdentity)
	return identity, ok
}

type sessionContextKey struct{}

func findActiveSession(ctx context.Context, pool *pgxpool.Pool, tokenHash []byte) (auth.SessionIdentity, error) {
	var identity auth.SessionIdentity
	err := pool.QueryRow(ctx, `
		SELECT s.user_id::text, s.organization_id::text, u.email, u.name, u.role, s.expires_at
		FROM sessions s
		JOIN users u ON u.id = s.user_id AND u.organization_id = s.organization_id
		WHERE s.token_hash = $1
		  AND s.revoked_at IS NULL
		  AND s.expires_at > NOW()`, tokenHash).Scan(
		&identity.UserID,
		&identity.OrganizationID,
		&identity.Email,
		&identity.Name,
		&identity.Role,
		&identity.ExpiresAt,
	)
	if err != nil {
		return auth.SessionIdentity{}, fmt.Errorf("lookup active session: %w", err)
	}
	if identity.ExpiresAt.Before(time.Now().UTC()) {
		return auth.SessionIdentity{}, fmt.Errorf("session expired")
	}
	return identity, nil
}
