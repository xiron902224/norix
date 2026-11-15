package api

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/xiron902224/norix/control-plane/internal/db"
)

func OpsAuditExportHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			http.Error(response, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		identity, err := sessionFromRequest(request, pool)
		if err != nil {
			http.Error(response, err.Error(), http.StatusUnauthorized)
			return
		}
		format := strings.ToLower(strings.TrimSpace(request.URL.Query().Get("format")))
		if format == "" {
			format = "json"
		}
		events, listErr := db.ListAuditEvents(request.Context(), pool, identity.OrganizationID, 10000)
		if listErr != nil {
			http.Error(response, listErr.Error(), http.StatusInternalServerError)
			return
		}
		switch format {
		case "csv":
			response.Header().Set("Content-Type", "text/csv; charset=utf-8")
			response.Header().Set("Content-Disposition", `attachment; filename="audit-export.csv"`)
			writer := csv.NewWriter(response)
			_ = writer.Write([]string{"id", "created_at", "actor_email", "action", "resource_type", "resource_id"})
			for _, event := range events {
				_ = writer.Write([]string{
					strconv.FormatInt(event.ID, 10),
					event.CreatedAt.Format(time.RFC3339),
					event.ActorEmail,
					event.Action,
					event.ResourceType,
					event.ResourceID,
				})
			}
			writer.Flush()
		default:
			response.Header().Set("Content-Type", "application/json")
			payload := make([]map[string]any, 0, len(events))
			for _, event := range events {
				payload = append(payload, map[string]any{
					"id":            event.ID,
					"created_at":    event.CreatedAt,
					"actor_user_id": event.ActorUserID,
					"actor_email":   event.ActorEmail,
					"action":        event.Action,
					"resource_type": event.ResourceType,
					"resource_id":   event.ResourceID,
					"metadata":      event.Metadata,
				})
			}
			_ = json.NewEncoder(response).Encode(map[string]any{"events": payload})
		}
	}
}

func OpsUsageExportHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			http.Error(response, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		identity, err := sessionFromRequest(request, pool)
		if err != nil {
			http.Error(response, err.Error(), http.StatusUnauthorized)
			return
		}
		format := strings.ToLower(strings.TrimSpace(request.URL.Query().Get("format")))
		if format == "" {
			format = "json"
		}
		records, listErr := db.ListUsageRecords(request.Context(), pool, identity.OrganizationID, 10000)
		if listErr != nil {
			http.Error(response, listErr.Error(), http.StatusInternalServerError)
			return
		}
		switch format {
		case "csv":
			response.Header().Set("Content-Type", "text/csv; charset=utf-8")
			response.Header().Set("Content-Disposition", `attachment; filename="usage-export.csv"`)
			writer := csv.NewWriter(response)
			_ = writer.Write([]string{"id", "recorded_at", "team_id", "metric", "quantity", "unit"})
			for _, item := range records {
				_ = writer.Write([]string{
					strconv.FormatInt(item.ID, 10),
					item.Recorded.Format(time.RFC3339),
					item.TeamID,
					item.Metric,
					fmt.Sprintf("%.4f", item.Quantity),
					item.Unit,
				})
			}
			writer.Flush()
		default:
			response.Header().Set("Content-Type", "application/json")
			payload := make([]map[string]any, 0, len(records))
			for _, item := range records {
				payload = append(payload, map[string]any{
					"id":          item.ID,
					"recorded_at": item.Recorded,
					"team_id":     item.TeamID,
					"metric":      item.Metric,
					"quantity":    item.Quantity,
					"unit":        item.Unit,
					"metadata":    item.Metadata,
				})
			}
			_ = json.NewEncoder(response).Encode(map[string]any{"records": payload})
		}
	}
}
