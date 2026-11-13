package ops

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/xiron902224/norix/control-plane/internal/db"
)

func Start(ctx context.Context, pool *pgxpool.Pool, logger *slog.Logger) {
	if logger == nil {
		logger = slog.Default()
	}
	go startSandboxGC(ctx, pool, logger)
	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()
	runOnce(ctx, pool, logger)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			runOnce(ctx, pool, logger)
		}
	}
}

func runOnce(ctx context.Context, pool *pgxpool.Pool, logger *slog.Logger) {
	_ = db.RetainWorkloadLogs(ctx, pool, 500)
	_ = db.RetainBuildLogs(ctx, pool, 800)

	orgs, err := db.ListOrganizationIDs(ctx, pool)
	if err != nil {
		logger.Warn("ops list orgs", "error", err)
		return
	}
	for _, orgID := range orgs {
		marked, err := markStaleNodes(ctx, pool, orgID)
		if err != nil {
			logger.Warn("mark stale nodes", "org", orgID, "error", err)
			continue
		}
		for _, node := range marked {
			msg := "node went offline (heartbeat timeout)"
			_ = db.InsertAlert(ctx, pool, orgID, "node.down", "critical", msg, "node", node.ID)
			deliverWebhook(ctx, map[string]any{
				"kind": "node.down", "organization_id": orgID, "node_id": node.ID, "node_name": node.Name, "message": msg,
			})
		}
		failed, err := listNewlyFailedDeployments(ctx, pool, orgID)
		if err != nil {
			logger.Warn("list failed deploys", "org", orgID, "error", err)
			continue
		}
		for _, workloadID := range failed {
			exists, _ := db.HasRecentAlert(ctx, pool, orgID, "deploy.failed", "workload", workloadID, 10*time.Minute)
			if exists {
				continue
			}
			msg := "workload deploy failed"
			_ = db.InsertAlert(ctx, pool, orgID, "deploy.failed", "warning", msg, "workload", workloadID)
			deliverWebhook(ctx, map[string]any{
				"kind": "deploy.failed", "organization_id": orgID, "workload_id": workloadID, "message": msg,
			})
		}
	}
}

func markStaleNodes(ctx context.Context, pool *pgxpool.Pool, orgID string) ([]db.Node, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	if err := db.WithTenant(ctx, tx, orgID); err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `
		UPDATE nodes SET status = 'offline', updated_at = NOW()
		WHERE status = 'online'
		  AND last_seen_at IS NOT NULL
		  AND last_seen_at < NOW() - INTERVAL '45 seconds'
		  AND is_local = FALSE
		RETURNING id::text, organization_id::text, name, status, last_seen_at, is_local`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]db.Node, 0)
	for rows.Next() {
		var node db.Node
		if err := rows.Scan(&node.ID, &node.OrganizationID, &node.Name, &node.Status, &node.LastSeenAt, &node.IsLocal); err != nil {
			return nil, err
		}
		result = append(result, node)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, tx.Commit(ctx)
}

func listNewlyFailedDeployments(ctx context.Context, pool *pgxpool.Pool, orgID string) ([]string, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	if err := db.WithTenant(ctx, tx, orgID); err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `
		SELECT id::text FROM workloads
		WHERE status = 'failed' AND updated_at > NOW() - INTERVAL '30 seconds'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func deliverWebhook(ctx context.Context, payload map[string]any) {
	url := strings.TrimSpace(os.Getenv("NORIX_ALERT_WEBHOOK_URL"))
	if url == "" {
		return
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return
	}
	_ = resp.Body.Close()
}
