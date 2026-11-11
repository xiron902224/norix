package localruntime

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/xiron902224/norix/control-plane/internal/db"
)

// Start runs the embedded control-plane reconciler until ctx is cancelled.
func Start(ctx context.Context, pool *pgxpool.Pool, logger *slog.Logger) {
	if logger == nil {
		logger = slog.Default()
	}
	runtime, err := Detect(ctx)
	if err != nil {
		logger.Warn("local runtime disabled; deploy needs a remote agent or docker on this host", "error", err)
		// Still keep local nodes visible/online for inventory, without container reconcile.
		go touchLoop(ctx, pool, logger)
		return
	}
	logger.Info("local runtime enabled", "engine", runtime.engine)

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	runOnce(ctx, pool, runtime, logger)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			runOnce(ctx, pool, runtime, logger)
		}
	}
}

func touchLoop(ctx context.Context, pool *pgxpool.Pool, logger *slog.Logger) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		caps := db.DetectHostCapabilities()
		orgs, err := db.ListOrganizationIDs(ctx, pool)
		if err != nil {
			logger.Error("list organizations for local node", "error", err)
		} else {
			for _, orgID := range orgs {
				if _, err := db.EnsureLocalNode(ctx, pool, orgID, caps); err != nil {
					logger.Warn("ensure local node", "org", orgID, "error", err)
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

func runOnce(ctx context.Context, pool *pgxpool.Pool, runtime *Runtime, logger *slog.Logger) {
	caps := db.DetectHostCapabilities()
	orgs, err := db.ListOrganizationIDs(ctx, pool)
	if err != nil {
		logger.Error("list organizations for local reconcile", "error", err)
		return
	}
	for _, orgID := range orgs {
		node, err := db.EnsureLocalNode(ctx, pool, orgID, caps)
		if err != nil {
			logger.Warn("ensure local node", "org", orgID, "error", err)
			continue
		}
		if _, err := db.ReclaimDesiredWorkloads(ctx, pool, orgID, node.ID); err != nil {
			logger.Warn("reclaim desired for local node", "org", orgID, "error", err)
		}
		desired, err := db.ListDesiredWorkloadsForNode(ctx, pool, orgID, node.ID)
		if err != nil {
			logger.Warn("list desired for local node", "org", orgID, "error", err)
			continue
		}
		if len(desired) == 0 {
			continue
		}
		observed := runtime.Reconcile(ctx, desired)
		if err := db.ApplyObservedWorkloads(ctx, pool, orgID, observed); err != nil {
			logger.Warn("apply observed local workloads", "org", orgID, "error", err)
		}
	}
}
