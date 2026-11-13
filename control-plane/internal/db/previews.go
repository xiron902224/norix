package db

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type PRPreview struct {
	ID               string
	OrganizationID   string
	ProjectID        string
	Repo             string
	PRNumber         int32
	Status           string
	PreviewURL       string
	GithubCommentID  int64
	CreatedAt        time.Time
}

func ListPRPreviews(ctx context.Context, pool *pgxpool.Pool, tenantID, projectID string) ([]PRPreview, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	if err := WithTenant(ctx, tx, tenantID); err != nil {
		return nil, err
	}
	query := `
		SELECT id::text, organization_id::text, project_id::text, repo, pr_number, status,
		       preview_url, github_comment_id, created_at
		FROM pr_previews WHERE organization_id = $1::uuid`
	args := []any{tenantID}
	if projectID != "" {
		query += ` AND project_id = $2::uuid`
		args = append(args, projectID)
	}
	query += ` ORDER BY created_at DESC`
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]PRPreview, 0)
	for rows.Next() {
		var item PRPreview
		if err := rows.Scan(
			&item.ID, &item.OrganizationID, &item.ProjectID, &item.Repo, &item.PRNumber, &item.Status,
			&item.PreviewURL, &item.GithubCommentID, &item.CreatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, tx.Commit(ctx)
}

func UpsertPRPreview(ctx context.Context, pool *pgxpool.Pool, tenantID string, preview PRPreview) (PRPreview, error) {
	preview.Repo = strings.TrimSpace(preview.Repo)
	if preview.ProjectID == "" || preview.Repo == "" || preview.PRNumber <= 0 {
		return PRPreview{}, fmt.Errorf("project, repo and pr_number are required")
	}
	if preview.Status == "" {
		preview.Status = "pending"
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return PRPreview{}, err
	}
	defer tx.Rollback(ctx)
	if err := WithTenant(ctx, tx, tenantID); err != nil {
		return PRPreview{}, err
	}
	err = tx.QueryRow(ctx, `
		INSERT INTO pr_previews (organization_id, project_id, repo, pr_number, status, preview_url, github_comment_id)
		VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6, $7)
		ON CONFLICT (organization_id, repo, pr_number) DO UPDATE SET
			status = EXCLUDED.status,
			preview_url = EXCLUDED.preview_url,
			github_comment_id = EXCLUDED.github_comment_id,
			updated_at = NOW()
		RETURNING id::text, organization_id::text, project_id::text, repo, pr_number, status,
		          preview_url, github_comment_id, created_at`,
		tenantID, preview.ProjectID, preview.Repo, preview.PRNumber, preview.Status, preview.PreviewURL, preview.GithubCommentID,
	).Scan(
		&preview.ID, &preview.OrganizationID, &preview.ProjectID, &preview.Repo, &preview.PRNumber, &preview.Status,
		&preview.PreviewURL, &preview.GithubCommentID, &preview.CreatedAt,
	)
	if err != nil {
		return PRPreview{}, fmt.Errorf("upsert preview: %w", err)
	}
	return preview, tx.Commit(ctx)
}
