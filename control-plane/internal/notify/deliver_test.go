package notify

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/xiron902224/norix/control-plane/internal/gitssrf"
)

func TestSummaryAndBody(t *testing.T) {
	payload := map[string]string{
		"event":         "deploy.failed",
		"project_name":  "shop",
		"workload_name": "web",
		"message":       "container exited",
	}
	if got := Summary(payload); !strings.Contains(got, "shop/web") || !strings.Contains(got, "deploy.failed") {
		t.Fatalf("summary %q", got)
	}
	slack, err := Body("slack", payload)
	if err != nil || !strings.Contains(string(slack), `"text"`) {
		t.Fatalf("slack %s %v", slack, err)
	}
	discord, err := Body("discord", payload)
	if err != nil || !strings.Contains(string(discord), `"content"`) {
		t.Fatalf("discord %s %v", discord, err)
	}
	raw, err := Body("webhook", payload)
	if err != nil || !strings.Contains(string(raw), `"workload_name":"web"`) {
		t.Fatalf("webhook %s %v", raw, err)
	}
	node := Summary(map[string]string{"event": "node.down", "node_name": "edge-1", "message": "timeout"})
	if !strings.Contains(node, "edge-1") {
		t.Fatalf("node summary %q", node)
	}
}

func TestBackoffAndGiveUp(t *testing.T) {
	now := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
	if got := NextAttempt(now, 1).Sub(now); got != 30*time.Second {
		t.Fatalf("first backoff %s", got)
	}
	if got := NextAttempt(now, 3).Sub(now); got != 120*time.Second {
		t.Fatalf("third backoff %s", got)
	}
	if GaveUp(5) || !GaveUp(6) {
		t.Fatal("give up threshold")
	}
}

func TestDeliverWebhookRejectsPrivateAndPostsJSON(t *testing.T) {
	policy := gitssrf.Policy{}
	err := Deliver(context.Background(), "webhook", "http://127.0.0.1/hook", map[string]string{"event": "node.down"}, policy)
	if err == nil || !strings.Contains(err.Error(), "loopback") {
		t.Fatalf("private url err=%v", err)
	}

	var gotBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		gotBody = string(raw)
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("content type %s", r.Header.Get("Content-Type"))
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)

	host, _, err := net.SplitHostPort(strings.TrimPrefix(server.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	allowed := gitssrf.Policy{Allowlist: []string{host}}
	if err := Deliver(context.Background(), "slack", server.URL, map[string]string{
		"event": "node.down", "node_name": "n1", "message": "down",
	}, allowed); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(gotBody, `"text"`) || !strings.Contains(gotBody, "n1") {
		t.Fatalf("body %s", gotBody)
	}
}

func TestEmailWithoutSMTPFailsClosed(t *testing.T) {
	t.Setenv("NORIX_SMTP_ADDR", "")
	t.Setenv("NORIX_SMTP_FROM", "")
	err := Deliver(context.Background(), "email", "ops@example.com", map[string]string{"event": "node.down"}, gitssrf.Policy{})
	if err == nil || !strings.Contains(err.Error(), "NORIX_SMTP_ADDR") {
		t.Fatalf("err=%v", err)
	}
}
