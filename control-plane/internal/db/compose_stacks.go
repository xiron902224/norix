package db

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrComposeStackNotFound is returned when a stack id is unknown in the tenant.
var ErrComposeStackNotFound = errors.New("compose stack not found")

const maxStackLogLinesPerReport = 200

// ComposeStackService is one service as last seen by `docker compose ps`.
type ComposeStackService struct {
	Name     string `json:"name"`
	State    string `json:"state"`
	Health   string `json:"health,omitempty"`
	ExitCode int32  `json:"exit_code"`
	Image    string `json:"image,omitempty"`
	Ports    string `json:"ports,omitempty"`
}

// ComposeStack is a compose file applied as a unit on one node (#143).
type ComposeStack struct {
	ID                 string
	OrganizationID     string
	ProjectID          string
	Name               string
	GitURL             string
	GitRef             string
	ComposePath        string
	AssignedNodeID     string
	DesiredAction      string
	DesiredGeneration  int64
	ObservedGeneration int64
	Status             string
	Message            string
	Services           []ComposeStackService
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// ComposeProject is the `docker compose -p` name; stable and unique per stack.
func (s ComposeStack) ComposeProject() string {
	return ComposeProjectName(s.ID)
}

// ComposeProjectName derives the compose project from a stack id.
func ComposeProjectName(stackID string) string {
	hex := strings.ReplaceAll(stackID, "-", "")
	if len(hex) > 12 {
		hex = hex[:12]
	}
	return "norix-" + strings.ToLower(hex)
}

// ComposeStackLog is one stored log line of a stack service.
type ComposeStackLog struct {
	Service   string
	Line      string
	CreatedAt time.Time
}

// CreateComposeStackInput is what the API accepts to create a stack.
type CreateComposeStackInput struct {
	ProjectID   string
	Name        string
	GitURL      string
	GitRef      string
	ComposePath string
}

// ObservedComposeStack is an agent report for one stack.
type ObservedComposeStack struct {
	ID                 string
	Status             string
	ObservedGeneration int64
	Message            string
	Services           []ComposeStackService
	Logs               []ComposeStackLog
}

const composeStackColumns = `
	id::text, organization_id::text, project_id::text, name, git_url, git_ref, compose_path,
	COALESCE(assigned_node_id::text, ''), desired_action, desired_generation, observed_generation,
	status, message, services::text, created_at, updated_at`

func scanComposeStack(row pgx.Row) (ComposeStack, error) {
	var item ComposeStack
	var services string
	err := row.Scan(
		&item.ID, &item.OrganizationID, &item.ProjectID, &item.Name, &item.GitURL, &item.GitRef, &item.ComposePath,
		&item.AssignedNodeID, &item.DesiredAction, &item.DesiredGeneration, &item.ObservedGeneration,
		&item.Status, &item.Message, &services, &item.CreatedAt, &item.UpdatedAt,
	)
	if err != nil {
		return ComposeStack{}, err
	}
	if services != "" {
		_ = json.Unmarshal([]byte(services), &item.Services)
	}
	if item.Services == nil {
		item.Services = []ComposeStackService{}
	}
	return item, nil
}

func withTenantTx(ctx context.Context, pool *pgxpool.Pool, tenantID string, fn func(pgx.Tx) error) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := WithTenant(ctx, tx, tenantID); err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// CreateComposeStack stores a stack; a Docker-capable node claims it on its next heartbeat.
func CreateComposeStack(ctx context.Context, pool *pgxpool.Pool, tenantID string, in CreateComposeStackInput, actor AuditActor) (ComposeStack, error) {
	var out ComposeStack
	err := withTenantTx(ctx, pool, tenantID, func(tx pgx.Tx) error {
		// The FK alone would accept another tenant's project id, so select it under RLS.
		row := tx.QueryRow(ctx, `
			INSERT INTO compose_stacks (organization_id, project_id, name, git_url, git_ref, compose_path)
			SELECT $1::uuid, p.id, $3, $4, $5, $6
			FROM projects p
			WHERE p.id = $2::uuid AND p.organization_id = $1::uuid
			RETURNING `+composeStackColumns,
			tenantID, in.ProjectID, in.Name, in.GitURL, in.GitRef, in.ComposePath,
		)
		item, err := scanComposeStack(row)
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("project not found")
		}
		if err != nil {
			if strings.Contains(err.Error(), "compose_stacks_project_id_name_key") {
				return fmt.Errorf("a stack named %q already exists in this project", in.Name)
			}
			return err
		}
		out = item
		return writeAudit(ctx, tx, tenantID, "compose_stack.create", "compose_stack", item.ID, actor, map[string]string{
			"name": item.Name, "git_url": item.GitURL, "git_ref": item.GitRef, "compose_path": item.ComposePath,
		})
	})
	return out, err
}

// ListComposeStacks returns the stacks of a project (all projects when projectID is empty).
func ListComposeStacks(ctx context.Context, pool *pgxpool.Pool, tenantID, projectID string) ([]ComposeStack, error) {
	out := []ComposeStack{}
	err := withTenantTx(ctx, pool, tenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT `+composeStackColumns+`
			FROM compose_stacks
			WHERE organization_id = $1::uuid AND ($2 = '' OR project_id::text = $2)
			ORDER BY created_at`, tenantID, projectID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			item, err := scanComposeStack(rows)
			if err != nil {
				return err
			}
			out = append(out, item)
		}
		return rows.Err()
	})
	return out, err
}

// GetComposeStack loads one stack.
func GetComposeStack(ctx context.Context, pool *pgxpool.Pool, tenantID, stackID string) (ComposeStack, error) {
	var out ComposeStack
	err := withTenantTx(ctx, pool, tenantID, func(tx pgx.Tx) error {
		item, err := scanComposeStack(tx.QueryRow(ctx, `
			SELECT `+composeStackColumns+`
			FROM compose_stacks WHERE id = $1::uuid AND organization_id = $2::uuid`, stackID, tenantID))
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrComposeStackNotFound
		}
		out = item
		return err
	})
	return out, err
}

// ListComposeStackLogs returns the newest lines, oldest first.
func ListComposeStackLogs(ctx context.Context, pool *pgxpool.Pool, tenantID, stackID, service string, tail int) ([]ComposeStackLog, error) {
	if tail <= 0 || tail > 2000 {
		tail = 300
	}
	out := []ComposeStackLog{}
	err := withTenantTx(ctx, pool, tenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT service, line, created_at FROM (
				SELECT id, service, line, created_at
				FROM compose_stack_logs
				WHERE stack_id = $1::uuid AND ($2 = '' OR service = $2)
				ORDER BY id DESC
				LIMIT $3
			) newest ORDER BY id`, stackID, service, tail)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var item ComposeStackLog
			if err := rows.Scan(&item.Service, &item.Line, &item.CreatedAt); err != nil {
				return err
			}
			out = append(out, item)
		}
		return rows.Err()
	})
	return out, err
}

// ComposeStackActionDesired maps a user action to the desired agent action.
func ComposeStackActionDesired(action string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "start", "redeploy":
		return "up", nil
	case "stop":
		return "stop", nil
	case "delete":
		return "down", nil
	default:
		return "", fmt.Errorf("action must be start, stop, redeploy or delete")
	}
}

// SetComposeStackAction bumps the generation so the agent re-applies the stack.
// Deleting a stack that never reached a node removes the row directly.
func SetComposeStackAction(ctx context.Context, pool *pgxpool.Pool, tenantID, stackID, action string, actor AuditActor) (ComposeStack, error) {
	desired, err := ComposeStackActionDesired(action)
	if err != nil {
		return ComposeStack{}, err
	}
	var out ComposeStack
	err = withTenantTx(ctx, pool, tenantID, func(tx pgx.Tx) error {
		if desired == "down" {
			tag, err := tx.Exec(ctx, `
				DELETE FROM compose_stacks
				WHERE id = $1::uuid AND organization_id = $2::uuid AND assigned_node_id IS NULL`, stackID, tenantID)
			if err != nil {
				return err
			}
			if tag.RowsAffected() == 1 {
				out = ComposeStack{ID: stackID, Status: "removed", DesiredAction: "down"}
				return writeAudit(ctx, tx, tenantID, "compose_stack.delete", "compose_stack", stackID, actor, nil)
			}
		}
		status := "pending"
		if desired == "down" {
			status = "removing"
		}
		item, err := scanComposeStack(tx.QueryRow(ctx, `
			UPDATE compose_stacks SET
				desired_action = $3,
				desired_generation = desired_generation + 1,
				status = $4,
				message = '',
				updated_at = NOW()
			WHERE id = $1::uuid AND organization_id = $2::uuid
			RETURNING `+composeStackColumns, stackID, tenantID, desired, status))
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrComposeStackNotFound
		}
		if err != nil {
			return err
		}
		out = item
		return writeAudit(ctx, tx, tenantID, "compose_stack."+strings.ToLower(strings.TrimSpace(action)), "compose_stack", stackID, actor, nil)
	})
	return out, err
}

// ClaimComposeStacksForNode assigns unplaced stacks to a Docker-capable node.
// Stacks of a project stay on one node so they can share networks (#144).
func ClaimComposeStacksForNode(ctx context.Context, pool *pgxpool.Pool, tenantID, nodeID string) (int64, error) {
	var claimed int64
	err := withTenantTx(ctx, pool, tenantID, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			UPDATE compose_stacks s SET assigned_node_id = $2::uuid, updated_at = NOW()
			WHERE s.organization_id = $1::uuid
			  AND s.assigned_node_id IS NULL
			  AND s.desired_action <> 'down'
			  AND EXISTS (
				SELECT 1 FROM nodes n
				WHERE n.id = $2::uuid AND n.organization_id = $1::uuid
				  AND (n.docker_socket OR n.containerd_socket)
				  AND NOT n.cordoned AND NOT n.draining
			  )
			  AND NOT EXISTS (
				SELECT 1 FROM compose_stacks other
				WHERE other.project_id = s.project_id
				  AND other.assigned_node_id IS NOT NULL
				  AND other.assigned_node_id <> $2::uuid
			  )`, tenantID, nodeID)
		if err != nil {
			return err
		}
		claimed = tag.RowsAffected()
		return nil
	})
	return claimed, err
}

// ListDesiredComposeStacksForNode returns every stack placed on the node.
func ListDesiredComposeStacksForNode(ctx context.Context, pool *pgxpool.Pool, tenantID, nodeID string) ([]ComposeStack, error) {
	out := []ComposeStack{}
	err := withTenantTx(ctx, pool, tenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT `+composeStackColumns+`
			FROM compose_stacks
			WHERE organization_id = $1::uuid AND assigned_node_id = $2::uuid AND desired_action <> 'none'
			ORDER BY created_at`, tenantID, nodeID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			item, err := scanComposeStack(rows)
			if err != nil {
				return err
			}
			out = append(out, item)
		}
		return rows.Err()
	})
	return out, err
}

// ResolveObservedStatus decides the stored status for an agent report. A report
// for an older generation must not hide a pending start/stop/redeploy/delete.
func ResolveObservedStatus(desiredAction string, desiredGeneration int64, observed ObservedComposeStack) string {
	status := strings.ToLower(strings.TrimSpace(observed.Status))
	switch status {
	case "deploying", "running", "degraded", "stopped", "failed":
	default:
		status = "failed"
	}
	if observed.ObservedGeneration < desiredGeneration && status != "deploying" {
		if desiredAction == "down" {
			return "removing"
		}
		return "pending"
	}
	return status
}

// ApplyObservedComposeStack stores an agent report. Only the assigned node may
// report a stack. A "removed" report for a deleted stack drops the row.
func ApplyObservedComposeStack(ctx context.Context, pool *pgxpool.Pool, tenantID, nodeID string, observed ObservedComposeStack) error {
	return withTenantTx(ctx, pool, tenantID, func(tx pgx.Tx) error {
		var desiredAction string
		var desiredGeneration int64
		err := tx.QueryRow(ctx, `
			SELECT desired_action, desired_generation FROM compose_stacks
			WHERE id = $1::uuid AND organization_id = $2::uuid AND assigned_node_id = $3::uuid
			FOR UPDATE`, observed.ID, tenantID, nodeID).Scan(&desiredAction, &desiredGeneration)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrComposeStackNotFound
		}
		if err != nil {
			return err
		}
		if strings.EqualFold(observed.Status, "removed") {
			if desiredAction == "down" && observed.ObservedGeneration >= desiredGeneration {
				_, err := tx.Exec(ctx, `DELETE FROM compose_stacks WHERE id = $1::uuid`, observed.ID)
				return err
			}
			observed.Status = "stopped"
		}
		services := observed.Services
		if services == nil {
			services = []ComposeStackService{}
		}
		encoded, err := json.Marshal(services)
		if err != nil {
			return err
		}
		status := ResolveObservedStatus(desiredAction, desiredGeneration, observed)
		if _, err := tx.Exec(ctx, `
			UPDATE compose_stacks SET
				status = $2,
				message = $3,
				services = $4::jsonb,
				observed_generation = GREATEST(observed_generation, $5),
				updated_at = NOW()
			WHERE id = $1::uuid`,
			observed.ID, status, truncate(observed.Message, 2000), string(encoded), observed.ObservedGeneration,
		); err != nil {
			return err
		}
		logs := observed.Logs
		if len(logs) > maxStackLogLinesPerReport {
			logs = logs[len(logs)-maxStackLogLinesPerReport:]
		}
		for _, line := range logs {
			if strings.TrimSpace(line.Line) == "" {
				continue
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO compose_stack_logs (organization_id, stack_id, service, line)
				VALUES ($1::uuid, $2::uuid, $3, $4)`,
				tenantID, observed.ID, truncate(line.Service, 120), truncate(line.Line, 4000),
			); err != nil {
				return err
			}
		}
		return nil
	})
}
