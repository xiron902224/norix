package statuspage

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Component struct {
	Name   string `json:"name"`
	Status string `json:"status"` // operational | degraded | down
	Detail string `json:"detail,omitempty"`
}

type Report struct {
	Status     string      `json:"status"`
	Version    string      `json:"version"`
	PublicURL  string      `json:"public_url,omitempty"`
	CheckedAt  time.Time   `json:"checked_at"`
	Components []Component `json:"components"`
}

// Handler returns a public JSON status page at GET /status.
func Handler(pool *pgxpool.Pool, version string) http.HandlerFunc {
	if version == "" {
		version = os.Getenv("NORIX_VERSION")
	}
	if version == "" {
		version = "1.0.0"
	}
	return func(w http.ResponseWriter, r *http.Request) {
		report := Build(r.Context(), pool, version)
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(report)
	}
}

func Build(ctx context.Context, pool *pgxpool.Pool, version string) Report {
	components := []Component{
		{Name: "api", Status: "operational", Detail: "http listener up"},
	}
	dbStatus := Component{Name: "postgres", Status: "operational"}
	if pool == nil {
		dbStatus.Status = "down"
		dbStatus.Detail = "database not configured"
	} else {
		pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		if err := pool.Ping(pingCtx); err != nil {
			dbStatus.Status = "down"
			dbStatus.Detail = err.Error()
		}
	}
	components = append(components, dbStatus)

	telemetryStatus := Component{Name: "telemetry", Status: "operational", Detail: "disabled"}
	if v := os.Getenv("NORIX_TELEMETRY"); v == "1" || v == "true" || v == "yes" {
		if os.Getenv("NORIX_TELEMETRY_URL") == "" {
			telemetryStatus.Detail = "enabled (no-op without URL)"
		} else {
			telemetryStatus.Detail = "enabled"
		}
	}
	components = append(components, telemetryStatus)

	overall := "operational"
	for _, c := range components {
		if c.Status == "down" {
			overall = "down"
			break
		}
		if c.Status == "degraded" && overall == "operational" {
			overall = "degraded"
		}
	}
	return Report{
		Status:     overall,
		Version:    version,
		PublicURL:  strings.TrimRight(strings.TrimSpace(os.Getenv("NORIX_PUBLIC_URL")), "/"),
		CheckedAt:  time.Now().UTC(),
		Components: components,
	}
}
