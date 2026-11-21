package db

import (
	"context"
	"fmt"
	"hash/crc32"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/xiron902224/norix/control-plane/internal/network"
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
	ListenPort     int32
	Hostname       string
}

// WasmListenPort returns a stable host port in 19000–19999 from the function id (#47).
func WasmListenPort(id string) int32 {
	h := crc32.ChecksumIEEE([]byte(strings.TrimSpace(id)))
	return 19000 + int32(h%1000)
}

func wasmHostname(projectName, fnName string) string {
	return network.ServiceFQDN(projectName+"-wasm", fnName)
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
	var projectName string
	if err := tx.QueryRow(ctx, `SELECT name FROM projects WHERE id = $1::uuid`, fn.ProjectID).Scan(&projectName); err != nil {
		if err == pgx.ErrNoRows {
			return WasmFunction{}, fmt.Errorf("project not found")
		}
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
	fn.ListenPort = WasmListenPort(fn.ID)
	fn.Hostname = wasmHostname(projectName, fn.Name)
	_, err = tx.Exec(ctx, `
		UPDATE wasm_functions SET status = 'ready', listen_port = $2, hostname = $3, updated_at = NOW()
		WHERE id = $1::uuid`, fn.ID, fn.ListenPort, fn.Hostname)
	if err != nil {
		return WasmFunction{}, err
	}
	fn.Status = "ready"
	return fn, tx.Commit(ctx)
}

// ApplyObservedWasm updates wasm_functions status from agent ReportWasm.
func ApplyObservedWasm(ctx context.Context, pool *pgxpool.Pool, tenantID string, functions []ObservedWasmFunction) error {
	if len(functions) == 0 {
		return nil
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := WithTenant(ctx, tx, tenantID); err != nil {
		return err
	}
	for _, item := range functions {
		status := strings.TrimSpace(item.Status)
		if status == "" {
			status = "ready"
		}
		_, err := tx.Exec(ctx, `
			UPDATE wasm_functions
			SET status = $2, updated_at = NOW()
			WHERE organization_id = $1::uuid AND id = $3::uuid`,
			tenantID, status, item.ID,
		)
		if err != nil {
			return fmt.Errorf("apply observed wasm %s: %w", item.ID, err)
		}
	}
	return tx.Commit(ctx)
}

type ObservedWasmFunction struct {
	ID     string
	Status string
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
		SELECT id::text, organization_id::text, project_id::text, name, image_ref, status, fuel_limit, memory_limit_mb,
		       COALESCE(listen_port, 0), COALESCE(hostname, '')
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
			&item.FuelLimit, &item.MemoryLimitMB, &item.ListenPort, &item.Hostname,
		); err != nil {
			return nil, err
		}
		if item.ListenPort <= 0 {
			item.ListenPort = WasmListenPort(item.ID)
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, tx.Commit(ctx)
}

// WasmEdgeRoutes builds Caddy upserts for wasm functions with hostname + listen_port (#47).
func WasmEdgeRoutes(ctx context.Context, tx pgx.Tx) ([]DesiredEdgeRoute, error) {
	rows, err := tx.Query(ctx, `
		SELECT id::text, COALESCE(hostname, ''), COALESCE(listen_port, 0)
		FROM wasm_functions
		WHERE status IN ('ready', 'running', 'pending', 'degraded')
		  AND COALESCE(hostname, '') <> ''
		  AND COALESCE(listen_port, 0) > 0
		ORDER BY hostname`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]DesiredEdgeRoute, 0)
	for rows.Next() {
		var id, hostname string
		var port int32
		if err := rows.Scan(&id, &hostname, &port); err != nil {
			return nil, err
		}
		out = append(out, DesiredEdgeRoute{
			DomainID:       id,
			Hostname:       hostname,
			TLSMode:        "none",
			Action:         "upsert",
			Upstream:       fmt.Sprintf("http://%s:%d", EdgeUpstreamHost(), port),
			EdgeGeneration: 1,
		})
	}
	return out, rows.Err()
}
