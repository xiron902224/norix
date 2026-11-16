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

func signGitHubBody(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func TestVerifyGitHubSignature(t *testing.T) {
	body := []byte(`{"ref":"refs/heads/main"}`)
	secret := "webhook-secret-token"

	if !verifyGitHubSignature(secret, signGitHubBody(secret, body), body) {
		t.Fatal("valid signature rejected")
	}
	if verifyGitHubSignature(secret, signGitHubBody("other", body), body) {
		t.Fatal("invalid signature accepted")
	}
	if verifyGitHubSignature(secret, "", body) {
		t.Fatal("missing signature accepted")
	}
	if verifyGitHubSignature(secret, "sha1=deadbeef", body) {
		t.Fatal("wrong algorithm accepted")
	}
	if verifyGitHubSignature("", signGitHubBody(secret, body), body) {
		t.Fatal("empty secret accepted")
	}
}

func TestGitHubWebhookRequiresSignature(t *testing.T) {
	// Unknown token path still 401 before signature; with nil pool we only assert
	// the handler rejects missing signature when token lookup is short-circuited.
	// Unit coverage for verifyGitHubSignature above; integration needs DB.
	handler := http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		token := "tok"
		body := []byte(`{}`)
		if !verifyGitHubSignature(token, request.Header.Get("X-Hub-Signature-256"), body) {
			http.Error(response, "invalid signature", http.StatusUnauthorized)
			return
		}
		response.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodPost, "/hooks/github/tok", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("missing sig status = %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodPost, "/hooks/github/tok", strings.NewReader(`{}`))
	req.Header.Set("X-Hub-Signature-256", signGitHubBody("tok", []byte(`{}`)))
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("valid sig status = %d", rec.Code)
	}
}
