package db

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/xiron902224/norix/control-plane/internal/auth"
)

const patPrefix = "norix_pat_"

// UserAPIToken is a personal access token metadata row (never includes plaintext).
type UserAPIToken struct {
	ID             string
	OrganizationID string
	UserID         string
	Name           string
	TokenPrefix    string
	Scopes         []string
	ExpiresAt      *time.Time
	RevokedAt      *time.Time
	LastUsedAt     *time.Time
	CreatedAt      time.Time
}

// NewPersonalAccessToken returns plaintext (shown once) and SHA-256 hash.
func NewPersonalAccessToken() (plainText string, hash []byte, prefix string, err error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", nil, "", fmt.Errorf("generate pat: %w", err)
	}
	secret := base64.RawURLEncoding.EncodeToString(bytes)
	plainText = patPrefix + secret
	hash = auth.HashSessionToken(plainText)
	prefix = plainText
	if len(prefix) > 16 {
		prefix = prefix[:16]
	}
	return plainText, hash, prefix, nil
}

func CreateUserAPIToken(ctx context.Context, pool *pgxpool.Pool, tenantID, userID, name string, scopes []string, expiresAt *time.Time, actor AuditActor) (UserAPIToken, string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return UserAPIToken{}, "", fmt.Errorf("name is required")
	}
	if len(scopes) == 0 {
		scopes = []string{"*"}
	}
	plain, hash, prefix, err := NewPersonalAccessToken()
	if err != nil {
		return UserAPIToken{}, "", err
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return UserAPIToken{}, "", err
	}
	defer tx.Rollback(ctx)
	if err := WithTenant(ctx, tx, tenantID); err != nil {
		return UserAPIToken{}, "", err
	}
	var tok UserAPIToken
	err = tx.QueryRow(ctx, `
		INSERT INTO user_api_tokens (organization_id, user_id, name, token_prefix, token_hash, scopes, expires_at)
		VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6, $7)
		RETURNING id::text, organization_id::text, user_id::text, name, token_prefix, scopes, expires_at, revoked_at, last_used_at, created_at`,
		tenantID, userID, name, prefix, hash, scopes, expiresAt,
	).Scan(
		&tok.ID, &tok.OrganizationID, &tok.UserID, &tok.Name, &tok.TokenPrefix, &tok.Scopes,
		&tok.ExpiresAt, &tok.RevokedAt, &tok.LastUsedAt, &tok.CreatedAt,
	)
	if err != nil {
		return UserAPIToken{}, "", fmt.Errorf("insert pat: %w", err)
	}
	if err := writeAudit(ctx, tx, tenantID, "user.pat_created", "user_api_token", tok.ID, actor, map[string]string{
		"name": name, "prefix": prefix,
	}); err != nil {
		return UserAPIToken{}, "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return UserAPIToken{}, "", err
	}
	return tok, plain, nil
}

func ListUserAPITokens(ctx context.Context, pool *pgxpool.Pool, tenantID, userID string) ([]UserAPIToken, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	if err := WithTenant(ctx, tx, tenantID); err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `
		SELECT id::text, organization_id::text, user_id::text, name, token_prefix, scopes,
		       expires_at, revoked_at, last_used_at, created_at
		FROM user_api_tokens
		WHERE user_id = $1::uuid
		ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]UserAPIToken, 0)
	for rows.Next() {
		var tok UserAPIToken
		if err := rows.Scan(
			&tok.ID, &tok.OrganizationID, &tok.UserID, &tok.Name, &tok.TokenPrefix, &tok.Scopes,
			&tok.ExpiresAt, &tok.RevokedAt, &tok.LastUsedAt, &tok.CreatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, tok)
	}
	return out, tx.Commit(ctx)
}

func RevokeUserAPIToken(ctx context.Context, pool *pgxpool.Pool, tenantID, userID, tokenID string, actor AuditActor) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := WithTenant(ctx, tx, tenantID); err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `
		UPDATE user_api_tokens SET revoked_at = NOW()
		WHERE id = $1::uuid AND user_id = $2::uuid AND revoked_at IS NULL`, tokenID, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("token not found")
	}
	if err := writeAudit(ctx, tx, tenantID, "user.pat_revoked", "user_api_token", tokenID, actor, nil); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// FindIdentityByPATHash authenticates a personal access token (#22).
func FindIdentityByPATHash(ctx context.Context, pool *pgxpool.Pool, tokenHash []byte) (auth.SessionIdentity, error) {
	var identity auth.SessionIdentity
	var expiresAt *time.Time
	var tokenID string
	err := pool.QueryRow(ctx, `
		SELECT t.user_id::text, t.organization_id::text, u.email, u.name, u.role, t.expires_at, t.id::text
		FROM user_api_tokens t
		JOIN users u ON u.id = t.user_id AND u.organization_id = t.organization_id
		WHERE t.token_hash = $1
		  AND t.revoked_at IS NULL`, tokenHash).Scan(
		&identity.UserID, &identity.OrganizationID, &identity.Email, &identity.Name, &identity.Role,
		&expiresAt, &tokenID,
	)
	_ = tokenID
	if err != nil {
		if err == pgx.ErrNoRows {
			return auth.SessionIdentity{}, fmt.Errorf("pat not found")
		}
		return auth.SessionIdentity{}, err
	}
	if expiresAt != nil && expiresAt.Before(time.Now().UTC()) {
		return auth.SessionIdentity{}, fmt.Errorf("pat expired")
	}
	identity.ExpiresAt = time.Now().UTC().Add(24 * time.Hour)
	if expiresAt != nil {
		identity.ExpiresAt = *expiresAt
	}
	_, _ = pool.Exec(ctx, `UPDATE user_api_tokens SET last_used_at = NOW() WHERE token_hash = $1`, tokenHash)
	return identity, nil
}
