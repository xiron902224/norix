package db

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SSOPasswordPlaceholder marks users provisioned via SSO (password login disabled).
const SSOPasswordPlaceholder = "$sso$no-password"

type SSOProvider struct {
	ID             string
	OrganizationID string
	Name           string
	Kind           string
	IssuerURL      string
	ClientID       string
	ClientSecretEnc string
	Enabled        bool
	Metadata       map[string]string
}

func ListSSOProviders(ctx context.Context, pool *pgxpool.Pool, organizationID string) ([]SSOProvider, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin list sso: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := WithTenant(ctx, tx, organizationID); err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `
		SELECT id::text, organization_id::text, name, kind, issuer_url, client_id, enabled, metadata::text
		FROM sso_providers ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("list sso providers: %w", err)
	}
	defer rows.Close()
	out := make([]SSOProvider, 0)
	for rows.Next() {
		var item SSOProvider
		var meta string
		if err := rows.Scan(&item.ID, &item.OrganizationID, &item.Name, &item.Kind, &item.IssuerURL, &item.ClientID, &item.Enabled, &meta); err != nil {
			return nil, fmt.Errorf("scan sso: %w", err)
		}
		item.Metadata = map[string]string{}
		_ = json.Unmarshal([]byte(meta), &item.Metadata)
		out = append(out, item)
	}
	return out, rows.Err()
}

func UpsertSSOProvider(ctx context.Context, pool *pgxpool.Pool, organizationID string, provider SSOProvider, clientSecret string) (SSOProvider, error) {
	name := strings.TrimSpace(provider.Name)
	kind := strings.TrimSpace(strings.ToLower(provider.Kind))
	if name == "" {
		return SSOProvider{}, fmt.Errorf("provider name is required")
	}
	if kind != "oidc" && kind != "saml" {
		return SSOProvider{}, fmt.Errorf("provider kind must be oidc or saml")
	}
	if kind == "saml" && provider.IssuerURL == "" {
		provider.IssuerURL = "stub://saml/metadata"
	}
	meta := []byte(`{}`)
	if len(provider.Metadata) > 0 {
		encoded, err := json.Marshal(provider.Metadata)
		if err != nil {
			return SSOProvider{}, fmt.Errorf("encode sso metadata: %w", err)
		}
		meta = encoded
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return SSOProvider{}, fmt.Errorf("begin upsert sso: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := WithTenant(ctx, tx, organizationID); err != nil {
		return SSOProvider{}, err
	}

	var stored SSOProvider
	query := `
		INSERT INTO sso_providers (organization_id, name, kind, issuer_url, client_id, client_secret_enc, metadata, enabled)
		VALUES ($1::uuid, $2, $3, $4, $5, $6, $7::jsonb, $8)
		ON CONFLICT (organization_id, name) DO UPDATE SET
			kind = EXCLUDED.kind,
			issuer_url = EXCLUDED.issuer_url,
			client_id = EXCLUDED.client_id,
			client_secret_enc = CASE WHEN $6 <> '' THEN EXCLUDED.client_secret_enc ELSE sso_providers.client_secret_enc END,
			metadata = EXCLUDED.metadata,
			enabled = EXCLUDED.enabled,
			updated_at = NOW()
		RETURNING id::text, organization_id::text, name, kind, issuer_url, client_id, enabled, metadata::text`
	secretEnc := clientSecret
	if secretEnc == "" {
		secretEnc = " "
	}
	err = tx.QueryRow(ctx, query,
		organizationID, name, kind, provider.IssuerURL, provider.ClientID, secretEnc, meta, provider.Enabled,
	).Scan(&stored.ID, &stored.OrganizationID, &stored.Name, &stored.Kind, &stored.IssuerURL, &stored.ClientID, &stored.Enabled, &meta)
	if err != nil {
		return SSOProvider{}, fmt.Errorf("upsert sso provider: %w", err)
	}
	stored.Metadata = map[string]string{}
	_ = json.Unmarshal(meta, &stored.Metadata)
	if err := tx.Commit(ctx); err != nil {
		return SSOProvider{}, fmt.Errorf("commit sso: %w", err)
	}
	return stored, nil
}

// GetSSOProviderByID loads a provider (including client secret) by id.
// Intended for bootstrap/migration pool (bypasses RLS) during OIDC login.
func GetSSOProviderByID(ctx context.Context, pool *pgxpool.Pool, providerID string) (SSOProvider, error) {
	var item SSOProvider
	var meta string
	err := pool.QueryRow(ctx, `
		SELECT id::text, organization_id::text, name, kind, issuer_url, client_id,
		       client_secret_enc, enabled, metadata::text
		FROM sso_providers WHERE id = $1::uuid`, providerID).Scan(
		&item.ID, &item.OrganizationID, &item.Name, &item.Kind, &item.IssuerURL,
		&item.ClientID, &item.ClientSecretEnc, &item.Enabled, &meta,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return SSOProvider{}, fmt.Errorf("sso provider not found")
		}
		return SSOProvider{}, fmt.Errorf("get sso provider: %w", err)
	}
	item.Metadata = map[string]string{}
	_ = json.Unmarshal([]byte(meta), &item.Metadata)
	item.ClientSecretEnc = strings.TrimSpace(item.ClientSecretEnc)
	return item, nil
}

// ListEnabledOIDCProviders returns public fields for login UI (no secrets).
func ListEnabledOIDCProviders(ctx context.Context, pool *pgxpool.Pool) ([]SSOProvider, error) {
	rows, err := pool.Query(ctx, `
		SELECT id::text, organization_id::text, name, kind, issuer_url, client_id, enabled
		FROM sso_providers
		WHERE enabled = TRUE AND kind = 'oidc'
		ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("list enabled oidc providers: %w", err)
	}
	defer rows.Close()
	out := make([]SSOProvider, 0)
	for rows.Next() {
		var item SSOProvider
		if err := rows.Scan(&item.ID, &item.OrganizationID, &item.Name, &item.Kind, &item.IssuerURL, &item.ClientID, &item.Enabled); err != nil {
			return nil, fmt.Errorf("scan oidc provider: %w", err)
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

// FindOrCreateSSOUser finds a user in the organization by email or JIT-provisions a member.
func FindOrCreateSSOUser(ctx context.Context, pool *pgxpool.Pool, organizationID, email, name string) (AuthUser, error) {
	email = strings.TrimSpace(strings.ToLower(email))
	name = strings.TrimSpace(name)
	if organizationID == "" || email == "" {
		return AuthUser{}, fmt.Errorf("organization and email are required")
	}
	if name == "" {
		name = email
	}
	user, err := FindUser(ctx, pool, organizationID, email)
	if err == nil {
		return user, nil
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return AuthUser{}, fmt.Errorf("begin sso user: %w", err)
	}
	defer tx.Rollback(ctx)
	var userID string
	err = tx.QueryRow(ctx, `
		INSERT INTO users (organization_id, email, email_normalized, name, role, password_hash)
		VALUES ($1::uuid, $2, $2, $3, 'member', $4)
		ON CONFLICT (organization_id, email_normalized) DO UPDATE SET
			name = CASE WHEN EXCLUDED.name <> '' THEN EXCLUDED.name ELSE users.name END,
			updated_at = NOW()
		RETURNING id::text`,
		organizationID, email, name, SSOPasswordPlaceholder).Scan(&userID)
	if err != nil {
		return AuthUser{}, fmt.Errorf("create sso user: %w", err)
	}
	if _, err := EnsureDefaultTeam(ctx, tx, organizationID, userID); err != nil {
		return AuthUser{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return AuthUser{}, fmt.Errorf("commit sso user: %w", err)
	}
	return FindUser(ctx, pool, organizationID, email)
}
