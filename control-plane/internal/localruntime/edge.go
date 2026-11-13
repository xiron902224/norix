package localruntime

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/xiron902224/norix/control-plane/internal/db"
)

type edgeState struct {
	LastGeneration int64  `json:"lastGeneration"`
	LastConfig     string `json:"lastConfig"`
}

func (runtime *Runtime) ApplyEdge(ctx context.Context, plan db.DesiredEdgePlan) db.ObservedEdge {
	logs := []string{fmt.Sprintf("edge generation %d routes=%d", plan.Generation, len(plan.Routes))}
	admin := plan.CaddyAdminURL
	if admin == "" {
		admin = db.CaddyAdminURL()
	}

	if err := pingCaddy(ctx, admin); err != nil {
		return db.ObservedEdge{
			Generation: plan.Generation,
			Status:     "failed",
			Message:    fmt.Sprintf("caddy admin unreachable at %s: %v", admin, err),
			LogLines:   append(logs, err.Error()),
		}
	}

	statePath := edgeStatePath()
	prev := readEdgeState(statePath)
	previousConfig, _ := fetchCaddyConfig(ctx, admin)
	if previousConfig == "" {
		previousConfig = prev.LastConfig
	}

	configJSON, routeStatuses, buildLogs, err := buildCaddyConfig(plan)
	logs = append(logs, buildLogs...)
	if err != nil {
		return db.ObservedEdge{
			Generation:    plan.Generation,
			Status:        "failed",
			Message:       err.Error(),
			LogLines:      logs,
			RouteStatuses: routeStatuses,
		}
	}

	if err := loadCaddyConfig(ctx, admin, configJSON); err != nil {
		logs = append(logs, "load failed: "+err.Error())
		if previousConfig != "" {
			_ = loadCaddyConfig(ctx, admin, previousConfig)
			logs = append(logs, "rolled back previous caddy config")
			return db.ObservedEdge{
				Generation:    plan.Generation,
				Status:        "rolled_back",
				Message:       err.Error(),
				LogLines:      logs,
				RouteStatuses: failAll(plan, err.Error()),
			}
		}
		return db.ObservedEdge{
			Generation:    plan.Generation,
			Status:        "failed",
			Message:       err.Error(),
			LogLines:      logs,
			RouteStatuses: failAll(plan, err.Error()),
		}
	}

	// Health-check upserted upstreams; rollback if any fail.
	for _, route := range plan.Routes {
		if route.Action != "upsert" || route.Upstream == "" {
			continue
		}
		if err := healthCheck(ctx, healthUpstreamURL(route.Upstream)); err != nil {
			logs = append(logs, fmt.Sprintf("health check failed for %s → %s: %v", route.Hostname, route.Upstream, err))
			if previousConfig != "" {
				_ = loadCaddyConfig(ctx, admin, previousConfig)
				logs = append(logs, "rolled back previous caddy config after health failure")
				return db.ObservedEdge{
					Generation:    plan.Generation,
					Status:        "rolled_back",
					Message:       err.Error(),
					LogLines:      logs,
					RouteStatuses: failAll(plan, err.Error()),
				}
			}
			routeStatuses[route.DomainID] = "failed"
			return db.ObservedEdge{
				Generation:    plan.Generation,
				Status:        "failed",
				Message:       err.Error(),
				LogLines:      logs,
				RouteStatuses: routeStatuses,
			}
		}
		routeStatuses[route.DomainID] = "active"
		logs = append(logs, fmt.Sprintf("healthy %s → %s", route.Hostname, route.Upstream))
	}

	writeEdgeState(statePath, edgeState{LastGeneration: plan.Generation, LastConfig: configJSON})
	logs = append(logs, "edge config active")
	return db.ObservedEdge{
		Generation:    plan.Generation,
		Status:        "active",
		LogLines:      logs,
		RouteStatuses: routeStatuses,
	}
}

func buildCaddyConfig(plan db.DesiredEdgePlan) (string, map[string]string, []string, error) {
	logs := []string{}
	routeStatuses := map[string]string{}
	apps := map[string]any{
		"http": map[string]any{
			"servers": map[string]any{
				"norix": map[string]any{
					"listen": []string{":80", ":443"},
					"routes": []any{},
				},
			},
		},
	}
	routes := make([]any, 0)
	hasACME := false

	for _, route := range plan.Routes {
		if route.Action == "delete" {
			routeStatuses[route.DomainID] = "deleted"
			logs = append(logs, "delete "+route.Hostname)
			continue
		}
		if route.Upstream == "" {
			routeStatuses[route.DomainID] = "failed"
			logs = append(logs, "skip "+route.Hostname+" (no upstream port yet)")
			continue
		}
		handler := map[string]any{
			"handle": []any{
				map[string]any{
					"handler": "reverse_proxy",
					"upstreams": []any{
						map[string]any{"dial": strings.TrimPrefix(strings.TrimPrefix(route.Upstream, "http://"), "https://")},
					},
				},
			},
			"match": []any{
				map[string]any{"host": []string{route.Hostname}},
			},
		}
		if route.TLSMode == "acme" {
			hasACME = true
			handler["terminal"] = true
		}
		routes = append(routes, handler)
		routeStatuses[route.DomainID] = "provisioning"
		logs = append(logs, fmt.Sprintf("route %s tls=%s upstream=%s", route.Hostname, route.TLSMode, route.Upstream))
	}

	// Always keep a default respond so empty configs are valid.
	routes = append(routes, map[string]any{
		"handle": []any{
			map[string]any{"handler": "static_response", "body": "norix edge", "status_code": 200},
		},
	})
	apps["http"].(map[string]any)["servers"].(map[string]any)["norix"].(map[string]any)["routes"] = routes

	tlsHostsInternal := []string{}
	tlsHostsACME := []string{}
	for _, route := range plan.Routes {
		if route.Action != "upsert" || route.Upstream == "" {
			continue
		}
		if route.TLSMode == "acme" {
			tlsHostsACME = append(tlsHostsACME, route.Hostname)
		} else {
			tlsHostsInternal = append(tlsHostsInternal, route.Hostname)
		}
	}
	automationPolicies := []any{}
	if len(tlsHostsInternal) > 0 {
		automationPolicies = append(automationPolicies, map[string]any{
			"subjects": tlsHostsInternal,
			"issuers":  []any{map[string]any{"module": "internal"}},
		})
	}
	if len(tlsHostsACME) > 0 {
		issuer := map[string]any{"module": "acme"}
		if plan.ACMECA != "" {
			issuer["ca"] = plan.ACMECA
		}
		automationPolicies = append(automationPolicies, map[string]any{
			"subjects": tlsHostsACME,
			"issuers":  []any{issuer},
		})
		hasACME = true
	}
	if len(automationPolicies) > 0 {
		apps["tls"] = map[string]any{
			"automation": map[string]any{"policies": automationPolicies},
		}
	}
	_ = hasACME

	// Keep admin on all interfaces so host-published :2019 keeps working after /load.
	payload := map[string]any{
		"admin": map[string]any{"listen": "0.0.0.0:2019"},
		"apps":  apps,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", routeStatuses, logs, err
	}
	return string(raw), routeStatuses, logs, nil
}

func failAll(plan db.DesiredEdgePlan, message string) map[string]string {
	out := map[string]string{}
	for _, route := range plan.Routes {
		if route.Action == "delete" {
			// Keep deleting so the next reconcile can finish removal after rollback.
			continue
		}
		out[route.DomainID] = "failed"
	}
	_ = message
	return out
}

func pingCaddy(ctx context.Context, admin string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, admin+"/config/", nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("status %d", resp.StatusCode)
	}
	return nil
}

func fetchCaddyConfig(ctx context.Context, admin string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, admin+"/config/", nil)
	if err != nil {
		return "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("status %d", resp.StatusCode)
	}
	return string(body), nil
}

func loadCaddyConfig(ctx context.Context, admin, configJSON string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, admin+"/load", bytes.NewBufferString(configJSON))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return fmt.Errorf("load status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

func healthUpstreamURL(upstream string) string {
	return strings.Replace(upstream, "host.docker.internal", "127.0.0.1", 1)
}

// CaddyMissingHosts reports whether the live Caddy config lacks any of the
// expected hostnames (e.g. after a compose recreate left the bootstrap Caddyfile).
func CaddyMissingHosts(ctx context.Context, admin string, hosts []string) bool {
	if admin == "" {
		admin = db.CaddyAdminURL()
	}
	if len(hosts) == 0 {
		return false
	}
	cfg, err := fetchCaddyConfig(ctx, admin)
	if err != nil || cfg == "" {
		return true
	}
	for _, host := range hosts {
		host = strings.TrimSpace(host)
		if host == "" {
			continue
		}
		if !strings.Contains(cfg, host) {
			return true
		}
	}
	return false
}

func healthCheck(ctx context.Context, upstream string) error {
	client := &http.Client{Timeout: 3 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, upstream, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 500 {
		return fmt.Errorf("upstream status %d", resp.StatusCode)
	}
	return nil
}

func edgeStatePath() string {
	dir := os.Getenv("NORIX_EDGE_STATE_DIR")
	if dir == "" {
		dir = filepath.Join(os.TempDir(), "norix-edge")
	}
	_ = os.MkdirAll(dir, 0o700)
	return filepath.Join(dir, "state.json")
}

func readEdgeState(path string) edgeState {
	raw, err := os.ReadFile(path)
	if err != nil {
		return edgeState{}
	}
	var state edgeState
	_ = json.Unmarshal(raw, &state)
	return state
}

func writeEdgeState(path string, state edgeState) {
	raw, _ := json.Marshal(state)
	_ = os.WriteFile(path, raw, 0o600)
}
