package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/xiron902224/norix/control-plane/internal/limits"
)

// EnsureScaleRoom checks org/project hard caps before creating a resource (#45).
func EnsureScaleRoom(ctx context.Context, pool *pgxpool.Pool, tenantID, projectID, kind string) error {
	caps := limits.FromEnv()
	return withTenantTx(ctx, pool, tenantID, func(tx pgx.Tx) error {
		switch kind {
		case "project":
			var count int
			if err := tx.QueryRow(ctx, `SELECT count(*) FROM projects`).Scan(&count); err != nil {
				return err
			}
			if count >= caps.MaxProjectsPerOrg {
				return fmt.Errorf("organization already has %d projects (NORIX_MAX_PROJECTS_PER_ORG=%d)", count, caps.MaxProjectsPerOrg)
			}
		case "workload":
			var count int
			if err := tx.QueryRow(ctx, `SELECT count(*) FROM workloads WHERE project_id = $1::uuid AND status <> 'deleting'`, projectID).Scan(&count); err != nil {
				return err
			}
			if count >= caps.MaxWorkloadsPerProject {
				return fmt.Errorf("project already has %d workloads (NORIX_MAX_WORKLOADS_PER_PROJECT=%d)", count, caps.MaxWorkloadsPerProject)
			}
		case "node":
			var count int
			if err := tx.QueryRow(ctx, `SELECT count(*) FROM nodes`).Scan(&count); err != nil {
				return err
			}
			if count >= caps.MaxNodesPerOrg {
				return fmt.Errorf("organization already has %d nodes (NORIX_MAX_NODES_PER_ORG=%d)", count, caps.MaxNodesPerOrg)
			}
		default:
			return fmt.Errorf("unknown scale kind %q", kind)
		}
		return nil
	})
}
