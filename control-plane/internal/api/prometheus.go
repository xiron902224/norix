package api

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/xiron902224/norix/control-plane/internal/db"
)

// PrometheusMetricsHandler exposes recent metric_samples as Prometheus text (optional).
func PrometheusMetricsHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		orgID := strings.TrimSpace(request.URL.Query().Get("org_id"))
		if orgID == "" {
			http.Error(response, "org_id query required", http.StatusBadRequest)
			return
		}
		samples, err := db.ListMetricSamples(request.Context(), pool, orgID, "", "", 50)
		if err != nil {
			http.Error(response, "metrics error", http.StatusInternalServerError)
			return
		}
		var builder strings.Builder
		for _, sample := range samples {
			name := sanitizePromName(sample.Name)
			builder.WriteString("# TYPE ")
			builder.WriteString(name)
			builder.WriteString(" gauge\n")
			builder.WriteString(name)
			builder.WriteString("{node_id=\"")
			builder.WriteString(sample.NodeID)
			builder.WriteString("\"}")
			builder.WriteString(" ")
			builder.WriteString(fmt.Sprintf("%g", sample.Value))
			builder.WriteString("\n")
		}
		response.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		response.WriteHeader(http.StatusOK)
		_, _ = response.Write([]byte(builder.String()))
	}
}

func sanitizePromName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "norix_metric"
	}
	startsWithDigit := name[0] >= '0' && name[0] <= '9'
	name = strings.ReplaceAll(name, "-", "_")
	name = strings.ReplaceAll(name, ".", "_")
	name = strings.ReplaceAll(name, " ", "_")
	var b strings.Builder
	for i, ch := range name {
		ok := (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || ch == '_' ||
			(i > 0 && ch >= '0' && ch <= '9')
		if ok {
			b.WriteRune(ch)
		} else if i > 0 {
			b.WriteByte('_')
		}
	}
	out := b.String()
	if out == "" || startsWithDigit {
		return "norix_metric"
	}
	return out
}
