package db

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type DomainRoute struct {
	ID             string
	OrganizationID string
	WorkloadID     string
	Hostname       string
	Status         string
	TLSMode        string
	EdgeGeneration int64
	LastError      string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type DesiredEdgeRoute struct {
	DomainID       string
	Hostname       string
	Upstream       string
	TLSMode        string
	EdgeGeneration int64
	Action         string
}

type DesiredEdgePlan struct {
	Generation    int64
	Routes        []DesiredEdgeRoute
	ACMECA        string
	CaddyAdminURL string
}

type ObservedEdge struct {
	Generation    int64
	Status        string
	Message       string
	LogLines      []string
	RouteStatuses map[string]string
}

func ACMECA() string {
	if value := strings.TrimSpace(os.Getenv("NORIX_ACME_CA")); value != "" {
		return value
	}
	return ""
}

func CaddyAdminURL() string {
	if value := strings.TrimSpace(os.Getenv("NORIX_CADDY_ADMIN_URL")); value != "" {
		return strings.TrimRight(value, "/")
	}
	return "http://127.0.0.1:2019"
}

func EdgeUpstreamHost() string {
	if value := strings.TrimSpace(os.Getenv("NORIX_EDGE_UPSTREAM_HOST")); value != "" {
		return value
	}
	return "127.0.0.1"
}

func DomainPublicURL(hostname string) string {
	hostname = NormalizeHostname(hostname)
	if hostname == "" {
		return ""
	}
	port := strings.TrimSpace(os.Getenv("NORIX_EDGE_HTTPS_PORT"))
	if port == "" {
		port = "8443"
	}
	if port == "443" {
		return "https://" + hostname
	}
	return fmt.Sprintf("https://%s:%s", hostname, port)
}

func NormalizeHostname(hostname string) string {
	hostname = strings.ToLower(strings.TrimSpace(hostname))
	hostname = strings.TrimPrefix(hostname, "https://")
	hostname = strings.TrimPrefix(hostname, "http://")
	hostname = strings.TrimSuffix(hostname, "/")
	return hostname
}

func ChooseTLSMode(hostname, requested string) string {
	requested = strings.ToLower(strings.TrimSpace(requested))
	if requested == "internal" || requested == "acme" {
		return requested
	}
	if strings.HasSuffix(hostname, ".localhost") || hostname == "localhost" || strings.HasSuffix(hostname, ".local") {
		return "internal"
	}
	if ACMECA() != "" || os.Getenv("NORIX_FORCE_ACME") == "1" {
		return "acme"
	}
	return "internal"
}

func AssignDomain(ctx context.Context, pool *pgxpool.Pool, tenantID, workloadID, hostname, tlsMode string, actor AuditActor) (DomainRoute, error) {
	hostname = NormalizeHostname(hostname)
	if hostname == "" || workloadID == "" {
		return DomainRoute{}, fmt.Errorf("workload id and hostname are required")
	}
	tlsMode = ChooseTLSMode(hostname, tlsMode)

	tx, err := pool.Begin(ctx)
	if err != nil {
		return DomainRoute{}, err
	}
	defer tx.Rollback(ctx)
	if err := WithTenant(ctx, tx, tenantID); err != nil {
		return DomainRoute{}, err
	}

	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM workloads WHERE id = $1::uuid)`, workloadID).Scan(&exists); err != nil {
		return DomainRoute{}, err
	}
	if !exists {
		return DomainRoute{}, fmt.Errorf("workload not found")
	}

	var generation int64
	if err := tx.QueryRow(ctx, `
		INSERT INTO edge_snapshots (organization_id, generation, status, message)
		VALUES ($1::uuid,
			COALESCE((SELECT MAX(generation) FROM edge_snapshots WHERE organization_id = $1::uuid), 0) + 1,
			'pending', 'domain assigned')
		RETURNING generation`, tenantID).Scan(&generation); err != nil {
		return DomainRoute{}, fmt.Errorf("bump edge generation: %w", err)
	}

	var route DomainRoute
	err = tx.QueryRow(ctx, `
		INSERT INTO domain_routes (organization_id, workload_id, hostname, status, tls_mode, edge_generation)
		VALUES ($1::uuid, $2::uuid, $3, 'pending', $4, $5)
		ON CONFLICT (organization_id, hostname)
		DO UPDATE SET
			workload_id = EXCLUDED.workload_id,
			tls_mode = EXCLUDED.tls_mode,
			status = 'pending',
			edge_generation = EXCLUDED.edge_generation,
			last_error = '',
			updated_at = NOW()
		RETURNING id::text, organization_id::text, workload_id::text, hostname, status, tls_mode, edge_generation, last_error, created_at, updated_at`,
		tenantID, workloadID, hostname, tlsMode, generation,
	).Scan(
		&route.ID, &route.OrganizationID, &route.WorkloadID, &route.Hostname, &route.Status, &route.TLSMode,
		&route.EdgeGeneration, &route.LastError, &route.CreatedAt, &route.UpdatedAt,
	)
	if err != nil {
		return DomainRoute{}, fmt.Errorf("assign domain: %w", err)
	}

	_, _ = tx.Exec(ctx, `UPDATE workloads SET domain = $2, updated_at = NOW() WHERE id = $1::uuid`, workloadID, hostname)
	if err := writeAudit(ctx, tx, tenantID, "domain.assigned", "domain", route.ID, actor, map[string]string{
		"hostname":    hostname,
		"workload_id": workloadID,
		"tls_mode":    tlsMode,
	}); err != nil {
		return DomainRoute{}, err
	}
	return route, tx.Commit(ctx)
}

func UnassignDomain(ctx context.Context, pool *pgxpool.Pool, tenantID, domainID string, actor AuditActor) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := WithTenant(ctx, tx, tenantID); err != nil {
		return err
	}
	var generation int64
	if err := tx.QueryRow(ctx, `
		INSERT INTO edge_snapshots (organization_id, generation, status, message)
		VALUES ($1::uuid,
			COALESCE((SELECT MAX(generation) FROM edge_snapshots WHERE organization_id = $1::uuid), 0) + 1,
			'pending', 'domain unassigned')
		RETURNING generation`, tenantID).Scan(&generation); err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `
		UPDATE domain_routes
		SET status = 'deleting', edge_generation = $2, updated_at = NOW()
		WHERE id = $1::uuid`, domainID, generation)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("domain not found")
	}
	if err := writeAudit(ctx, tx, tenantID, "domain.unassigned", "domain", domainID, actor, nil); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func ListDomains(ctx context.Context, pool *pgxpool.Pool, tenantID, projectID string) ([]DomainRoute, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	if err := WithTenant(ctx, tx, tenantID); err != nil {
		return nil, err
	}
	query := `
		SELECT d.id::text, d.organization_id::text, d.workload_id::text, d.hostname, d.status, d.tls_mode,
		       d.edge_generation, d.last_error, d.created_at, d.updated_at
		FROM domain_routes d`
	args := []any{}
	if projectID != "" {
		query += ` JOIN workloads w ON w.id = d.workload_id WHERE w.project_id = $1::uuid`
		args = append(args, projectID)
	}
	query += ` ORDER BY d.hostname`
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]DomainRoute, 0)
	for rows.Next() {
		var route DomainRoute
		if err := rows.Scan(
			&route.ID, &route.OrganizationID, &route.WorkloadID, &route.Hostname, &route.Status, &route.TLSMode,
			&route.EdgeGeneration, &route.LastError, &route.CreatedAt, &route.UpdatedAt,
		); err != nil {
			return nil, err
		}
		result = append(result, route)
	}
	return result, rows.Err()
}

func GetDesiredEdgePlan(ctx context.Context, pool *pgxpool.Pool, organizationID, nodeID string) (DesiredEdgePlan, error) {
	return getDesiredEdgePlan(ctx, pool, organizationID, nodeID, false)
}

// ForceDesiredEdgePlan rebuilds the full Caddy route set (active + pending +
// retryable failed). Used when the edge proxy lost its config (compose recreate).
func ForceDesiredEdgePlan(ctx context.Context, pool *pgxpool.Pool, organizationID, nodeID string) (DesiredEdgePlan, error) {
	return getDesiredEdgePlan(ctx, pool, organizationID, nodeID, true)
}

func getDesiredEdgePlan(ctx context.Context, pool *pgxpool.Pool, organizationID, nodeID string, force bool) (DesiredEdgePlan, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return DesiredEdgePlan{}, err
	}
	defer tx.Rollback(ctx)
	if err := WithTenant(ctx, tx, organizationID); err != nil {
		return DesiredEdgePlan{}, err
	}

	var generation int64
	_ = tx.QueryRow(ctx, `
		SELECT COALESCE(MAX(generation), 0) FROM edge_snapshots WHERE organization_id = $1::uuid`, organizationID,
	).Scan(&generation)

	rows, err := tx.Query(ctx, `
		SELECT d.id::text, d.hostname, d.tls_mode, d.edge_generation, d.status,
		       COALESCE(w.host_port, 0), COALESCE(w.assigned_node_id::text, ''), d.updated_at
		FROM domain_routes d
		JOIN workloads w ON w.id = d.workload_id
		WHERE d.status IN ('pending', 'provisioning', 'active', 'deleting', 'failed')
		ORDER BY d.hostname`)
	if err != nil {
		return DesiredEdgePlan{}, err
	}
	defer rows.Close()

	plan := DesiredEdgePlan{
		Generation:    generation,
		Routes:        make([]DesiredEdgeRoute, 0),
		ACMECA:        ACMECA(),
		CaddyAdminURL: CaddyAdminURL(),
	}
	needsApply := force
	type pendingRow struct {
		route        DesiredEdgeRoute
		status       string
		hostPort     int32
		assignedNode string
		updatedAt    time.Time
	}
	pending := make([]pendingRow, 0)
	for rows.Next() {
		var item pendingRow
		if err := rows.Scan(&item.route.DomainID, &item.route.Hostname, &item.route.TLSMode, &item.route.EdgeGeneration, &item.status, &item.hostPort, &item.assignedNode, &item.updatedAt); err != nil {
			return DesiredEdgePlan{}, err
		}
		if item.status == "pending" || item.status == "provisioning" || item.status == "deleting" {
			needsApply = true
		}
		// Retry failed routes after a short cooldown so a temporary dead
		// upstream (container restart) does not stick forever in the UI.
		if item.status == "failed" && item.hostPort > 0 && time.Since(item.updatedAt) >= 30*time.Second {
			needsApply = true
		}
		pending = append(pending, item)
	}
	if err := rows.Err(); err != nil {
		return DesiredEdgePlan{}, err
	}
	if !needsApply {
		return DesiredEdgePlan{}, tx.Commit(ctx)
	}

	for _, item := range pending {
		route := item.route
		if item.status == "deleting" {
			route.Action = "delete"
			plan.Routes = append(plan.Routes, route)
			continue
		}
		switch item.status {
		case "failed":
			if item.hostPort <= 0 || time.Since(item.updatedAt) < 30*time.Second {
				continue
			}
		case "pending", "provisioning", "active":
			// include
		default:
			continue
		}
		if item.hostPort <= 0 {
			continue
		}
		if item.assignedNode != "" && item.assignedNode != nodeID {
			continue
		}
		route.Action = "upsert"
		route.Upstream = fmt.Sprintf("http://%s:%d", EdgeUpstreamHost(), item.hostPort)
		plan.Routes = append(plan.Routes, route)
	}
	if len(plan.Routes) == 0 {
		return DesiredEdgePlan{}, tx.Commit(ctx)
	}

	_, _ = tx.Exec(ctx, `
		UPDATE domain_routes SET status = 'provisioning', updated_at = NOW()
		WHERE status = 'pending'`)
	if err := tx.Commit(ctx); err != nil {
		return DesiredEdgePlan{}, err
	}
	return plan, nil
}

func ApplyObservedEdge(ctx context.Context, pool *pgxpool.Pool, organizationID string, observed ObservedEdge, actor AuditActor) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := WithTenant(ctx, tx, organizationID); err != nil {
		return err
	}

	status := strings.TrimSpace(observed.Status)
	if status == "" {
		status = "failed"
	}
	snapshotStatus := status
	if status == "active" {
		snapshotStatus = "active"
	} else if status == "rolled_back" {
		snapshotStatus = "rolled_back"
	} else {
		snapshotStatus = "failed"
	}

	_, err = tx.Exec(ctx, `
		UPDATE edge_snapshots SET status = $3, message = $4
		WHERE organization_id = $1::uuid AND generation = $2`,
		organizationID, observed.Generation, snapshotStatus, observed.Message)
	if err != nil {
		return err
	}

	for domainID, routeStatus := range observed.RouteStatuses {
		routeStatus = strings.TrimSpace(routeStatus)
		switch routeStatus {
		case "active":
			_, _ = tx.Exec(ctx, `
				UPDATE domain_routes SET status = 'active', last_error = '', updated_at = NOW()
				WHERE id = $1::uuid`, domainID)
		case "deleted":
			_, _ = tx.Exec(ctx, `DELETE FROM domain_routes WHERE id = $1::uuid`, domainID)
		case "failed":
			_, _ = tx.Exec(ctx, `
				UPDATE domain_routes SET status = 'failed', last_error = $2, updated_at = NOW()
				WHERE id = $1::uuid`, domainID, observed.Message)
		}
	}

	if status == "active" || status == "rolled_back" || status == "failed" {
		_ = writeAudit(ctx, tx, organizationID, "edge."+status, "edge", fmt.Sprintf("%d", observed.Generation), actor, map[string]string{
			"message": observed.Message,
		})
	}
	return tx.Commit(ctx)
}

func LatestActiveEdgeConfig(ctx context.Context, pool *pgxpool.Pool, tenantID string) (string, int64, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return "", 0, err
	}
	defer tx.Rollback(ctx)
	if err := WithTenant(ctx, tx, tenantID); err != nil {
		return "", 0, err
	}
	var config string
	var generation int64
	err = tx.QueryRow(ctx, `
		SELECT config_json, generation FROM edge_snapshots
		WHERE organization_id = $1::uuid AND status = 'active'
		ORDER BY generation DESC LIMIT 1`, tenantID).Scan(&config, &generation)
	if err == pgx.ErrNoRows {
		return "", 0, nil
	}
	if err != nil {
		return "", 0, err
	}
	return config, generation, tx.Commit(ctx)
}

func SaveEdgeConfigSnapshot(ctx context.Context, pool *pgxpool.Pool, tenantID string, generation int64, configJSON string) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := WithTenant(ctx, tx, tenantID); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		UPDATE edge_snapshots SET config_json = $3
		WHERE organization_id = $1::uuid AND generation = $2`, tenantID, generation, configJSON)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}
