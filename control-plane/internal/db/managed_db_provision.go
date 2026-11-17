package db

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/xiron902224/norix/control-plane/internal/secrets"
)

// ManagedConnSecret is stored encrypted for managed databases (#20).
type ManagedConnSecret struct {
	User     string `json:"user"`
	Password string `json:"password"`
	Database string `json:"database,omitempty"`
	Host     string `json:"host,omitempty"`
	Port     int32  `json:"port,omitempty"`
	URI      string `json:"uri,omitempty"`
}

// DesiredManagedDatabase is sent to the agent to run an engine container.
type DesiredManagedDatabase struct {
	ID              string
	ProjectID       string
	Name            string
	Engine          string
	Image           string
	EnvVars         map[string]string
	Command         []string
	HostPort        int32
	ContainerPort   int32
	VolumeName      string
	VolumeMountPath string
	Action          string
}

// ObservedManagedDatabase is reported by the agent after reconcile.
type ObservedManagedDatabase struct {
	ID          string
	Status      string
	ContainerID string
	Message     string
	HostPort    int32
}

func ManagedDBContainerName(id string) string {
	clean := strings.ReplaceAll(strings.TrimSpace(id), "-", "")
	if len(clean) > 12 {
		clean = clean[:12]
	}
	return "norix-mdb-" + clean
}

func ManagedDBVolumeName(id string) string {
	return ManagedDBContainerName(id) + "-data"
}

func buildManagedSecret(engine, name, password string, hostPort int32) (ManagedConnSecret, string, error) {
	user := "norix"
	dbName := sanitizeDBName(name)
	secret := ManagedConnSecret{User: user, Password: password, Database: dbName, Host: "127.0.0.1", Port: hostPort}
	var uri, redacted string
	switch engine {
	case "postgres":
		uri = fmt.Sprintf("postgres://%s:%s@127.0.0.1:%d/%s?sslmode=disable", user, password, hostPort, dbName)
		redacted = fmt.Sprintf("postgres://%s:******@127.0.0.1:%d/%s?sslmode=disable", user, hostPort, dbName)
	case "mysql":
		uri = fmt.Sprintf("mysql://%s:%s@127.0.0.1:%d/%s", user, password, hostPort, dbName)
		redacted = fmt.Sprintf("mysql://%s:******@127.0.0.1:%d/%s", user, hostPort, dbName)
	case "redis":
		uri = fmt.Sprintf("redis://:%s@127.0.0.1:%d/0", password, hostPort)
		redacted = fmt.Sprintf("redis://:******@127.0.0.1:%d/0", hostPort)
		secret.User = ""
		secret.Database = "0"
	default:
		return ManagedConnSecret{}, "", fmt.Errorf("engine %q not provisioned yet", engine)
	}
	secret.URI = uri
	return secret, redacted, nil
}

func sanitizeDBName(name string) string {
	out := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_':
			return r
		default:
			return '_'
		}
	}, strings.TrimSpace(name))
	if out == "" {
		return "norix"
	}
	if len(out) > 48 {
		out = out[:48]
	}
	return out
}

func engineRuntimeSpec(engine, name, password string) (image string, env map[string]string, cmd []string, containerPort int32, mountPath string, err error) {
	image, err = CatalogImageForEngine(engine)
	if err != nil {
		return "", nil, nil, 0, "", err
	}
	dbName := sanitizeDBName(name)
	env = map[string]string{}
	switch engine {
	case "postgres":
		env["POSTGRES_USER"] = "norix"
		env["POSTGRES_PASSWORD"] = password
		env["POSTGRES_DB"] = dbName
		return image, env, nil, 5432, "/var/lib/postgresql/data", nil
	case "mysql":
		env["MYSQL_USER"] = "norix"
		env["MYSQL_PASSWORD"] = password
		env["MYSQL_DATABASE"] = dbName
		env["MYSQL_ROOT_PASSWORD"] = password
		return image, env, nil, 3306, "/var/lib/mysql", nil
	case "redis":
		cmd = []string{"redis-server", "--requirepass", password, "--appendonly", "yes"}
		return image, env, cmd, 6379, "/data", nil
	default:
		return "", nil, nil, 0, "", fmt.Errorf("engine %q not provisioned yet", engine)
	}
}

func encryptManagedSecret(secret ManagedConnSecret) (enc, nonce []byte, err error) {
	key, err := secrets.KeyFromEnv()
	if err != nil {
		return nil, nil, err
	}
	raw, err := json.Marshal(secret)
	if err != nil {
		return nil, nil, err
	}
	return secrets.Encrypt(key, raw)
}

func DecryptManagedSecret(enc, nonce []byte) (ManagedConnSecret, error) {
	key, err := secrets.KeyFromEnv()
	if err != nil {
		return ManagedConnSecret{}, err
	}
	plain, err := secrets.Decrypt(key, enc, nonce)
	if err != nil {
		return ManagedConnSecret{}, err
	}
	var secret ManagedConnSecret
	if err := json.Unmarshal(plain, &secret); err != nil {
		return ManagedConnSecret{}, err
	}
	return secret, nil
}

type managedDBRow struct {
	ID, ProjectID, Engine, Name, Status string
	HostPort                            int32
	Enc, Nonce                          []byte
}

// ListDesiredManagedDatabasesForNode returns engine containers this node should run.
func ListDesiredManagedDatabasesForNode(ctx context.Context, pool *pgxpool.Pool, tenantID, nodeID string) ([]DesiredManagedDatabase, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	if err := WithTenant(ctx, tx, tenantID); err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `
		SELECT id::text, project_id::text, engine, name, status, COALESCE(host_port, 0),
		       connection_secret_enc, connection_secret_nonce
		FROM managed_databases
		WHERE organization_id = $1::uuid
		  AND (node_id IS NULL OR node_id = $2::uuid)
		  AND status IN ('provisioning', 'ready', 'failed', 'backing_up', 'restoring')
		  AND engine IN ('postgres', 'mysql', 'redis')
		ORDER BY created_at`, tenantID, nodeID)
	if err != nil {
		return nil, err
	}
	var pending []managedDBRow
	for rows.Next() {
		var row managedDBRow
		if err := rows.Scan(&row.ID, &row.ProjectID, &row.Engine, &row.Name, &row.Status, &row.HostPort, &row.Enc, &row.Nonce); err != nil {
			rows.Close()
			return nil, err
		}
		pending = append(pending, row)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	out := make([]DesiredManagedDatabase, 0, len(pending))
	for _, row := range pending {
		secret, err := DecryptManagedSecret(row.Enc, row.Nonce)
		if err != nil || secret.Password == "" {
			continue
		}
		hostPort := row.HostPort
		if hostPort <= 0 {
			hostPort, err = AllocateHostPort(ctx, pool, tenantID)
			if err != nil {
				continue
			}
		}
		tx2, err := pool.Begin(ctx)
		if err != nil {
			continue
		}
		if err := WithTenant(ctx, tx2, tenantID); err != nil {
			_ = tx2.Rollback(ctx)
			continue
		}
		_, _ = tx2.Exec(ctx, `
			UPDATE managed_databases SET host_port = $2, node_id = $3::uuid, updated_at = NOW()
			WHERE id = $1::uuid`, row.ID, hostPort, nodeID)
		_ = tx2.Commit(ctx)

		image, env, cmd, cport, mount, err := engineRuntimeSpec(row.Engine, row.Name, secret.Password)
		if err != nil {
			continue
		}
		out = append(out, DesiredManagedDatabase{
			ID: row.ID, ProjectID: row.ProjectID, Name: row.Name, Engine: row.Engine, Image: image,
			EnvVars: env, Command: cmd, HostPort: hostPort, ContainerPort: cport,
			VolumeName: ManagedDBVolumeName(row.ID), VolumeMountPath: mount, Action: "run",
		})
	}
	return out, nil
}

// ApplyObservedManagedDatabases updates status / container / connection URI from agent reports.
func ApplyObservedManagedDatabases(ctx context.Context, pool *pgxpool.Pool, tenantID string, observed []ObservedManagedDatabase) error {
	for _, item := range observed {
		id := strings.TrimSpace(item.ID)
		if id == "" {
			continue
		}
		tx, err := pool.Begin(ctx)
		if err != nil {
			return err
		}
		if err := WithTenant(ctx, tx, tenantID); err != nil {
			_ = tx.Rollback(ctx)
			return err
		}
		var enc, nonce []byte
		var engine, name string
		var hostPort int32
		err = tx.QueryRow(ctx, `
			SELECT engine, name, COALESCE(host_port, 0),
			       connection_secret_enc, connection_secret_nonce
			FROM managed_databases WHERE id = $1::uuid`, id).Scan(&engine, &name, &hostPort, &enc, &nonce)
		if err != nil {
			_ = tx.Rollback(ctx)
			continue
		}
		if item.HostPort > 0 {
			hostPort = item.HostPort
		}
		status := strings.ToLower(strings.TrimSpace(item.Status))
		if status == "" {
			status = "failed"
		}
		redacted := ""
		if status == "ready" {
			if secret, err := DecryptManagedSecret(enc, nonce); err == nil {
				if rebuilt, red, err := buildManagedSecret(engine, name, secret.Password, hostPort); err == nil {
					redacted = red
					if enc2, nonce2, err := encryptManagedSecret(rebuilt); err == nil {
						_, _ = tx.Exec(ctx, `
							UPDATE managed_databases SET connection_secret_enc = $2, connection_secret_nonce = $3
							WHERE id = $1::uuid`, id, enc2, nonce2)
					}
				}
			}
		}
		_, err = tx.Exec(ctx, `
			UPDATE managed_databases SET
				status = $2,
				container_id = $3,
				host_port = $4,
				connection_uri = CASE WHEN $5 <> '' THEN $5 ELSE connection_uri END,
				last_error = $6,
				updated_at = NOW()
			WHERE id = $1::uuid`,
			id, status, strings.TrimSpace(item.ContainerID), hostPort, redacted, truncate(item.Message, 500),
		)
		if err != nil {
			_ = tx.Rollback(ctx)
			return err
		}
		if err := tx.Commit(ctx); err != nil {
			return err
		}
	}
	return nil
}
