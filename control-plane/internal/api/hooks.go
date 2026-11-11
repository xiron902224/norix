package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/xiron902224/norix/control-plane/internal/auth"
	"github.com/xiron902224/norix/control-plane/internal/db"
)

type githubPushPayload struct {
	Ref        string `json:"ref"`
	Repository struct {
		CloneURL string `json:"clone_url"`
		HTMLURL  string `json:"html_url"`
		SSHURL   string `json:"ssh_url"`
	} `json:"repository"`
}

// GitHubWebhook handles POST /hooks/github/{token}
func GitHubWebhook(pool *pgxpool.Pool) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			http.Error(response, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		token := strings.TrimSpace(request.PathValue("token"))
		if token == "" {
			http.Error(response, "token required", http.StatusBadRequest)
			return
		}
		sum := sha256.Sum256([]byte(token))
		tokenHash := hex.EncodeToString(sum[:])
		orgID, workloadID, err := db.FindWebhookToken(request.Context(), pool, tokenHash)
		if err != nil {
			http.Error(response, "unknown webhook token", http.StatusUnauthorized)
			return
		}
		body, err := io.ReadAll(io.LimitReader(request.Body, 1<<20))
		if err != nil {
			http.Error(response, "read body", http.StatusBadRequest)
			return
		}
		var payload githubPushPayload
		if err := json.Unmarshal(body, &payload); err != nil {
			http.Error(response, "invalid json", http.StatusBadRequest)
			return
		}
		ref := strings.TrimPrefix(payload.Ref, "refs/heads/")
		if ref == "" {
			ref = "main"
		}
		workload, err := db.GetWorkload(request.Context(), pool, orgID, workloadID)
		if err != nil {
			http.Error(response, "workload not found", http.StatusNotFound)
			return
		}
		gitURL := payload.Repository.CloneURL
		if gitURL == "" {
			gitURL = workload.GitURL
		}
		build, err := db.CreateBuild(
			request.Context(),
			pool,
			orgID,
			workloadID,
			gitURL,
			ref,
			workload.DockerfilePath,
			workload.ContainerPort,
			nil,
			db.AuditActor{Email: "github-webhook"},
		)
		if err != nil {
			http.Error(response, err.Error(), http.StatusBadRequest)
			return
		}
		response.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(response).Encode(map[string]string{
			"build_id":    build.ID,
			"workload_id": build.WorkloadID,
			"status":      build.Status,
		})
	}
}

// CreateWebhookTokenHandler returns a token for GitHub push hooks.
func CreateWebhookTokenHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			http.Error(response, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		authHeader := request.Header.Get("Authorization")
		if !strings.HasPrefix(authHeader, "Bearer ") {
			http.Error(response, "Bearer token required", http.StatusUnauthorized)
			return
		}
		token := strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer "))
		identity, err := findActiveSession(request.Context(), pool, auth.HashSessionToken(token))
		if err != nil {
			http.Error(response, "session invalid", http.StatusUnauthorized)
			return
		}
		workloadID := strings.TrimSpace(request.PathValue("workloadID"))
		if workloadID == "" {
			http.Error(response, "workload id required", http.StatusBadRequest)
			return
		}
		plain, _, err := auth.NewSessionToken()
		if err != nil {
			http.Error(response, "token mint failed", http.StatusInternalServerError)
			return
		}
		sum := sha256.Sum256([]byte(plain))
		if err := db.EnsureWebhookToken(request.Context(), pool, identity.OrganizationID, workloadID, hex.EncodeToString(sum[:])); err != nil {
			http.Error(response, err.Error(), http.StatusBadRequest)
			return
		}
		response.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(response).Encode(map[string]string{
			"token":       plain,
			"webhook_url": "/hooks/github/" + plain,
		})
	}
}
