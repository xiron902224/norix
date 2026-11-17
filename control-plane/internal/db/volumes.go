package db

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// VolumeMount is a named Docker volume bound into a container (#16).
type VolumeMount struct {
	Name      string `json:"name"`
	MountPath string `json:"mount_path"`
	ReadOnly  bool   `json:"read_only,omitempty"`
}

// VolumePendingOp is a one-shot backup/delete for the assigned agent.
type VolumePendingOp struct {
	ID   string `json:"id"`
	Op   string `json:"op"` // backup | delete
	Name string `json:"name"`
}

// VolumeBackup records a completed (or failed) volume backup on a node.
type VolumeBackup struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	HostPath  string `json:"host_path,omitempty"`
	Status    string `json:"status"` // done | failed
	Error     string `json:"error,omitempty"`
	CreatedAt string `json:"created_at"`
}

var volumeNameRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,62}$`)

func HasVolumeMounts(mounts []VolumeMount) bool {
	return len(NormalizeVolumeMounts(mounts)) > 0
}

func NormalizeVolumeMounts(mounts []VolumeMount) []VolumeMount {
	if len(mounts) == 0 {
		return nil
	}
	out := make([]VolumeMount, 0, len(mounts))
	seen := map[string]struct{}{}
	for _, m := range mounts {
		name := strings.TrimSpace(m.Name)
		mountPath := strings.TrimSpace(m.MountPath)
		if name == "" && mountPath == "" {
			continue
		}
		out = append(out, VolumeMount{Name: name, MountPath: mountPath, ReadOnly: m.ReadOnly})
		seen[name] = struct{}{}
	}
	_ = seen
	return out
}

func ValidateVolumeMounts(mounts []VolumeMount) error {
	mounts = NormalizeVolumeMounts(mounts)
	seenName := map[string]struct{}{}
	seenPath := map[string]struct{}{}
	for _, m := range mounts {
		if !volumeNameRe.MatchString(m.Name) {
			return fmt.Errorf("invalid volume name %q (use [a-zA-Z0-9._-], max 63)", m.Name)
		}
		if !strings.HasPrefix(m.MountPath, "/") || m.MountPath == "/" {
			return fmt.Errorf("mount_path must be an absolute path under / (got %q)", m.MountPath)
		}
		clean := path.Clean(m.MountPath)
		if clean != m.MountPath || strings.Contains(m.MountPath, "..") {
			return fmt.Errorf("mount_path must be clean absolute path (got %q)", m.MountPath)
		}
		if _, ok := seenName[m.Name]; ok {
			return fmt.Errorf("duplicate volume name %q", m.Name)
		}
		if _, ok := seenPath[clean]; ok {
			return fmt.Errorf("duplicate mount_path %q", clean)
		}
		seenName[m.Name] = struct{}{}
		seenPath[clean] = struct{}{}
	}
	return nil
}

func EncodeVolumeMounts(mounts []VolumeMount) ([]byte, error) {
	mounts = NormalizeVolumeMounts(mounts)
	if err := ValidateVolumeMounts(mounts); err != nil {
		return nil, err
	}
	if mounts == nil {
		mounts = []VolumeMount{}
	}
	return json.Marshal(mounts)
}

func DecodeVolumeMounts(raw string) ([]VolumeMount, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" {
		return nil, nil
	}
	var mounts []VolumeMount
	if err := json.Unmarshal([]byte(raw), &mounts); err != nil {
		return nil, err
	}
	mounts = NormalizeVolumeMounts(mounts)
	if err := ValidateVolumeMounts(mounts); err != nil {
		return nil, err
	}
	return mounts, nil
}

func VolumeMountsJSON(mounts []VolumeMount) string {
	b, err := EncodeVolumeMounts(mounts)
	if err != nil {
		return "[]"
	}
	return string(b)
}

// DockerVolumeName returns a stable, unique Docker volume name for a workload mount.
func DockerVolumeName(workloadID, logicalName string) string {
	id := strings.ReplaceAll(strings.TrimSpace(workloadID), "-", "")
	if len(id) > 12 {
		id = id[:12]
	}
	name := strings.ToLower(strings.TrimSpace(logicalName))
	name = strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			return r
		default:
			return '-'
		}
	}, name)
	if name == "" {
		name = "data"
	}
	if len(name) > 40 {
		name = name[:40]
	}
	return fmt.Sprintf("norix-wv-%s-%s", id, name)
}

func DecodeVolumePendingOp(raw string) (VolumePendingOp, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" {
		return VolumePendingOp{}, false
	}
	var op VolumePendingOp
	if err := json.Unmarshal([]byte(raw), &op); err != nil {
		return VolumePendingOp{}, false
	}
	op.Op = strings.ToLower(strings.TrimSpace(op.Op))
	op.Name = strings.TrimSpace(op.Name)
	op.ID = strings.TrimSpace(op.ID)
	if op.ID == "" || (op.Op != "backup" && op.Op != "delete") || op.Name == "" {
		return VolumePendingOp{}, false
	}
	return op, true
}

func EncodeVolumePendingOp(op VolumePendingOp) string {
	b, err := json.Marshal(op)
	if err != nil {
		return ""
	}
	return string(b)
}

func DecodeVolumeBackups(raw string) []VolumeBackup {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" {
		return nil
	}
	var out []VolumeBackup
	_ = json.Unmarshal([]byte(raw), &out)
	return out
}

func newVolumeOpID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// EnqueueVolumeOp sets a pending backup/delete for the agent on the pinned node.
func EnqueueVolumeOp(ctx context.Context, pool *pgxpool.Pool, tenantID, workloadID, op, volumeName string, actor AuditActor) (VolumePendingOp, error) {
	volumeName = strings.TrimSpace(volumeName)
	op = strings.ToLower(strings.TrimSpace(op))
	if op != "backup" && op != "delete" {
		return VolumePendingOp{}, fmt.Errorf("op must be backup or delete")
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return VolumePendingOp{}, err
	}
	defer tx.Rollback(ctx)
	if err := WithTenant(ctx, tx, tenantID); err != nil {
		return VolumePendingOp{}, err
	}
	var mountsRaw, pendingRaw string
	var assigned string
	if err := tx.QueryRow(ctx, `
		SELECT COALESCE(volume_mounts::text, '[]'), COALESCE(volume_pending_op::text, ''),
		       COALESCE(assigned_node_id::text, '')
		FROM workloads WHERE id = $1::uuid`, workloadID).Scan(&mountsRaw, &pendingRaw, &assigned); err != nil {
		return VolumePendingOp{}, fmt.Errorf("workload not found")
	}
	if _, ok := DecodeVolumePendingOp(pendingRaw); ok {
		return VolumePendingOp{}, fmt.Errorf("a volume operation is already pending")
	}
	mounts, err := DecodeVolumeMounts(mountsRaw)
	if err != nil {
		return VolumePendingOp{}, err
	}
	found := false
	for _, m := range mounts {
		if m.Name == volumeName {
			found = true
			break
		}
	}
	if !found {
		return VolumePendingOp{}, fmt.Errorf("volume %q is not declared on this workload", volumeName)
	}
	if assigned == "" {
		return VolumePendingOp{}, fmt.Errorf("workload has no assigned node yet")
	}
	pending := VolumePendingOp{ID: newVolumeOpID(), Op: op, Name: volumeName}
	tag, err := tx.Exec(ctx, `
		UPDATE workloads SET volume_pending_op = $2::jsonb, updated_at = NOW()
		WHERE id = $1::uuid`, workloadID, EncodeVolumePendingOp(pending))
	if err != nil {
		return VolumePendingOp{}, err
	}
	if tag.RowsAffected() != 1 {
		return VolumePendingOp{}, fmt.Errorf("workload not found")
	}
	// Bump generation so agents refresh desired plan promptly.
	_, _ = tx.Exec(ctx, `
		UPDATE workload_replicas SET desired_generation = desired_generation + 1, updated_at = NOW()
		WHERE workload_id = $1::uuid`, workloadID)
	if err := writeAudit(ctx, tx, tenantID, "workload.volume_"+op, "workload", workloadID, actor, map[string]string{
		"volume": volumeName, "op_id": pending.ID,
	}); err != nil {
		return VolumePendingOp{}, err
	}
	return pending, tx.Commit(ctx)
}

// ApplyVolumeOpResult clears pending op and appends backup history when applicable.
func ApplyVolumeOpResult(ctx context.Context, pool *pgxpool.Pool, tenantID, workloadID, opID, status, hostPath, errMsg string) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := WithTenant(ctx, tx, tenantID); err != nil {
		return err
	}
	var pendingRaw, backupsRaw string
	if err := tx.QueryRow(ctx, `
		SELECT COALESCE(volume_pending_op::text, ''), COALESCE(volume_backups::text, '[]')
		FROM workloads WHERE id = $1::uuid`, workloadID).Scan(&pendingRaw, &backupsRaw); err != nil {
		return err
	}
	pending, ok := DecodeVolumePendingOp(pendingRaw)
	if !ok || pending.ID != opID {
		return nil
	}
	backups := DecodeVolumeBackups(backupsRaw)
	if pending.Op == "backup" {
		backups = append([]VolumeBackup{{
			ID: pending.ID, Name: pending.Name, HostPath: hostPath,
			Status: status, Error: errMsg, CreatedAt: time.Now().UTC().Format(time.RFC3339),
		}}, backups...)
		if len(backups) > 20 {
			backups = backups[:20]
		}
	}
	if pending.Op == "delete" && status == "done" {
		mountsRaw := "[]"
		_ = tx.QueryRow(ctx, `SELECT COALESCE(volume_mounts::text, '[]') FROM workloads WHERE id = $1::uuid`, workloadID).Scan(&mountsRaw)
		mounts, _ := DecodeVolumeMounts(mountsRaw)
		filtered := mounts[:0]
		for _, m := range mounts {
			if m.Name != pending.Name {
				filtered = append(filtered, m)
			}
		}
		enc, _ := EncodeVolumeMounts(filtered)
		_, _ = tx.Exec(ctx, `UPDATE workloads SET volume_mounts = $2::jsonb WHERE id = $1::uuid`, workloadID, string(enc))
	}
	encBackups, _ := json.Marshal(backups)
	_, err = tx.Exec(ctx, `
		UPDATE workloads SET volume_pending_op = NULL, volume_backups = $2::jsonb, updated_at = NOW()
		WHERE id = $1::uuid`, workloadID, string(encBackups))
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// ListStatefulWorkloadNamesOnNode returns workload names with volume mounts on a node (drain warnings).
func ListStatefulWorkloadNamesOnNode(ctx context.Context, pool *pgxpool.Pool, tenantID, nodeID string) ([]string, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	if err := WithTenant(ctx, tx, tenantID); err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `
		SELECT DISTINCT w.name
		FROM workloads w
		JOIN workload_replicas wr ON wr.workload_id = w.id
		WHERE wr.assigned_node_id = $1::uuid
		  AND jsonb_array_length(COALESCE(w.volume_mounts, '[]'::jsonb)) > 0
		  AND w.status <> 'deleting'
		ORDER BY w.name`, nodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]string, 0)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		out = append(out, name)
	}
	return out, rows.Err()
}
