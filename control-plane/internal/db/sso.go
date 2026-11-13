package db

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

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
