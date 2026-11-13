package db

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type UsageRecord struct {
	ID        int64
	TeamID    string
	Metric    string
	Quantity  float64
	Unit      string
	Recorded  time.Time
	Metadata  map[string]string
}

func RecordUsage(ctx context.Context, pool *pgxpool.Pool, organizationID, teamID, metric string, quantity float64, unit string, metadata map[string]string) error {
	payload := []byte(`{}`)
	if len(metadata) > 0 {
		encoded, err := json.Marshal(metadata)
		if err != nil {
			return fmt.Errorf("encode usage metadata: %w", err)
		}
		payload = encoded
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin record usage: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := WithTenant(ctx, tx, organizationID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO usage_records (organization_id, team_id, metric, quantity, unit, metadata)
		VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6::jsonb)`,
		organizationID, teamID, metric, quantity, unit, payload,
	); err != nil {
		return fmt.Errorf("insert usage record: %w", err)
	}
	return tx.Commit(ctx)
}

func ListUsageRecords(ctx context.Context, pool *pgxpool.Pool, organizationID string, limit int) ([]UsageRecord, error) {
	if limit <= 0 || limit > 50000 {
		limit = 5000
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin list usage: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := WithTenant(ctx, tx, organizationID); err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `
		SELECT id, team_id::text, metric, quantity::float8, unit, recorded_at, metadata::text
		FROM usage_records
		WHERE organization_id = $1::uuid
		ORDER BY recorded_at DESC
		LIMIT $2`, organizationID, limit)
	if err != nil {
		return nil, fmt.Errorf("list usage records: %w", err)
	}
	defer rows.Close()
	out := make([]UsageRecord, 0)
	for rows.Next() {
		var item UsageRecord
		var metaJSON string
		if err := rows.Scan(&item.ID, &item.TeamID, &item.Metric, &item.Quantity, &item.Unit, &item.Recorded, &metaJSON); err != nil {
			return nil, fmt.Errorf("scan usage: %w", err)
		}
		item.Metadata = map[string]string{}
		_ = json.Unmarshal([]byte(metaJSON), &item.Metadata)
		out = append(out, item)
	}
	return out, rows.Err()
}
