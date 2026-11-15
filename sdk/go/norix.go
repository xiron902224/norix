package norix

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client is a minimal Norix control-plane client (mirrors sdk/typescript).
type Client struct {
	BaseURL    string
	Token      string
	HTTPClient *http.Client
}

func New(baseURL, token string) *Client {
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		Token:   token,
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (c *Client) Health(ctx context.Context) (string, error) {
	body, err := c.get(ctx, "/healthz")
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func (c *Client) Status(ctx context.Context) (map[string]any, error) {
	body, err := c.get(ctx, "/status")
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) ListProjects(ctx context.Context) (any, error) {
	return c.connect(ctx, "v1.ProjectService/ListProjects", map[string]any{})
}

func (c *Client) ListWorkloads(ctx context.Context, projectID string) (any, error) {
	return c.connect(ctx, "v1.WorkloadService/ListWorkloads", map[string]any{"projectId": projectID})
}

func (c *Client) ListSandboxes(ctx context.Context, projectID string) (any, error) {
	return c.connect(ctx, "v1.SandboxService/ListSandboxes", map[string]any{"projectId": projectID})
}

func (c *Client) CreateSandbox(ctx context.Context, projectID, name, template string, ttlSeconds int) (any, error) {
	if template == "" {
		template = "blank"
	}
	if ttlSeconds <= 0 {
		ttlSeconds = 3600
	}
	return c.connect(ctx, "v1.SandboxService/CreateSandbox", map[string]any{
		"projectId":  projectID,
		"name":       name,
		"template":   template,
		"ttlSeconds": ttlSeconds,
	})
}

func (c *Client) QueryLogs(ctx context.Context, query string, limit int) (any, error) {
	if limit <= 0 {
		limit = 100
	}
	return c.connect(ctx, "v1.MetricsService/QueryLogs", map[string]any{"query": query, "limit": limit})
}

func (c *Client) get(ctx context.Context, path string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+path, nil)
	if err != nil {
		return nil, err
	}
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("GET %s failed: %d", path, resp.StatusCode)
	}
	return body, nil
}

func (c *Client) connect(ctx context.Context, procedure string, payload map[string]any) (any, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	url := c.BaseURL + "/api/" + procedure
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Connect-Protocol-Version", "1")
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("Norix API %s failed: %d", procedure, resp.StatusCode)
	}
	var out any
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, err
	}
	return out, nil
}
