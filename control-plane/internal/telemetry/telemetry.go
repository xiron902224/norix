package telemetry

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"
)

// Enabled reports whether anonymous opt-in telemetry is on (NORIX_TELEMETRY=1).
func Enabled() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("NORIX_TELEMETRY")))
	return v == "1" || v == "true" || v == "yes"
}

// Endpoint returns NORIX_TELEMETRY_URL. Empty means no-op (even when Enabled).
func Endpoint() string {
	return strings.TrimSpace(os.Getenv("NORIX_TELEMETRY_URL"))
}

type payload struct {
	Product   string `json:"product"`
	Version   string `json:"version"`
	GOOS      string `json:"goos"`
	GOARCH    string `json:"goarch"`
	Anonymous bool   `json:"anonymous"`
	Timestamp string `json:"timestamp"`
}

// Start posts an anonymous heartbeat on an interval when Enabled and a URL is set.
// Default is off (NORIX_TELEMETRY unset). Without NORIX_TELEMETRY_URL the loop is a no-op.
func Start(ctx context.Context, logger *slog.Logger, version string) {
	if !Enabled() {
		return
	}
	if logger == nil {
		logger = slog.Default()
	}
	if version == "" {
		version = "1.0.0"
	}
	interval := 24 * time.Hour
	if raw := strings.TrimSpace(os.Getenv("NORIX_TELEMETRY_INTERVAL")); raw != "" {
		if d, err := time.ParseDuration(raw); err == nil && d >= time.Minute {
			interval = d
		}
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		send(ctx, logger, version)
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				send(ctx, logger, version)
			}
		}
	}()
}

func send(ctx context.Context, logger *slog.Logger, version string) {
	url := Endpoint()
	body, _ := json.Marshal(payload{
		Product:   "norix",
		Version:   version,
		GOOS:      runtime.GOOS,
		GOARCH:    runtime.GOARCH,
		Anonymous: true,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	})
	if url == "" {
		logger.Debug("telemetry enabled but NORIX_TELEMETRY_URL unset; no-op", "bytes", len(body))
		return
	}
	reqCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		logger.Debug("telemetry build request", "error", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "norix-telemetry/1")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		logger.Debug("telemetry send failed", "error", err)
		return
	}
	_ = resp.Body.Close()
	logger.Info("telemetry heartbeat sent", "status", resp.StatusCode)
}
