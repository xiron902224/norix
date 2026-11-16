package api

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func signPreviewBody(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func TestGitHubPreviewWebhookRequiresSecret(t *testing.T) {
	t.Setenv("NORIX_GITHUB_WEBHOOK_SECRET", "")
	handler := GitHubPreviewWebhook(nil)

	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/github", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("unset secret status = %d, want 503", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "NORIX_GITHUB_WEBHOOK_SECRET") {
		t.Fatalf("body = %q", rec.Body.String())
	}
}

func TestGitHubPreviewWebhookRejectsBadSignature(t *testing.T) {
	t.Setenv("NORIX_GITHUB_WEBHOOK_SECRET", "preview-secret")
	handler := GitHubPreviewWebhook(nil)

	body := `{"action":"opened","number":1}`
	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/github", strings.NewReader(body))
	req.Header.Set("X-Hub-Signature-256", signPreviewBody("wrong", []byte(body)))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("bad sig status = %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodPost, "/api/webhooks/github", strings.NewReader(body))
	req.Header.Set("X-Hub-Signature-256", signPreviewBody("preview-secret", []byte(body)))
	req.Header.Set("X-GitHub-Event", "pull_request")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("valid sig status = %d body=%s", rec.Code, rec.Body.String())
	}
}