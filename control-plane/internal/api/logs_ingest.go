package api

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/xiron902224/norix/control-plane/internal/db"
)

type logIngestRequest struct {
	ProjectID  string   `json:"project_id"`
	WorkloadID string   `json:"workload_id"`
	Level      string   `json:"level"`
	Lines      []string `json:"lines"`
}

// OpsLogsIngestHandler accepts batched log lines (agent or scripts) into log_segments (+ workload_logs when workload_id is set).
func OpsLogsIngestHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			http.Error(response, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		identity, err := sessionFromRequest(request, pool)
		if err != nil {
			http.Error(response, err.Error(), http.StatusUnauthorized)
			return
		}
		body, err := io.ReadAll(io.LimitReader(request.Body, 2<<20))
		if err != nil {
			http.Error(response, "read body", http.StatusBadRequest)
			return
		}
		var payload logIngestRequest
		if err := json.Unmarshal(body, &payload); err != nil {
			http.Error(response, "invalid json", http.StatusBadRequest)
			return
		}
		if len(payload.Lines) == 0 {
			http.Error(response, "lines required", http.StatusBadRequest)
			return
		}
		if len(payload.Lines) > 500 {
			payload.Lines = payload.Lines[:500]
		}
		level := strings.ToLower(strings.TrimSpace(payload.Level))
		inserted, err := db.InsertLogSegmentsBatch(request.Context(), pool, identity.OrganizationID, payload.ProjectID, payload.WorkloadID, payload.Lines, level)
		if err != nil {
			http.Error(response, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(response, map[string]any{"ingested": inserted})
	}
}
