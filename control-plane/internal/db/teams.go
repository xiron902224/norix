package db

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Team struct {
	ID             string
	OrganizationID string
	Name           string
	Slug           string
	IsDefault      bool
}

type TeamMember struct {
	UserID string
	Email  string
	Name   string
	Role   string
}

func EnsureDefaultTeam(ctx context.Context, tx pgx.Tx, organizationID, ownerUserID string) (string, error) {
	var teamID string
	err := tx.QueryRow(ctx, `
		SELECT id::text FROM teams WHERE organization_id = $1::uuid AND is_default LIMIT 1`,
		organizationID).Scan(&teamID)
	if err == pgx.ErrNoRows {
		if err := tx.QueryRow(ctx, `
			INSERT INTO teams (organization_id, name, slug, is_default)
			VALUES ($1::uuid, 'Default', 'default', TRUE)
			RETURNING id::text`, organizationID).Scan(&teamID); err != nil {
			return "", fmt.Errorf("ensure default team: %w", err)
		}
	} else if err != nil {
		return "", fmt.Errorf("ensure default team: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO team_quotas (organization_id, team_id) VALUES ($1::uuid, $2::uuid)
		ON CONFLICT (team_id) DO NOTHING`, organizationID, teamID); err != nil {
		return "", fmt.Errorf("ensure default team quota: %w", err)
	}
	if ownerUserID != "" {
		var userID string
		if err := tx.QueryRow(ctx, `SELECT id::text FROM users WHERE organization_id = $1::uuid AND id = $2::uuid`,
			organizationID, ownerUserID).Scan(&userID); err == nil {
			_, _ = tx.Exec(ctx, `
				INSERT INTO team_members (organization_id, team_id, user_id, role)
				VALUES ($1::uuid, $2::uuid, $3::uuid, 'owner')
				ON CONFLICT (team_id, user_id) DO NOTHING`,
				organizationID, teamID, userID)
		}
	}
	return teamID, nil
}

func ListTeams(ctx context.Context, pool *pgxpool.Pool, organizationID string) ([]Team, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin list teams: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := WithTenant(ctx, tx, organizationID); err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `
		SELECT id::text, organization_id::text, name, slug, is_default
		FROM teams ORDER BY is_default DESC, name`)
	if err != nil {
		return nil, fmt.Errorf("list teams: %w", err)
	}
	defer rows.Close()
	out := make([]Team, 0)
	for rows.Next() {
		var item Team
		if err := rows.Scan(&item.ID, &item.OrganizationID, &item.Name, &item.Slug, &item.IsDefault); err != nil {
			return nil, fmt.Errorf("scan team: %w", err)
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func CreateTeam(ctx context.Context, pool *pgxpool.Pool, organizationID, name, slug string) (Team, error) {
	name = strings.TrimSpace(name)
	slug = strings.TrimSpace(strings.ToLower(slug))
	if name == "" {
		return Team{}, fmt.Errorf("team name is required")
	}
	if slug == "" {
		slug = strings.ToLower(strings.ReplaceAll(name, " ", "-"))
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return Team{}, fmt.Errorf("begin create team: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := WithTenant(ctx, tx, organizationID); err != nil {
		return Team{}, err
	}
	var team Team
	if err := tx.QueryRow(ctx, `
		INSERT INTO teams (organization_id, name, slug, is_default)
		VALUES ($1::uuid, $2, $3, FALSE)
		RETURNING id::text, organization_id::text, name, slug, is_default`,
		organizationID, name, slug).Scan(&team.ID, &team.OrganizationID, &team.Name, &team.Slug, &team.IsDefault); err != nil {
		return Team{}, fmt.Errorf("create team: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO team_quotas (organization_id, team_id) VALUES ($1::uuid, $2::uuid)`,
		organizationID, team.ID); err != nil {
		return Team{}, fmt.Errorf("create team quota row: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Team{}, fmt.Errorf("commit team: %w", err)
	}
	return team, nil
}

func AddTeamMember(ctx context.Context, pool *pgxpool.Pool, organizationID, teamID, userEmail, role string) (TeamMember, error) {
	userEmail = strings.TrimSpace(strings.ToLower(userEmail))
	role = strings.TrimSpace(role)
	if role == "" {
		role = "member"
	}
	if teamID == "" || userEmail == "" {
		return TeamMember{}, fmt.Errorf("team_id and user_email are required")
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return TeamMember{}, fmt.Errorf("begin add team member: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := WithTenant(ctx, tx, organizationID); err != nil {
		return TeamMember{}, err
	}
	var userID string
	err = tx.QueryRow(ctx, `
		INSERT INTO team_members (organization_id, team_id, user_id, role)
		SELECT $1::uuid, $2::uuid, u.id, $4
		FROM users u
		WHERE u.organization_id = $1::uuid AND u.email_normalized = $3
		ON CONFLICT (team_id, user_id) DO UPDATE SET role = EXCLUDED.role
		RETURNING user_id::text`,
		organizationID, teamID, userEmail, role).Scan(&userID)
	if err != nil {
		return TeamMember{}, fmt.Errorf("add team member: %w", err)
	}
	var member TeamMember
	if err := tx.QueryRow(ctx, `
		SELECT id::text, email, name FROM users WHERE organization_id = $1::uuid AND id = $2::uuid`,
		organizationID, userID).Scan(&member.UserID, &member.Email, &member.Name); err != nil {
		return TeamMember{}, fmt.Errorf("load team member user: %w", err)
	}
	member.Role = role
	if err := tx.Commit(ctx); err != nil {
		return TeamMember{}, fmt.Errorf("commit team member: %w", err)
	}
	return member, nil
}

func DefaultTeamID(ctx context.Context, pool *pgxpool.Pool, organizationID string) (string, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("begin default team: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := WithTenant(ctx, tx, organizationID); err != nil {
		return "", err
	}
	var teamID string
	err = tx.QueryRow(ctx, `
		SELECT id::text FROM teams
		WHERE organization_id = $1::uuid AND is_default
		ORDER BY created_at LIMIT 1`, organizationID).Scan(&teamID)
	if err != nil {
		return "", fmt.Errorf("default team: %w", err)
	}
	return teamID, nil
}

func TeamIDForProject(ctx context.Context, pool *pgxpool.Pool, organizationID, projectID string) (string, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("begin team for project: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := WithTenant(ctx, tx, organizationID); err != nil {
		return "", err
	}
	var teamID *string
	err = tx.QueryRow(ctx, `
		SELECT team_id::text FROM projects WHERE organization_id = $1::uuid AND id = $2::uuid`,
		organizationID, projectID).Scan(&teamID)
	if err != nil {
		return "", fmt.Errorf("team for project: %w", err)
	}
	if teamID != nil && *teamID != "" {
		return *teamID, nil
	}
	return DefaultTeamID(ctx, pool, organizationID)
}
