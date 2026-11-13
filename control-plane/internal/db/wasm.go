package db

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

type WasmFunction struct {
	ID             string
	OrganizationID string
	ProjectID      string
	Name           string
	ImageRef       string
	Status         string
	FuelLimit      int64
	MemoryLimitMB  int32
}

func CreateWasmFunction(ctx context.Context, pool *pgxpool.Pool, tenantID string, fn WasmFunction) (WasmFunction, error) {
	fn.Name = strings.TrimSpace(fn.Name)
	fn.ImageRef = strings.TrimSpace(fn.ImageRef)
	if fn.ProjectID == "" || fn.Name == "" || fn.ImageRef == "" {
		return WasmFunction{}, fmt.Errorf("project, name and image_ref are required")
	}
	if fn.FuelLimit <= 0 {
		fn.FuelLimit = 100_000_000
	}
	if fn.MemoryLimitMB <= 0 {
		fn.MemoryLimitMB = 128
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return WasmFunction{}, err
	}
	defer tx.Rollback(ctx)
	if err := WithTenant(ctx, tx, tenantID); err != nil {
		return WasmFunction{}, err
	}
	err = tx.QueryRow(ctx, `
		INSERT INTO wasm_functions (organization_id, project_id, name, image_ref, status, fuel_limit, memory_limit_mb)
		VALUES ($1::uuid, $2::uuid, $3, $4, 'pending', $5, $6)
		RETURNING id::text, organization_id::text, project_id::text, name, image_ref, status, fuel_limit, memory_limit_mb`,
		tenantID, fn.ProjectID, fn.Name, fn.ImageRef, fn.FuelLimit, fn.MemoryLimitMB,
	).Scan(
		&fn.ID, &fn.OrganizationID, &fn.ProjectID, &fn.Name, &fn.ImageRef, &fn.Status,
		&fn.FuelLimit, &fn.MemoryLimitMB,
	)
	if err != nil {
		return WasmFunction{}, fmt.Errorf("create wasm function: %w", err)
	}
	_, _ = tx.Exec(ctx, `UPDATE wasm_functions SET status = 'ready', updated_at = NOW() WHERE id = $1::uuid`, fn.ID)
	fn.Status = "ready"
	return fn, tx.Commit(ctx)
}

func ListWasmFunctions(ctx context.Context, pool *pgxpool.Pool, tenantID, projectID string) ([]WasmFunction, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	if err := WithTenant(ctx, tx, tenantID); err != nil {
		return nil, err
	}
	query := `
		SELECT id::text, organization_id::text, project_id::text, name, image_ref, status, fuel_limit, memory_limit_mb
		FROM wasm_functions WHERE organization_id = $1::uuid`
	args := []any{tenantID}
	if projectID != "" {
		query += ` AND project_id = $2::uuid`
		args = append(args, projectID)
	}
	query += ` ORDER BY name`
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]WasmFunction, 0)
	for rows.Next() {
		var item WasmFunction
		if err := rows.Scan(
			&item.ID, &item.OrganizationID, &item.ProjectID, &item.Name, &item.ImageRef, &item.Status,
			&item.FuelLimit, &item.MemoryLimitMB,
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
