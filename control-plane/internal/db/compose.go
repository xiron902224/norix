package db

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/xiron902224/norix/control-plane/internal/composeparse"
	"github.com/xiron902224/norix/control-plane/internal/gitssrf"
)

// AnalyzeComposeFromGit shallow-clones the repo and parses the compose file.
func AnalyzeComposeFromGit(gitURL, gitRef, composePath string) (composeparse.Stack, error) {
	gitURL = strings.TrimSpace(gitURL)
	if gitURL == "" {
		return composeparse.Stack{}, fmt.Errorf("git url is required")
	}
	if err := gitssrf.ValidateGitURL(gitURL); err != nil {
		return composeparse.Stack{}, err
	}
	if gitRef == "" {
		gitRef = "main"
	}
	if composePath == "" {
		composePath = "docker-compose.yml"
	}
	workdir, err := os.MkdirTemp("", "norix-compose-*")
	if err != nil {
		return composeparse.Stack{}, err
	}
	defer os.RemoveAll(workdir)
	cloneDir := filepath.Join(workdir, "src")
	cmd := exec.Command("git", "clone", "--depth", "1", "--branch", gitRef, gitURL, cloneDir)
	if out, err := cmd.CombinedOutput(); err != nil {
		_ = os.RemoveAll(cloneDir)
		cmd = exec.Command("git", "clone", "--depth", "1", gitURL, cloneDir)
		if out2, err2 := cmd.CombinedOutput(); err2 != nil {
			msg := strings.TrimSpace(string(out) + "\n" + string(out2))
			if len(msg) > 400 {
				msg = msg[:400] + "…"
			}
			return composeparse.Stack{}, fmt.Errorf("git clone: %w (%s)", err2, msg)
		}
	}
	raw, err := os.ReadFile(filepath.Join(cloneDir, composePath))
	if err != nil {
		// try common alternates
		for _, alt := range []string{"compose.yaml", "compose.yml", "docker-compose.yaml", "docker-compose.yml"} {
			if alt == composePath {
				continue
			}
			if data, readErr := os.ReadFile(filepath.Join(cloneDir, alt)); readErr == nil {
				raw = data
				composePath = alt
				err = nil
				break
			}
		}
		if err != nil {
			return composeparse.Stack{}, fmt.Errorf("compose file %s: %w", composePath, err)
		}
	}
	return composeparse.ParseYAML(composePath, raw)
}

// AttachStackMetadata sets stack/compose fields on an existing workload.
func AttachStackMetadata(ctx context.Context, pool *pgxpool.Pool, tenantID, workloadID, stackID, composeService string, spec any) error {
	raw, err := json.Marshal(spec)
	if err != nil {
		return err
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := WithTenant(ctx, tx, tenantID); err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `
		UPDATE workloads
		SET stack_id = $3::uuid,
		    compose_service = $4,
		    compose_spec = $5::jsonb,
		    runtime = CASE WHEN runtime = 'container' THEN 'docker-compose' ELSE runtime END,
		    updated_at = NOW()
		WHERE organization_id = $1::uuid AND id = $2::uuid`,
		tenantID, workloadID, stackID, composeService, string(raw),
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("workload not found")
	}
	return tx.Commit(ctx)
}

// NewStackID allocates a random UUID string (v4-ish).
func NewStackID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	hexed := hex.EncodeToString(b[:])
	return fmt.Sprintf("%s-%s-%s-%s-%s", hexed[0:8], hexed[8:12], hexed[12:16], hexed[16:20], hexed[20:32])
}

// PreferHostPort stores the intended published port on the workload and its replicas.
func PreferHostPort(ctx context.Context, pool *pgxpool.Pool, tenantID, workloadID string, hostPort int32) error {
	if hostPort <= 0 {
		return nil
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := WithTenant(ctx, tx, tenantID); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		UPDATE workloads SET host_port = $3, updated_at = NOW()
		WHERE organization_id = $1::uuid AND id = $2::uuid`,
		tenantID, workloadID, hostPort,
	)
	if err != nil {
		return err
	}
	_, _ = tx.Exec(ctx, `
		UPDATE workload_replicas SET host_port = $3, updated_at = NOW()
		WHERE organization_id = $1::uuid AND workload_id = $2::uuid`,
		tenantID, workloadID, hostPort,
	)
	return tx.Commit(ctx)
}
