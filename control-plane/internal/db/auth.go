package db

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func OwnerExists(ctx context.Context, pool *pgxpool.Pool) (bool, error) {
	var exists bool
	if err := pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM users WHERE role = 'owner')`).Scan(&exists); err != nil {
		return false, fmt.Errorf("check owner: %w", err)
	}
	return exists, nil
}

func CreateInitialOwner(ctx context.Context, pool *pgxpool.Pool, organizationName, adminName, adminEmail, passwordHash string) (AuthUser, error) {
	adminEmail = strings.TrimSpace(strings.ToLower(adminEmail))
	organizationName = strings.TrimSpace(organizationName)
	adminName = strings.TrimSpace(adminName)
	if organizationName == "" || adminName == "" || adminEmail == "" || passwordHash == "" {
		return AuthUser{}, fmt.Errorf("organization name, administrator name, email and password hash are required")
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return AuthUser{}, fmt.Errorf("begin initial owner transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext('norix_initial_owner'))`); err != nil {
		return AuthUser{}, fmt.Errorf("lock initial owner creation: %w", err)
	}
	var ownerExists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM users WHERE role = 'owner')`).Scan(&ownerExists); err != nil {
		return AuthUser{}, fmt.Errorf("check existing owner under lock: %w", err)
	}
	if ownerExists {
		return AuthUser{}, fmt.Errorf("initial administrator has already been created")
	}

	var organizationID string
	if err := tx.QueryRow(ctx, `
		INSERT INTO organizations (name, slug)
		VALUES ($1, $2)
		RETURNING id::text`, organizationName, strings.ToLower(strings.ReplaceAll(organizationName, " ", "-"))).Scan(&organizationID); err != nil {
		return AuthUser{}, fmt.Errorf("create organization: %w", err)
	}

	var userID string
	if err := tx.QueryRow(ctx, `
		INSERT INTO users (organization_id, email, email_normalized, name, role, password_hash)
		VALUES ($1::uuid, $2, $2, $3, 'owner', $4)
		RETURNING id::text`, organizationID, adminEmail, adminName, passwordHash).Scan(&userID); err != nil {
		return AuthUser{}, fmt.Errorf("create initial owner: %w", err)
	}
	if _, err := EnsureDefaultTeam(ctx, tx, organizationID, userID); err != nil {
		return AuthUser{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return AuthUser{}, fmt.Errorf("commit initial owner: %w", err)
	}
	return AuthUser{ID: userID, OrganizationID: organizationID, Email: adminEmail, Name: adminName, Role: "owner", PasswordHash: passwordHash}, nil
}

type AuthUser struct {
	ID             string
	OrganizationID string
	Email          string
	Name           string
	Role           string
	PasswordHash   string
}

func FindUserByEmail(ctx context.Context, pool *pgxpool.Pool, email string) (AuthUser, error) {
	var user AuthUser
	err := pool.QueryRow(ctx, `
		SELECT id::text, organization_id::text, email, name, role, password_hash
		FROM users
		WHERE email_normalized = $1
		LIMIT 2`, strings.TrimSpace(strings.ToLower(email))).Scan(
		&user.ID,
		&user.OrganizationID,
		&user.Email,
		&user.Name,
		&user.Role,
		&user.PasswordHash,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return AuthUser{}, fmt.Errorf("invalid credentials")
		}
		return AuthUser{}, fmt.Errorf("find user by email: %w", err)
	}
	return user, nil
}

func CreateSession(ctx context.Context, pool *pgxpool.Pool, organizationID, userID string, tokenHash []byte, expiresAt time.Time) error {
	if _, err := pool.Exec(ctx, `
		INSERT INTO sessions (organization_id, user_id, token_hash, expires_at)
		VALUES ($1::uuid, $2::uuid, $3, $4)`, organizationID, userID, tokenHash, expiresAt); err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	return nil
}

func RenewSession(ctx context.Context, pool *pgxpool.Pool, tokenHash []byte, expiresAt time.Time) (time.Time, error) {
	var renewedAt time.Time
	err := pool.QueryRow(ctx, `
		UPDATE sessions
		SET expires_at = $2
		WHERE token_hash = $1
		  AND revoked_at IS NULL
		  AND expires_at > NOW()
		RETURNING expires_at`, tokenHash, expiresAt).Scan(&renewedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return time.Time{}, fmt.Errorf("session inactive")
		}
		return time.Time{}, fmt.Errorf("renew session: %w", err)
	}
	return renewedAt, nil
}

func CleanupExpiredSessions(ctx context.Context, pool *pgxpool.Pool) (int64, error) {
	command, err := pool.Exec(ctx, `DELETE FROM sessions WHERE expires_at <= NOW() OR revoked_at IS NOT NULL AND revoked_at < NOW() - INTERVAL '30 days'`)
	if err != nil {
		return 0, fmt.Errorf("cleanup sessions: %w", err)
	}
	return command.RowsAffected(), nil
}

func RevokeSession(ctx context.Context, pool *pgxpool.Pool, tokenHash []byte) error {
	if _, err := pool.Exec(ctx, `
		UPDATE sessions
		SET revoked_at = NOW()
		WHERE token_hash = $1 AND revoked_at IS NULL`, tokenHash); err != nil {
		return fmt.Errorf("revoke session: %w", err)
	}
	return nil
}

func FindUser(ctx context.Context, pool *pgxpool.Pool, organizationID, email string) (AuthUser, error) {
	var user AuthUser
	err := pool.QueryRow(ctx, `
		SELECT id::text, organization_id::text, email, name, role, password_hash
		FROM users
		WHERE organization_id = $1::uuid AND email_normalized = $2`, organizationID, strings.TrimSpace(strings.ToLower(email))).Scan(
		&user.ID,
		&user.OrganizationID,
		&user.Email,
		&user.Name,
		&user.Role,
		&user.PasswordHash,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return AuthUser{}, fmt.Errorf("invalid credentials")
		}
		return AuthUser{}, fmt.Errorf("find user: %w", err)
	}
	return user, nil
}
