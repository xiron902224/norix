package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"connectrpc.com/connect/v2"
	"connectrpc.com/connect/v2/connecthttp"
	"github.com/xiron902224/norix/control-plane/internal/api"
	norixdb "github.com/xiron902224/norix/control-plane/internal/db"
	"github.com/xiron902224/norix/control-plane/internal/gen/v1/paasv1connect"
	"github.com/xiron902224/norix/control-plane/internal/localruntime"
	"github.com/xiron902224/norix/control-plane/internal/ops"
	"github.com/xiron902224/norix/control-plane/internal/webui"
)

var logger = slog.New(slog.NewTextHandler(os.Stdout, nil))

func newHandler(authService api.AuthService) http.Handler {
	mux := http.NewServeMux()
	apiMux := http.NewServeMux()
	var interceptors []connect.ServerInterceptor
	interceptors = append(interceptors, api.NewRateLimiter(10).Interceptor())
	if authService.BootstrapPool() != nil {
		interceptors = append(interceptors, api.WithAuthentication(authService.BootstrapPool()))
	}
	server := connect.NewServer(interceptors...)
	paasv1connect.RegisterAuthServiceHandler(server, authService)
	if authService.BootstrapPool() != nil {
		paasv1connect.RegisterProjectServiceHandler(server, api.NewProjectService(authService.BootstrapPool()))
		paasv1connect.RegisterWorkloadServiceHandler(server, api.NewWorkloadService(authService.BootstrapPool()))
		paasv1connect.RegisterAgentServiceHandler(server, api.NewAgentService(authService.BootstrapPool()))
	}
	connecthttp.Mount(apiMux, server)
	mux.Handle("/api/", http.StripPrefix("/api", apiMux))

	mux.HandleFunc("GET /healthz", func(response http.ResponseWriter, _ *http.Request) {
		response.Header().Set("Content-Type", "text/plain; charset=utf-8")
		response.WriteHeader(http.StatusOK)
		_, _ = response.Write([]byte("ok\n"))
	})
	if authService.BootstrapPool() != nil {
		pool := authService.BootstrapPool()
		mux.HandleFunc("POST /hooks/github/{token}", api.GitHubWebhook(pool))
		mux.HandleFunc("POST /api/webhooks/workloads/{workloadID}", api.CreateWebhookTokenHandler(pool))
		mux.HandleFunc("GET /api/ops/backups", api.OpsBackupsHandler(pool))
		mux.HandleFunc("POST /api/ops/backups", api.OpsBackupsHandler(pool))
		mux.HandleFunc("POST /api/ops/backups/{backupID}/restore", api.OpsRestoreHandler(pool))
		mux.HandleFunc("GET /api/ops/alerts", api.OpsAlertsHandler(pool))
	}
	mux.Handle("/", webui.Handler())

	return mux
}

func httpAddress() string {
	if address := os.Getenv("HTTP_ADDR"); address != "" {
		return address
	}
	return ":8080"
}

func main() {
	migrateOnly := flag.Bool("migrate-only", false, "apply migrations and exit")
	flag.Parse()

	ctx := context.Background()
	authService := api.AuthService{}
	databaseURL := os.Getenv("DATABASE_URL")
	migrationURL := os.Getenv("MIGRATION_DATABASE_URL")
	if migrationURL == "" {
		migrationURL = databaseURL
	}
	if *migrateOnly {
		migrateURL := migrationURL
		if migrateURL == "" {
			migrateURL = databaseURL
		}
		if migrateURL == "" {
			logger.Error("DATABASE_URL or MIGRATION_DATABASE_URL required for -migrate-only")
			os.Exit(1)
		}
		if err := norixdb.Migrate(migrateURL); err != nil {
			logger.Error("database migrations failed", "error", err)
			os.Exit(1)
		}
		logger.Info("database migrations completed")
		return
	}

	if databaseURL != "" {
		if migrationURL != databaseURL {
			if err := norixdb.Migrate(migrationURL); err != nil {
				logger.Error("database migrations failed", "error", err)
				os.Exit(1)
			}
		} else if err := norixdb.Migrate(databaseURL); err != nil {
			logger.Error("database migrations failed", "error", err)
			os.Exit(1)
		}
		appPool, err := norixdb.Open(ctx, databaseURL)
		if err != nil {
			logger.Error("database connection failed", "error", err)
			os.Exit(1)
		}
		defer appPool.Close()

		bootstrapPool := appPool
		if migrationURL != databaseURL {
			bootstrapPool, err = norixdb.Open(ctx, migrationURL)
			if err != nil {
				logger.Error("bootstrap database connection failed", "error", err)
				os.Exit(1)
			}
			defer bootstrapPool.Close()
		}
		authService = api.NewAuthService(appPool, bootstrapPool)
		logger.Info("database connected")

		backgroundCtx, cancelBackground := context.WithCancel(context.Background())
		defer cancelBackground()
		go localruntime.Start(backgroundCtx, bootstrapPool, logger)
		go ops.Start(backgroundCtx, bootstrapPool, logger)

		go func() {
			sessionTicker := time.NewTicker(time.Hour)
			nodeTicker := time.NewTicker(20 * time.Second)
			defer sessionTicker.Stop()
			defer nodeTicker.Stop()
			cleanupSessions := func() {
				removed, err := norixdb.CleanupExpiredSessions(context.Background(), appPool)
				if err != nil {
					logger.Error("session cleanup failed", "error", err)
					return
				}
				logger.Info("session cleanup completed", "removed", removed)
			}
			cleanupNodes := func() {
				marked, err := norixdb.MarkStaleNodesOffline(context.Background(), bootstrapPool, 45*time.Second)
				if err != nil {
					logger.Error("stale node cleanup failed", "error", err)
					return
				}
				if marked > 0 {
					logger.Info("marked stale nodes offline", "count", marked)
				}
				moved, rescheduleErr := norixdb.RescheduleAllOrphans(context.Background(), bootstrapPool)
				if rescheduleErr != nil {
					logger.Error("replica reschedule failed", "error", rescheduleErr)
					return
				}
				if moved > 0 {
					logger.Info("rescheduled orphan replicas", "count", moved)
				}
				purged, purgeErr := norixdb.PurgeAllDeletingWorkloads(context.Background(), bootstrapPool)
				if purgeErr != nil {
					logger.Error("purge deleting workloads failed", "error", purgeErr)
					return
				}
				if purged > 0 {
					logger.Info("purged deleted workloads", "count", purged)
				}
			}
			cleanupSessions()
			cleanupNodes()
			for {
				select {
				case <-sessionTicker.C:
					cleanupSessions()
				case <-nodeTicker.C:
					cleanupNodes()
				}
			}
		}()
	}

	server := &http.Server{
		Addr:              httpAddress(),
		Handler:           api.WithCORS(newHandler(authService)),
		ReadHeaderTimeout: 5 * time.Second,
	}

	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-shutdown
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			logger.Error("graceful shutdown failed", "error", err)
		}
	}()

	logger.Info("control plane started", "address", server.Addr)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error("control plane stopped", "error", err)
		os.Exit(1)
	}
}
