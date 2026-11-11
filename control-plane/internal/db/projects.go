package db

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Project struct {
	ID             string
	OrganizationID string
	Name           string
}

func CreateProject(ctx context.Context, pool *pgxpool.Pool, organizationID, name string, actor AuditActor) (Project, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Project{}, fmt.Errorf("project name is required")
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return Project{}, fmt.Errorf("begin project transaction: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := WithTenant(ctx, tx, organizationID); err != nil {
		return Project{}, err
	}

	var project Project
	if err := tx.QueryRow(ctx, `
		INSERT INTO projects (organization_id, name)
		VALUES ($1::uuid, $2)
		RETURNING id::text, organization_id::text, name`, organizationID, name).Scan(&project.ID, &project.OrganizationID, &project.Name); err != nil {
		return Project{}, fmt.Errorf("create project: %w", err)
	}
	if err := writeAudit(ctx, tx, organizationID, "project.created", "project", project.ID, actor, map[string]string{"name": project.Name}); err != nil {
		return Project{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Project{}, fmt.Errorf("commit project: %w", err)
	}
	return project, nil
}

func ListProjects(ctx context.Context, pool *pgxpool.Pool, organizationID string) ([]Project, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin projects transaction: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := WithTenant(ctx, tx, organizationID); err != nil {
		return nil, err
	}

	rows, err := tx.Query(ctx, `SELECT id::text, organization_id::text, name FROM projects ORDER BY created_at, name`)
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}
	defer rows.Close()
	projects := make([]Project, 0)
	for rows.Next() {
		var project Project
		if err := rows.Scan(&project.ID, &project.OrganizationID, &project.Name); err != nil {
			return nil, fmt.Errorf("scan project: %w", err)
		}
		projects = append(projects, project)
	}
	return projects, rows.Err()
}

func DeleteProject(ctx context.Context, pool *pgxpool.Pool, organizationID, projectID string, actor AuditActor) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin delete project transaction: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := WithTenant(ctx, tx, organizationID); err != nil {
		return err
	}
	command, err := tx.Exec(ctx, `DELETE FROM projects WHERE id = $1::uuid`, projectID)
	if err != nil {
		return fmt.Errorf("delete project: %w", err)
	}
	if command.RowsAffected() == 0 {
		return fmt.Errorf("project not found")
	}
	if err := writeAudit(ctx, tx, organizationID, "project.deleted", "project", projectID, actor, nil); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
