package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// PostOrUpdatePRComment creates or updates a PR comment with the preview URL (#21).
// Returns the GitHub comment id. Requires NORIX_GITHUB_TOKEN with issues:write.
func PostOrUpdatePRComment(ctx context.Context, repo string, prNumber int32, existingCommentID int64, body string) (int64, error) {
	token := strings.TrimSpace(os.Getenv("NORIX_GITHUB_TOKEN"))
	if token == "" {
		return 0, fmt.Errorf("NORIX_GITHUB_TOKEN is not set")
	}
	repo = strings.TrimSpace(repo)
	if repo == "" || prNumber < 1 {
		return 0, fmt.Errorf("repo and pr number are required")
	}
	api := strings.TrimRight(strings.TrimSpace(os.Getenv("NORIX_GITHUB_API")), "/")
	if api == "" {
		api = "https://api.github.com"
	}
	client := &http.Client{Timeout: 10 * time.Second}
	payload, _ := json.Marshal(map[string]string{"body": body})

	if existingCommentID > 0 {
		req, err := http.NewRequestWithContext(ctx, http.MethodPatch,
			fmt.Sprintf("%s/repos/%s/issues/comments/%d", api, repo, existingCommentID),
			bytes.NewReader(payload))
		if err != nil {
			return 0, err
		}
		setGitHubHeaders(req, token)
		resp, err := client.Do(req)
		if err != nil {
			return 0, err
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return existingCommentID, nil
		}
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 512))
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		fmt.Sprintf("%s/repos/%s/issues/%d/comments", api, repo, prNumber),
		bytes.NewReader(payload))
	if err != nil {
		return 0, err
	}
	setGitHubHeaders(req, token)
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return 0, fmt.Errorf("github comment returned %d", resp.StatusCode)
	}
	var out struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(raw, &out); err != nil || out.ID == 0 {
		return 0, fmt.Errorf("github comment response invalid")
	}
	return out.ID, nil
}

func setGitHubHeaders(req *http.Request, token string) {
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "norix-control-plane")
}
