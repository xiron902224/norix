package db

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/xiron902224/norix/control-plane/internal/secrets"
)

// ComposeVolume is a named volume or bind mount reported by the agent (#147).
type ComposeVolume struct {
	Name   string `json:"name"`
	Kind   string `json:"kind"`
	Source string `json:"source,omitempty"`
}

func sealStackSecret(plain string) (ciphertext, nonce []byte, err error) {
	key, err := secrets.KeyFromEnv()
	if err != nil {
		if secrets.InsecureDevAllowed() {
			return []byte(plain), []byte(secrets.DevNonceMarker), nil
		}
		return nil, nil, fmt.Errorf("stack variable encryption requires NORIX_BUILD_SECRETS_KEY: %w", err)
	}
	return secrets.Encrypt(key, []byte(plain))
}

func openStackSecret(ciphertext, nonce []byte) (string, error) {
	if secrets.IsDevNonce(nonce) {
		return string(ciphertext), nil
	}
	key, err := secrets.KeyFromEnv()
	if err != nil {
		return "", err
	}
	plain, err := secrets.Decrypt(key, ciphertext, nonce)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

func writeStackSpec(ctx context.Context, tx pgx.Tx, stackID string, public []string, domains map[string]string) error {
	if public == nil {
		public = []string{}
	}
	if domains == nil {
		domains = map[string]string{}
	}
	publicJSON, err := json.Marshal(public)
	if err != nil {
		return err
	}
	domainsJSON, err := json.Marshal(domains)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		UPDATE compose_stacks
		SET public_services = $2::jsonb, domains = $3::jsonb, updated_at = NOW()
		WHERE id = $1::uuid`, stackID, string(publicJSON), string(domainsJSON))
	return err
}

// upsertStackVariables sets or deletes variables. An empty value deletes the name.
// Values are encrypted. Callers must not log them.
func upsertStackVariables(ctx context.Context, tx pgx.Tx, tenantID, stackID string, vars map[string]string) error {
	for name, value := range vars {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if strings.TrimSpace(value) == "" {
			if _, err := tx.Exec(ctx, `
				DELETE FROM compose_stack_variables
				WHERE stack_id = $1::uuid AND name = $2`, stackID, name); err != nil {
				return err
			}
			continue
		}
		ciphertext, nonce, err := sealStackSecret(value)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO compose_stack_variables (organization_id, stack_id, name, ciphertext, nonce, generated)
			VALUES ($1::uuid, $2::uuid, $3, $4, $5, FALSE)
			ON CONFLICT (stack_id, name) DO UPDATE
			SET ciphertext = EXCLUDED.ciphertext, nonce = EXCLUDED.nonce, generated = FALSE, updated_at = NOW()`,
			tenantID, stackID, name, ciphertext, nonce); err != nil {
			return err
		}
	}
	return nil
}

// persistGeneratedStackEnv inserts agent-generated secrets once. A later report
// of the same name does not rotate the stored value (#145).
func persistGeneratedStackEnv(ctx context.Context, tx pgx.Tx, tenantID, stackID string, vars map[string]string) error {
	for name, value := range vars {
		name = strings.TrimSpace(name)
		if name == "" || strings.TrimSpace(value) == "" {
			continue
		}
		if !strings.HasPrefix(name, "SERVICE_PASSWORD_") && !strings.HasPrefix(name, "SERVICE_BASE64_") {
			continue
		}
		ciphertext, nonce, err := sealStackSecret(value)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO compose_stack_variables (organization_id, stack_id, name, ciphertext, nonce, generated)
			VALUES ($1::uuid, $2::uuid, $3, $4, $5, TRUE)
			ON CONFLICT (stack_id, name) DO NOTHING`,
			tenantID, stackID, name, ciphertext, nonce); err != nil {
			return err
		}
	}
	return nil
}

func stackVariableNames(ctx context.Context, tx pgx.Tx, stackID string) ([]string, error) {
	rows, err := tx.Query(ctx, `
		SELECT name FROM compose_stack_variables WHERE stack_id = $1::uuid ORDER BY name`, stackID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	names := []string{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		names = append(names, name)
	}
	return names, rows.Err()
}

func decryptStackEnv(ctx context.Context, tx pgx.Tx, stackID string) (map[string]string, error) {
	rows, err := tx.Query(ctx, `
		SELECT name, ciphertext, nonce FROM compose_stack_variables WHERE stack_id = $1::uuid`, stackID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var name string
		var ciphertext, nonce []byte
		if err := rows.Scan(&name, &ciphertext, &nonce); err != nil {
			return nil, err
		}
		plain, err := openStackSecret(ciphertext, nonce)
		if err != nil {
			return nil, fmt.Errorf("decrypt stack variable %s: %w", name, err)
		}
		out[name] = plain
	}
	return out, rows.Err()
}

func fillStackExtras(ctx context.Context, tx pgx.Tx, items []ComposeStack, withSecrets bool) error {
	for i := range items {
		names, err := stackVariableNames(ctx, tx, items[i].ID)
		if err != nil {
			return err
		}
		items[i].VariableNames = names
		if !withSecrets {
			continue
		}
		env, err := decryptStackEnv(ctx, tx, items[i].ID)
		if err != nil {
			return err
		}
		items[i].Env = env
	}
	return nil
}

// ComposeEdgeRoutes returns Caddy routes for every stack domain on this node (#146).
// Caddy replaces its whole config on load, so every plan must carry all of them.
// The upstream is the service alias `<compose project>-<service>` on the project network.
// pending reports whether any stack on the node changed since the last edge plan.
func ComposeEdgeRoutes(ctx context.Context, tx pgx.Tx, nodeID string) (routes []DesiredEdgeRoute, pending bool, err error) {
	rows, err := tx.Query(ctx, `
		SELECT id::text, status, domains::text, services::text, edge_pending
		FROM compose_stacks
		WHERE assigned_node_id = $1::uuid`, nodeID)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	routes = []DesiredEdgeRoute{}
	for rows.Next() {
		var stackID, status, domainsJSON, servicesJSON string
		var edgePending bool
		if err := rows.Scan(&stackID, &status, &domainsJSON, &servicesJSON, &edgePending); err != nil {
			return nil, false, err
		}
		if edgePending {
			pending = true
		}
		var domains map[string]string
		if err := json.Unmarshal([]byte(domainsJSON), &domains); err != nil || len(domains) == 0 {
			continue
		}
		if status == "stopped" || status == "removing" {
			// Not running: leave it out of the config so Caddy stops routing to it.
			continue
		}
		var services []ComposeStackService
		_ = json.Unmarshal([]byte(servicesJSON), &services)
		ports := map[string]int32{}
		for _, svc := range services {
			if svc.ContainerPort > 0 {
				ports[svc.Name] = svc.ContainerPort
			}
		}
		project := ComposeProjectName(stackID)
		for service, hostname := range domains {
			hostname = NormalizeHostname(hostname)
			service = strings.TrimSpace(service)
			port := ports[service]
			if hostname == "" || service == "" || port <= 0 {
				continue
			}
			routes = append(routes, DesiredEdgeRoute{
				DomainID: "compose-" + stackID + "-" + service,
				Hostname: hostname,
				TLSMode:  ChooseTLSMode(hostname, ""),
				Upstream: fmt.Sprintf("http://%s-%s:%d", project, service, port),
				Action:   "upsert",
			})
		}
	}
	return routes, pending, rows.Err()
}

// clearComposeEdgePending marks the node's stacks as applied in the edge plan just built.
func clearComposeEdgePending(ctx context.Context, tx pgx.Tx, nodeID string) error {
	_, err := tx.Exec(ctx, `
		UPDATE compose_stacks SET edge_pending = FALSE
		WHERE assigned_node_id = $1::uuid AND edge_pending`, nodeID)
	return err
}
