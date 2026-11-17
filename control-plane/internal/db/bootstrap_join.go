package db

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/xiron902224/norix/control-plane/internal/auth"
)

// BootstrapJoinTokenEnv holds a one-shot join token plaintext for Docker install
// (agent enrolls after first admin / on CP restart). Same value as NORIX_JOIN_TOKEN.
const BootstrapJoinTokenEnv = "NORIX_BOOTSTRAP_JOIN_TOKEN"

// SeedBootstrapJoinToken inserts NORIX_BOOTSTRAP_JOIN_TOKEN for the first org
// if that hash is not already present. Idempotent. TTL 7 days so the Compose
// agent can enroll after first-admin bootstrap (#10).
func SeedBootstrapJoinToken(ctx context.Context, pool *pgxpool.Pool) (int, error) {
	plain := strings.TrimSpace(os.Getenv(BootstrapJoinTokenEnv))
	if plain == "" {
		return 0, nil
	}
	hash := auth.HashSessionToken(plain)
	orgs, err := ListOrganizationIDs(ctx, pool)
	if err != nil {
		return 0, fmt.Errorf("list orgs for bootstrap join token: %w", err)
	}
	if len(orgs) == 0 {
		return 0, nil
	}
	expiresAt := time.Now().UTC().Add(7 * 24 * time.Hour)
	// token_hash is globally UNIQUE — seed at most one org (first install).
	return ensureBootstrapJoinToken(ctx, pool, orgs[0], hash, expiresAt)
}

func ensureBootstrapJoinToken(ctx context.Context, pool *pgxpool.Pool, organizationID string, tokenHash []byte, expiresAt time.Time) (int, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin bootstrap join token: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := WithTenant(ctx, tx, organizationID); err != nil {
		return 0, err
	}

	var existing string
	err = tx.QueryRow(ctx, `
		SELECT id::text FROM join_tokens
		WHERE token_hash = $1
		LIMIT 1`, tokenHash).Scan(&existing)
	if err == nil {
		// Already seeded or consumed — do not violate UNIQUE(token_hash).
		return 0, nil
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return 0, fmt.Errorf("lookup bootstrap join token: %w", err)
	}

	var tokenID string
	if err := tx.QueryRow(ctx, `
		INSERT INTO join_tokens (organization_id, token_hash, node_name, expires_at, created_by_user_id)
		VALUES ($1::uuid, $2, 'norix-1', $3, NULL)
		RETURNING id::text`, organizationID, tokenHash, expiresAt).Scan(&tokenID); err != nil {
		return 0, fmt.Errorf("insert bootstrap join token: %w", err)
	}
	if err := writeAudit(ctx, tx, organizationID, "node.join_token_bootstrap", "join_token", tokenID, AuditActor{}, map[string]string{
		"source": BootstrapJoinTokenEnv,
	}); err != nil {
		return 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit bootstrap join token: %w", err)
	}
	return 1, nil
}
