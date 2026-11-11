package db

import (
	"context"
	"crypto/rand"
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const localNodeName = "control-plane"

// ListOrganizationIDs returns every organization (bypass RLS; use bootstrap/admin pool).
func ListOrganizationIDs(ctx context.Context, pool *pgxpool.Pool) ([]string, error) {
	rows, err := pool.Query(ctx, `SELECT id::text FROM organizations ORDER BY created_at`)
	if err != nil {
		return nil, fmt.Errorf("list organizations: %w", err)
	}
	defer rows.Close()
	ids := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan organization: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// DetectHostCapabilities snapshots the machine running the control plane.
func DetectHostCapabilities() NodeCapabilities {
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "control-plane"
	}
	memMB := int64(0)
	if data, err := os.ReadFile("/proc/meminfo"); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(line, "MemTotal:") {
				fields := strings.Fields(line)
				if len(fields) >= 2 {
					var kb int64
					_, _ = fmt.Sscanf(fields[1], "%d", &kb)
					memMB = kb / 1024
				}
				break
			}
		}
	}
	docker := fileExists("/var/run/docker.sock") || fileExists("/run/docker.sock")
	containerd := fileExists("/run/containerd/containerd.sock") || fileExists("/var/run/containerd/containerd.sock")
	return NodeCapabilities{
		CPUCores:         int32(runtime.NumCPU()),
		MemoryTotalMB:    memMB,
		DockerSocket:     docker,
		ContainerdSocket: containerd,
		KVMLevel:         detectKVMLevel(),
		Hostname:         hostname,
		AgentVersion:     "control-plane-local",
	}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func detectKVMLevel() uint32 {
	if !fileExists("/dev/kvm") {
		return 0
	}
	for _, path := range []string{
		"/sys/module/kvm_intel/parameters/nested",
		"/sys/module/kvm_amd/parameters/nested",
	} {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		trimmed := strings.TrimSpace(string(data))
		if trimmed == "1" || strings.EqualFold(trimmed, "Y") {
			return 2
		}
	}
	return 1
}

// EnsureLocalNode upserts the per-org embedded control-plane node and marks it online.
func EnsureLocalNode(ctx context.Context, pool *pgxpool.Pool, organizationID string, caps NodeCapabilities) (Node, error) {
	if caps.Hostname == "" {
		caps = DetectHostCapabilities()
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return Node{}, fmt.Errorf("begin ensure local node: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := WithTenant(ctx, tx, organizationID); err != nil {
		return Node{}, err
	}

	now := time.Now().UTC()
	var node Node
	err = tx.QueryRow(ctx, `
		SELECT id::text, organization_id::text, name, status, last_seen_at,
		       cpu_cores, memory_total_mb, docker_socket, containerd_socket,
		       kvm_level, hostname, agent_version, is_local
		FROM nodes
		WHERE is_local
		LIMIT 1`).Scan(
		&node.ID, &node.OrganizationID, &node.Name, &node.Status, &node.LastSeenAt,
		&node.Capabilities.CPUCores, &node.Capabilities.MemoryTotalMB,
		&node.Capabilities.DockerSocket, &node.Capabilities.ContainerdSocket,
		&node.Capabilities.KVMLevel, &node.Capabilities.Hostname, &node.Capabilities.AgentVersion,
		&node.IsLocal,
	)
	if err == nil {
		_, err = tx.Exec(ctx, `
			UPDATE nodes SET
				status = 'online',
				last_seen_at = $2,
				updated_at = $2,
				cpu_cores = $3,
				memory_total_mb = $4,
				docker_socket = $5,
				containerd_socket = $6,
				kvm_level = $7,
				hostname = $8,
				agent_version = $9
			WHERE id = $1::uuid`,
			node.ID, now,
			caps.CPUCores, caps.MemoryTotalMB, caps.DockerSocket, caps.ContainerdSocket,
			int32(caps.KVMLevel), caps.Hostname, caps.AgentVersion,
		)
		if err != nil {
			return Node{}, fmt.Errorf("refresh local node: %w", err)
		}
		node.Status = "online"
		node.LastSeenAt = &now
		node.Capabilities = caps
		node.IsLocal = true
		if err := tx.Commit(ctx); err != nil {
			return Node{}, err
		}
		return node, nil
	}
	if err != pgx.ErrNoRows {
		return Node{}, fmt.Errorf("lookup local node: %w", err)
	}

	tokenHash := make([]byte, 32)
	if _, err := rand.Read(tokenHash); err != nil {
		return Node{}, fmt.Errorf("local node token: %w", err)
	}
	name, err := uniqueNodeName(ctx, tx, organizationID, localNodeName)
	if err != nil {
		return Node{}, err
	}
	if err := tx.QueryRow(ctx, `
		INSERT INTO nodes (
			organization_id, name, status, token_hash, last_seen_at, is_local,
			cpu_cores, memory_total_mb, docker_socket, containerd_socket,
			kvm_level, hostname, agent_version, updated_at
		) VALUES (
			$1::uuid, $2, 'online', $3, $4, TRUE,
			$5, $6, $7, $8,
			$9, $10, $11, $4
		)
		RETURNING id::text, organization_id::text, name, status, last_seen_at,
		          cpu_cores, memory_total_mb, docker_socket, containerd_socket,
		          kvm_level, hostname, agent_version, is_local`,
		organizationID, name, tokenHash, now,
		caps.CPUCores, caps.MemoryTotalMB, caps.DockerSocket, caps.ContainerdSocket,
		int32(caps.KVMLevel), caps.Hostname, caps.AgentVersion,
	).Scan(
		&node.ID, &node.OrganizationID, &node.Name, &node.Status, &node.LastSeenAt,
		&node.Capabilities.CPUCores, &node.Capabilities.MemoryTotalMB,
		&node.Capabilities.DockerSocket, &node.Capabilities.ContainerdSocket,
		&node.Capabilities.KVMLevel, &node.Capabilities.Hostname, &node.Capabilities.AgentVersion,
		&node.IsLocal,
	); err != nil {
		return Node{}, fmt.Errorf("create local node: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Node{}, err
	}
	return node, nil
}
