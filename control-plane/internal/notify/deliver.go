// Package notify turns a stored event into a Slack, Discord, generic webhook,
// or email message and sends it (#27).
package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/smtp"
	"os"
	"strings"
	"time"

	"github.com/xiron902224/norix/control-plane/internal/gitssrf"
)

const maxAttempts = 6

// Summary is the one-line text used by Slack, Discord, and email.
func Summary(payload map[string]string) string {
	event := payload["event"]
	switch event {
	case "node.down":
		return fmt.Sprintf("[norix] node.down %s: %s", payload["node_name"], payload["message"])
	default:
		return fmt.Sprintf("[norix] %s %s/%s: %s", event, payload["project_name"], payload["workload_name"], payload["message"])
	}
}

// Body returns the bytes to POST for a channel kind.
func Body(kind string, payload map[string]string) ([]byte, error) {
	switch kind {
	case "slack":
		return json.Marshal(map[string]string{"text": Summary(payload)})
	case "discord":
		return json.Marshal(map[string]string{"content": Summary(payload)})
	case "webhook":
		return json.Marshal(payload)
	default:
		return nil, fmt.Errorf("unsupported notification kind %q", kind)
	}
}

// NextAttempt is the backoff after a failed try. attempts is the count already used.
func NextAttempt(now time.Time, attempts int) time.Time {
	if attempts < 1 {
		attempts = 1
	}
	shift := attempts - 1
	if shift > 6 {
		shift = 6
	}
	return now.Add(30 * time.Second * time.Duration(1<<shift))
}

// GaveUp reports whether a failed delivery should stop retrying.
func GaveUp(attempts int) bool {
	return attempts >= maxAttempts
}

// Deliver sends one notification. policy is the SSRF check for webhook URLs.
func Deliver(ctx context.Context, kind, target string, payload map[string]string, policy gitssrf.Policy) error {
	switch kind {
	case "email":
		return sendEmail(target, Summary(payload), emailBody(payload))
	case "webhook", "slack", "discord":
		body, err := Body(kind, payload)
		if err != nil {
			return err
		}
		return postJSON(ctx, target, body, policy)
	default:
		return fmt.Errorf("unsupported notification kind %q", kind)
	}
}

func emailBody(payload map[string]string) string {
	var b strings.Builder
	b.WriteString(Summary(payload))
	b.WriteString("\n\n")
	for _, key := range []string{"event", "project_name", "workload_name", "node_name", "message"} {
		if payload[key] == "" {
			continue
		}
		b.WriteString(key)
		b.WriteString(": ")
		b.WriteString(payload[key])
		b.WriteString("\n")
	}
	return b.String()
}

func postJSON(ctx context.Context, target string, body []byte, policy gitssrf.Policy) error {
	if err := policy.ValidateWebhookURL(target); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build request")
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{
		Timeout: 8 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			// A redirect can point at a private address after the first check.
			return fmt.Errorf("redirects are not followed")
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("request failed")
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1024))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webhook returned %d", resp.StatusCode)
	}
	return nil
}

func sendEmail(to, subject, body string) error {
	addr := strings.TrimSpace(os.Getenv("NORIX_SMTP_ADDR"))
	from := strings.TrimSpace(os.Getenv("NORIX_SMTP_FROM"))
	if addr == "" || from == "" {
		return fmt.Errorf("email notifications need NORIX_SMTP_ADDR and NORIX_SMTP_FROM")
	}
	msg := strings.Join([]string{
		"From: " + from,
		"To: " + to,
		"Subject: " + subject,
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=UTF-8",
		"",
		body,
	}, "\r\n")
	var auth smtp.Auth
	user := os.Getenv("NORIX_SMTP_USER")
	if user != "" {
		host := addr
		if i := strings.LastIndex(addr, ":"); i > 0 {
			host = addr[:i]
		}
		auth = smtp.PlainAuth("", user, os.Getenv("NORIX_SMTP_PASSWORD"), host)
	}
	if err := smtp.SendMail(addr, auth, from, []string{to}, []byte(msg)); err != nil {
		return fmt.Errorf("send email")
	}
	return nil
}
