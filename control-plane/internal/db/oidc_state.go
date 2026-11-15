package db

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type OIDCLoginState struct {
	State          string
	Nonce          string
	ProviderID     string
	OrganizationID string
	ExpiresAt      time.Time
}

func StoreOIDCLoginState(ctx context.Context, pool *pgxpool.Pool, state OIDCLoginState) error {
	if state.State == "" || state.Nonce == "" || state.ProviderID == "" || state.OrganizationID == "" {
		return fmt.Errorf("oidc login state fields are required")
	}
	if state.ExpiresAt.IsZero() {
		state.ExpiresAt = time.Now().UTC().Add(10 * time.Minute)
	}
	_, err := pool.Exec(ctx, `
		INSERT INTO oidc_login_states (state, nonce, provider_id, organization_id, expires_at)
		VALUES ($1, $2, $3::uuid, $4::uuid, $5)`,
		state.State, state.Nonce, state.ProviderID, state.OrganizationID, state.ExpiresAt)
	if err != nil {
		return fmt.Errorf("store oidc login state: %w", err)
	}
	return nil
}

func ConsumeOIDCLoginState(ctx context.Context, pool *pgxpool.Pool, stateToken string) (OIDCLoginState, error) {
	var item OIDCLoginState
	err := pool.QueryRow(ctx, `
		DELETE FROM oidc_login_states
		WHERE state = $1 AND expires_at > NOW()
		RETURNING state, nonce, provider_id::text, organization_id::text, expires_at`,
		stateToken).Scan(&item.State, &item.Nonce, &item.ProviderID, &item.OrganizationID, &item.ExpiresAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return OIDCLoginState{}, fmt.Errorf("oidc state is invalid or expired")
		}
		return OIDCLoginState{}, fmt.Errorf("consume oidc state: %w", err)
	}
	return item, nil
}

func CleanupExpiredOIDCLoginStates(ctx context.Context, pool *pgxpool.Pool) (int64, error) {
	command, err := pool.Exec(ctx, `DELETE FROM oidc_login_states WHERE expires_at <= NOW()`)
	if err != nil {
		return 0, fmt.Errorf("cleanup oidc states: %w", err)
	}
	return command.RowsAffected(), nil
}
