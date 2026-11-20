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
	TTLSeconds       int32
	ExpiresAt        time.Time
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
		       preview_url, github_comment_id, ttl_seconds, expires_at, created_at
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
			&item.PreviewURL, &item.GithubCommentID, &item.TTLSeconds, &item.ExpiresAt, &item.CreatedAt,
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
	if preview.TTLSeconds <= 0 {
		preview.TTLSeconds = 86400
	}
	if preview.TTLSeconds < 60 {
		preview.TTLSeconds = 60
	}
	if preview.TTLSeconds > 604800 {
		preview.TTLSeconds = 604800
	}
	expiresAt := time.Now().UTC().Add(time.Duration(preview.TTLSeconds) * time.Second)
	tx, err := pool.Begin(ctx)
	if err != nil {
		return PRPreview{}, err
	}
	defer tx.Rollback(ctx)
	if err := WithTenant(ctx, tx, tenantID); err != nil {
		return PRPreview{}, err
	}
	err = tx.QueryRow(ctx, `
		INSERT INTO pr_previews (organization_id, project_id, repo, pr_number, status, preview_url, github_comment_id, ttl_seconds, expires_at)
		VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (organization_id, repo, pr_number) DO UPDATE SET
			status = EXCLUDED.status,
			preview_url = EXCLUDED.preview_url,
			github_comment_id = EXCLUDED.github_comment_id,
			ttl_seconds = EXCLUDED.ttl_seconds,
			expires_at = CASE
				WHEN pr_previews.status IN ('destroyed', 'failed') THEN EXCLUDED.expires_at
				WHEN EXCLUDED.status IN ('pending', 'building') THEN EXCLUDED.expires_at
				ELSE pr_previews.expires_at
			END,
			updated_at = NOW()
		RETURNING id::text, organization_id::text, project_id::text, repo, pr_number, status,
		          preview_url, github_comment_id, ttl_seconds, expires_at, created_at`,
		tenantID, preview.ProjectID, preview.Repo, preview.PRNumber, preview.Status, preview.PreviewURL, preview.GithubCommentID,
		preview.TTLSeconds, expiresAt,
	).Scan(
		&preview.ID, &preview.OrganizationID, &preview.ProjectID, &preview.Repo, &preview.PRNumber, &preview.Status,
		&preview.PreviewURL, &preview.GithubCommentID, &preview.TTLSeconds, &preview.ExpiresAt, &preview.CreatedAt,
	)
	if err != nil {
		return PRPreview{}, fmt.Errorf("upsert preview: %w", err)
	}
	return preview, tx.Commit(ctx)
}

// GCExpiredPRPreviews marks ready/building/pending previews past expires_at as destroyed
// and deletes the matching pr-N workloads (#21).
func GCExpiredPRPreviews(ctx context.Context, pool *pgxpool.Pool, tenantID string) (int64, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	if err := WithTenant(ctx, tx, tenantID); err != nil {
		return 0, err
	}
	rows, err := tx.Query(ctx, `
		UPDATE pr_previews
		SET status = 'destroyed', updated_at = NOW()
		WHERE organization_id = $1::uuid
		  AND expires_at < NOW()
		  AND status IN ('pending', 'building', 'ready')
		RETURNING project_id::text, pr_number`,
		tenantID,
	)
	if err != nil {
		return 0, fmt.Errorf("gc previews: %w", err)
	}
	type expired struct {
		projectID string
		pr        int32
	}
	var items []expired
	for rows.Next() {
		var item expired
		if err := rows.Scan(&item.projectID, &item.pr); err != nil {
			rows.Close()
			return 0, err
		}
		items = append(items, item)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	for _, item := range items {
		name := PreviewWorkloadName(item.pr)
		_, _ = tx.Exec(ctx, `
			UPDATE workloads
			SET status = 'deleting', desired_action = 'stop', updated_at = NOW()
			WHERE project_id = $1::uuid AND name = $2 AND status <> 'deleting'`,
			item.projectID, name)
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return int64(len(items)), nil
}
