package api

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/xiron902224/norix/control-plane/internal/db"
	"github.com/xiron902224/norix/control-plane/internal/gitssrf"
	"github.com/xiron902224/norix/control-plane/internal/notify"
)

type githubPREvent struct {
	Action string `json:"action"`
	Number int32  `json:"number"`
	Repo   struct {
		FullName string `json:"full_name"`
		CloneURL string `json:"clone_url"`
		SSHURL   string `json:"ssh_url"`
	} `json:"repository"`
	PullRequest struct {
		Number int32 `json:"number"`
		Head   struct {
			Ref string `json:"ref"`
			SHA string `json:"sha"`
		} `json:"head"`
		HTMLURL string `json:"html_url"`
	} `json:"pull_request"`
}

// GitHubPreviewWebhook handles POST /api/webhooks/github (#21).
// On pull_request opened/synchronize/reopened it upserts a git workload, queues a
// build, sets a unique preview URL, and comments on the PR when NORIX_GITHUB_TOKEN
// is set. closed (or TTL GC) destroys the preview and deletes the workload.
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
		secret := strings.TrimSpace(os.Getenv("NORIX_GITHUB_WEBHOOK_SECRET"))
		if secret == "" {
			http.Error(response, "NORIX_GITHUB_WEBHOOK_SECRET is required", http.StatusServiceUnavailable)
			return
		}
		if !verifyGitHubSignature(secret, request.Header.Get("X-Hub-Signature-256"), body) {
			http.Error(response, "invalid signature", http.StatusUnauthorized)
			return
		}
		event := strings.TrimSpace(request.Header.Get("X-GitHub-Event"))
		slog.Info("github webhook", "event", event, "bytes", len(body))

		if event != "" && event != "pull_request" && event != "ping" {
			response.Header().Set("Content-Type", "application/json")
			response.WriteHeader(http.StatusAccepted)
			_, _ = response.Write([]byte(`{"ok":true,"ignored":true}`))
			return
		}
		if event == "ping" {
			response.Header().Set("Content-Type", "application/json")
			response.WriteHeader(http.StatusAccepted)
			_, _ = response.Write([]byte(`{"ok":true}`))
			return
		}

		var payload githubPREvent
		if err := json.Unmarshal(body, &payload); err != nil {
			http.Error(response, "invalid json", http.StatusBadRequest)
			return
		}
		if payload.Number == 0 {
			payload.Number = payload.PullRequest.Number
		}

		orgID := strings.TrimSpace(os.Getenv("NORIX_PREVIEW_ORG_ID"))
		projectID := strings.TrimSpace(os.Getenv("NORIX_PREVIEW_PROJECT_ID"))
		if orgID == "" || projectID == "" || payload.Number <= 0 || payload.Repo.FullName == "" {
			response.Header().Set("Content-Type", "application/json")
			response.WriteHeader(http.StatusAccepted)
			_, _ = response.Write([]byte(`{"ok":true,"skipped":"preview org/project not configured"}`))
			return
		}

		ctx := request.Context()
		result := map[string]any{"ok": true, "action": payload.Action, "pr": payload.Number}

		switch payload.Action {
		case "closed":
			if err := destroyPreview(ctx, pool, orgID, projectID, payload); err != nil {
				slog.Warn("preview destroy", "error", err)
				result["error"] = err.Error()
			} else {
				result["status"] = "destroyed"
			}
		case "opened", "reopened", "synchronize", "ready_for_review":
			out, err := deployPreview(ctx, pool, orgID, projectID, payload)
			if err != nil {
				slog.Warn("preview deploy", "error", err)
				result["error"] = err.Error()
			} else {
				result["status"] = out.Status
				result["preview_url"] = out.PreviewURL
				result["workload_id"] = out.WorkloadID
				result["build_id"] = out.BuildID
			}
		default:
			result["ignored"] = true
		}

		response.Header().Set("Content-Type", "application/json")
		response.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(response).Encode(result)
	}
}

type previewDeployResult struct {
	Status     string
	PreviewURL string
	WorkloadID string
	BuildID    string
}

func deployPreview(ctx context.Context, pool *pgxpool.Pool, orgID, projectID string, payload githubPREvent) (previewDeployResult, error) {
	gitURL := strings.TrimSpace(payload.Repo.CloneURL)
	if gitURL == "" {
		gitURL = strings.TrimSpace(payload.Repo.SSHURL)
	}
	if gitURL == "" {
		return previewDeployResult{}, fmt.Errorf("repository clone url missing")
	}
	if err := gitssrf.ValidateGitURL(gitURL); err != nil {
		return previewDeployResult{}, err
	}
	ref := strings.TrimSpace(payload.PullRequest.Head.SHA)
	if ref == "" {
		ref = strings.TrimSpace(payload.PullRequest.Head.Ref)
	}
	if ref == "" {
		ref = "main"
	}
	name := db.PreviewWorkloadName(payload.Number)
	host := db.PreviewHostname(payload.Repo.FullName, payload.Number)
	previewURL := db.PreviewPublicURL(host)
	port := previewContainerPort()
	dockerfile := previewDockerfile()

	workload, err := db.FindWorkloadByName(ctx, pool, orgID, projectID, name)
	if err != nil {
		workload, err = db.CreateWorkload(ctx, pool, orgID, db.Workload{
			ProjectID:      projectID,
			Name:           name,
			Runtime:        "container",
			SourceKind:     "git",
			GitURL:         gitURL,
			GitRef:         ref,
			DockerfilePath: dockerfile,
			ImageOrSource:  "pending-build",
			ContainerPort:  port,
			Domain:         host,
			ReplicaCount:   1,
			CPUShares:      1024,
			MemoryLimitMB:  512,
		}, db.AuditActor{Email: "github-preview-webhook"})
		if err != nil {
			return previewDeployResult{}, err
		}
	} else {
		workload.GitURL = gitURL
		workload.GitRef = ref
		workload.DockerfilePath = dockerfile
		workload.ContainerPort = port
		workload.Domain = host
		workload.SourceKind = "git"
		if _, err := db.UpdateWorkload(ctx, pool, orgID, workload, db.AuditActor{Email: "github-preview-webhook"}); err != nil {
			return previewDeployResult{}, err
		}
	}

	build, err := db.CreateBuild(
		ctx, pool, orgID, workload.ID, gitURL, ref, dockerfile, port, nil,
		db.AuditActor{Email: "github-preview-webhook"},
	)
	if err != nil {
		return previewDeployResult{}, err
	}

	ttl := previewTTLSeconds()
	stored, err := db.UpsertPRPreview(ctx, pool, orgID, db.PRPreview{
		ProjectID:  projectID,
		Repo:       payload.Repo.FullName,
		PRNumber:   payload.Number,
		Status:     "building",
		PreviewURL: previewURL,
		TTLSeconds: ttl,
	})
	if err != nil {
		return previewDeployResult{}, err
	}

	commentBody := fmt.Sprintf(
		"## Norix preview\n\nPreview URL: %s\n\nStatus: building (build `%s`)\n\nUpdated: %s",
		previewURL, build.ID, time.Now().UTC().Format(time.RFC3339),
	)
	if commentID, commentErr := notify.PostOrUpdatePRComment(ctx, payload.Repo.FullName, payload.Number, stored.GithubCommentID, commentBody); commentErr != nil {
		slog.Warn("preview github comment", "error", commentErr)
	} else if commentID > 0 && commentID != stored.GithubCommentID {
		stored.GithubCommentID = commentID
		_, _ = db.UpsertPRPreview(ctx, pool, orgID, stored)
	}

	return previewDeployResult{
		Status:     "building",
		PreviewURL: previewURL,
		WorkloadID: workload.ID,
		BuildID:    build.ID,
	}, nil
}

func destroyPreview(ctx context.Context, pool *pgxpool.Pool, orgID, projectID string, payload githubPREvent) error {
	name := db.PreviewWorkloadName(payload.Number)
	if workload, err := db.FindWorkloadByName(ctx, pool, orgID, projectID, name); err == nil {
		_ = db.DeleteWorkload(ctx, pool, orgID, workload.ID, db.AuditActor{Email: "github-preview-webhook"})
	}
	stored, err := db.UpsertPRPreview(ctx, pool, orgID, db.PRPreview{
		ProjectID:  projectID,
		Repo:       payload.Repo.FullName,
		PRNumber:   payload.Number,
		Status:     "destroyed",
		PreviewURL: "",
		TTLSeconds: previewTTLSeconds(),
	})
	if err != nil {
		return err
	}
	if stored.GithubCommentID > 0 {
		body := fmt.Sprintf("## Norix preview\n\nPreview destroyed (PR closed or TTL).\n\nUpdated: %s", time.Now().UTC().Format(time.RFC3339))
		_, _ = notify.PostOrUpdatePRComment(ctx, payload.Repo.FullName, payload.Number, stored.GithubCommentID, body)
	}
	return nil
}

func previewContainerPort() int32 {
	raw := strings.TrimSpace(os.Getenv("NORIX_PREVIEW_CONTAINER_PORT"))
	if raw == "" {
		return 80
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v < 1 || v > 65535 {
		return 80
	}
	return int32(v)
}

func previewDockerfile() string {
	path := strings.TrimSpace(os.Getenv("NORIX_PREVIEW_DOCKERFILE"))
	if path == "" {
		return "Dockerfile"
	}
	return path
}

func previewTTLSeconds() int32 {
	raw := strings.TrimSpace(os.Getenv("NORIX_PREVIEW_TTL_SECONDS"))
	if raw == "" {
		return 86400
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return 86400
	}
	if v < 60 {
		return 60
	}
	if v > 604800 {
		return 604800
	}
	return int32(v)
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
