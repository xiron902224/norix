package api

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/xiron902224/norix/control-plane/internal/db"
)

type githubPREvent struct {
	Action string `json:"action"`
	Number int32  `json:"number"`
	Repo   struct {
		FullName string `json:"full_name"`
	} `json:"repository"`
}

// GitHubPreviewWebhook handles POST /api/webhooks/github (preview stub).
func GitHubPreviewWebhook(pool *pgxpool.Pool) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			http.Error(response, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		body, err := io.ReadAll(io.LimitReader(request.Body, 1<<20))
		if err != nil {
			http.Error(response, "read body", http.StatusBadRequest)
			return
		}
		if secret := strings.TrimSpace(os.Getenv("NORIX_GITHUB_WEBHOOK_SECRET")); secret != "" {
			if !verifyGitHubSignature(secret, request.Header.Get("X-Hub-Signature-256"), body) {
				http.Error(response, "invalid signature", http.StatusUnauthorized)
				return
			}
		}
		event := strings.TrimSpace(request.Header.Get("X-GitHub-Event"))
		slog.Info("github webhook", "event", event, "bytes", len(body))

		var payload githubPREvent
		_ = json.Unmarshal(body, &payload)

		orgID := strings.TrimSpace(os.Getenv("NORIX_PREVIEW_ORG_ID"))
		projectID := strings.TrimSpace(os.Getenv("NORIX_PREVIEW_PROJECT_ID"))
		if orgID != "" && projectID != "" && payload.Number > 0 && payload.Repo.FullName != "" {
			status := "pending"
			if payload.Action == "closed" {
				status = "destroyed"
			}
			_, _ = db.UpsertPRPreview(request.Context(), pool, orgID, db.PRPreview{
				ProjectID:  projectID,
				Repo:       payload.Repo.FullName,
				PRNumber:   payload.Number,
				Status:     status,
				PreviewURL: "",
			})
		}

		response.Header().Set("Content-Type", "application/json")
		response.WriteHeader(http.StatusAccepted)
		_, _ = response.Write([]byte(`{"ok":true}`))
	}
}

func verifyGitHubSignature(secret, signature string, body []byte) bool {
	signature = strings.TrimSpace(signature)
	if secret == "" || !strings.HasPrefix(signature, "sha256=") {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(body)
	expected := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(strings.TrimPrefix(signature, "sha256=")), []byte(expected))
}
