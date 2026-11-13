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
		if err := db.SampleNodeHostMetrics(ctx, pool, orgID, node.ID); err != nil {
			logger.Warn("sample host metrics", "org", orgID, "error", err)
		}
		if _, err := db.EnsureProvisioningDatabases(ctx, pool, orgID, node.ID); err != nil {
			logger.Warn("provision managed databases", "org", orgID, "error", err)
		}
		if _, err := db.ReclaimDesiredWorkloads(ctx, pool, orgID, node.ID); err != nil {
			logger.Warn("reclaim desired for local node", "org", orgID, "error", err)
		}
		if _, err := db.ReclaimPendingBuilds(ctx, pool, orgID, node.ID); err != nil {
			logger.Warn("reclaim builds for local node", "org", orgID, "error", err)
		}

		// Reconcile containers first so a slow git build cannot block deploys/deletes.
		desired, err := db.ListDesiredWorkloadsForNode(ctx, pool, orgID, node.ID)
		if err != nil {
			logger.Warn("list desired for local node", "org", orgID, "error", err)
		} else if len(desired) > 0 {
			observed := runtime.Reconcile(ctx, desired)
			if err := db.ApplyObservedWorkloads(ctx, pool, orgID, observed); err != nil {
				logger.Warn("apply observed local workloads", "org", orgID, "error", err)
			}
		}

		edgePlan, err := db.GetDesiredEdgePlan(ctx, pool, orgID, node.ID)
		if err != nil {
			logger.Warn("desired edge plan", "org", orgID, "error", err)
		} else {
			// After compose recreate, Caddy falls back to bootstrap while DB still
			// says domains are active — force a full re-apply when hosts are missing.
			if len(edgePlan.Routes) == 0 {
				if forced, forceErr := db.ForceDesiredEdgePlan(ctx, pool, orgID, node.ID); forceErr == nil && len(forced.Routes) > 0 {
					hosts := make([]string, 0, len(forced.Routes))
					for _, route := range forced.Routes {
						if route.Action == "upsert" && route.Hostname != "" {
							hosts = append(hosts, route.Hostname)
						}
					}
					admin := forced.CaddyAdminURL
					if admin == "" {
						admin = db.CaddyAdminURL()
					}
					if CaddyMissingHosts(ctx, admin, hosts) {
						edgePlan = forced
						logger.Info("edge resync: caddy missing domain hosts", "org", orgID, "hosts", len(hosts))
					}
				}
			}
			if edgePlan.Generation > 0 && len(edgePlan.Routes) > 0 {
				observedEdge := runtime.ApplyEdge(ctx, edgePlan)
				_ = db.SaveEdgeConfigSnapshot(ctx, pool, orgID, edgePlan.Generation, "")
				if err := db.ApplyObservedEdge(ctx, pool, orgID, observedEdge, db.AuditActor{Email: "local-runtime"}); err != nil {
					logger.Warn("apply observed edge", "org", orgID, "error", err)
				}
			}
		}

		networkPlan, err := db.GetDesiredNetworkPlan(ctx, pool, orgID, node.ID)
		if err != nil {
			logger.Warn("desired network plan", "org", orgID, "error", err)
		} else if networkPlan.Generation > 0 {
			observedNetwork := runtime.ApplyNetwork(ctx, networkPlan)
			if err := db.ApplyObservedNetwork(ctx, pool, orgID, node.ID, observedNetwork); err != nil {
				logger.Warn("apply observed network", "org", orgID, "error", err)
			}
		}

		if wasmFns, wasmErr := db.ListWasmFunctions(ctx, pool, orgID, ""); wasmErr == nil {
			_ = ApplyWasmPlan(ctx, orgID, wasmFns)
		}

		builds, err := db.ListDesiredBuildsForNode(ctx, pool, orgID, node.ID)
		if err != nil {
			logger.Warn("list desired builds for local node", "org", orgID, "error", err)
		} else if len(builds) > 0 {
			logger.Info("running local builds", "org", orgID, "count", len(builds))
			buildCtx, cancel := context.WithTimeout(ctx, 20*time.Minute)
			observedBuilds := runtime.RunBuilds(buildCtx, builds, func(buildID, line string) {
				if err := db.AppendBuildLogLines(ctx, pool, orgID, buildID, []string{line}); err != nil {
					logger.Debug("append build log", "build", buildID, "error", err)
				}
			})
			cancel()
			for _, observedBuild := range observedBuilds {
				// Lines were already streamed live; avoid duplicating in ApplyObservedBuild.
				observedBuild.LogLines = nil
				if err := db.ApplyObservedBuild(ctx, pool, orgID, observedBuild, db.AuditActor{Email: "local-runtime"}); err != nil {
					logger.Warn("apply observed build", "org", orgID, "build", observedBuild.ID, "error", err)
				}
			}
		}
	}
}
