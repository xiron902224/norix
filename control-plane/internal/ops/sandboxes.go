package ops

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/xiron902224/norix/control-plane/internal/db"
)

func startSandboxGC(ctx context.Context, pool *pgxpool.Pool, logger *slog.Logger) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		orgs, err := db.ListOrganizationIDs(ctx, pool)
		if err == nil {
			for _, orgID := range orgs {
				count, gcErr := db.GCExpiredSandboxes(ctx, pool, orgID)
				if gcErr != nil {
					logger.Warn("sandbox gc", "org", orgID, "error", gcErr)
				} else if count > 0 {
					logger.Info("expired sandboxes", "org", orgID, "count", count)
				}
				previews, previewErr := db.GCExpiredPRPreviews(ctx, pool, orgID)
				if previewErr != nil {
					logger.Warn("preview gc", "org", orgID, "error", previewErr)
				} else if previews > 0 {
					logger.Info("expired pr previews", "org", orgID, "count", previews)
				}
				if warmErr := db.EnsureSandboxWarmPool(ctx, pool, orgID); warmErr != nil {
					logger.Warn("sandbox warm pool", "org", orgID, "error", warmErr)
				}
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
