package db

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/xiron902224/norix/control-plane/internal/secrets"
)

// EnvVarMaskedPlaceholder is returned by the API instead of plaintext env values (#8).
const EnvVarMaskedPlaceholder = "********"

const envEncVersionKey = "__norix_env"
const envEncVersion = 1

type envEncEntry struct {
	C string `json:"c"` // base64 ciphertext
	N string `json:"n"` // base64 nonce
}

type envEncEnvelope struct {
	Version int                     `json:"__norix_env"`
	Entries map[string]envEncEntry  `json:"entries"`
}

// MaskEnvironment replaces every value with EnvVarMaskedPlaceholder (keys preserved).
func MaskEnvironment(env map[string]string) map[string]string {
	if len(env) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(env))
	for k := range env {
		out[k] = EnvVarMaskedPlaceholder
	}
	return out
}

// MergeEnvironmentUpdate applies incoming env over existing. Values equal to
// EnvVarMaskedPlaceholder keep the previous plaintext (UI did not change them).
func MergeEnvironmentUpdate(existing, incoming map[string]string) map[string]string {
	if incoming == nil {
		incoming = map[string]string{}
	}
	out := make(map[string]string, len(incoming))
	for k, v := range incoming {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		if v == EnvVarMaskedPlaceholder {
			if existing != nil {
				if prev, ok := existing[k]; ok {
					out[k] = prev
					continue
				}
			}
			// Masked with no prior value — skip (do not store the placeholder).
			continue
		}
		out[k] = v
	}
	return out
}

// encodeEnvironmentJSON encrypts plaintext env for storage (AES-GCM via NORIX_BUILD_SECRETS_KEY).
// Empty maps stay {}. Without a secrets key and with insecure-dev allowed, stores plaintext.
func encodeEnvironmentJSON(plain map[string]string) ([]byte, error) {
	if len(plain) == 0 {
		return []byte(`{}`), nil
	}
	key, err := secrets.KeyFromEnv()
	if err != nil {
		if secrets.InsecureDevAllowed() {
			return mapToJSON(plain), nil
		}
		return nil, fmt.Errorf("workload env encryption requires NORIX_BUILD_SECRETS_KEY: %w", err)
	}
	entries := make(map[string]envEncEntry, len(plain))
	for name, value := range plain {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		ct, nonce, encErr := secrets.Encrypt(key, []byte(value))
		if encErr != nil {
			return nil, fmt.Errorf("encrypt env %q: %w", name, encErr)
		}
		entries[name] = envEncEntry{
			C: base64.StdEncoding.EncodeToString(ct),
			N: base64.StdEncoding.EncodeToString(nonce),
		}
	}
	env := envEncEnvelope{Version: envEncVersion, Entries: entries}
	return json.Marshal(env)
}

// decodeEnvironmentJSON loads plaintext env from DB JSON (encrypted envelope or legacy plaintext).
func decodeEnvironmentJSON(raw string) (map[string]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" || raw == "{}" {
		return map[string]string{}, nil
	}

	var probe map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &probe); err != nil {
		return nil, fmt.Errorf("parse env_vars: %w", err)
	}
	if _, ok := probe[envEncVersionKey]; ok {
		var env envEncEnvelope
		if err := json.Unmarshal([]byte(raw), &env); err != nil {
			return nil, fmt.Errorf("parse encrypted env_vars: %w", err)
		}
		key, err := secrets.KeyFromEnv()
		if err != nil {
			return nil, fmt.Errorf("decrypt env_vars: %w", err)
		}
		out := make(map[string]string, len(env.Entries))
		for name, entry := range env.Entries {
			ct, err := base64.StdEncoding.DecodeString(entry.C)
			if err != nil {
				return nil, fmt.Errorf("decode env ciphertext %q: %w", name, err)
			}
			nonce, err := base64.StdEncoding.DecodeString(entry.N)
			if err != nil {
				return nil, fmt.Errorf("decode env nonce %q: %w", name, err)
			}
			plain, err := secrets.Decrypt(key, ct, nonce)
			if err != nil {
				return nil, fmt.Errorf("decrypt env %q: %w", name, err)
			}
			out[name] = string(plain)
		}
		return out, nil
	}

	// Legacy plaintext JSONB map[string]string.
	var plain map[string]string
	if err := json.Unmarshal([]byte(raw), &plain); err != nil {
		return nil, fmt.Errorf("parse plaintext env_vars: %w", err)
	}
	if plain == nil {
		plain = map[string]string{}
	}
	return plain, nil
}

func isEncryptedEnvJSON(raw string) bool {
	var probe map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &probe); err != nil {
		return false
	}
	_, ok := probe[envEncVersionKey]
	return ok
}

// BackfillEncryptWorkloadEnvs re-encrypts legacy plaintext env_vars rows (#8).
// Safe to call repeatedly; skips already-encrypted envelopes. Requires secrets key
// unless there are no plaintext rows with values.
func BackfillEncryptWorkloadEnvs(ctx context.Context, pool *pgxpool.Pool) (int, error) {
	rows, err := pool.Query(ctx, `
		SELECT id::text, env_vars::text
		FROM workloads
		WHERE env_vars IS NOT NULL AND env_vars::text <> '{}' AND env_vars::text <> 'null'`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	type row struct {
		id  string
		raw string
	}
	var pending []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.raw); err != nil {
			return 0, err
		}
		if isEncryptedEnvJSON(r.raw) {
			continue
		}
		pending = append(pending, r)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	if len(pending) == 0 {
		return 0, nil
	}

	if _, err := secrets.KeyFromEnv(); err != nil {
		if secrets.InsecureDevAllowed() {
			return 0, nil
		}
		return 0, fmt.Errorf("backfill workload env encryption: %w", err)
	}

	updated := 0
	for _, r := range pending {
		plain, err := decodeEnvironmentJSON(r.raw)
		if err != nil {
			return updated, fmt.Errorf("backfill workload %s: %w", r.id, err)
		}
		if len(plain) == 0 {
			continue
		}
		enc, err := encodeEnvironmentJSON(plain)
		if err != nil {
			return updated, fmt.Errorf("backfill workload %s: %w", r.id, err)
		}
		tag, err := pool.Exec(ctx, `UPDATE workloads SET env_vars = $2::jsonb, updated_at = NOW() WHERE id = $1::uuid`, r.id, enc)
		if err != nil {
			return updated, err
		}
		updated += int(tag.RowsAffected())
	}
	return updated, nil
}
