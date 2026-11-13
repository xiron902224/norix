package db

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type NodeCapabilities struct {
	CPUCores         int32
	MemoryTotalMB    int64
	DockerSocket     bool
	ContainerdSocket bool
	KVMLevel         uint32
	Hostname         string
	AgentVersion     string
}

type Node struct {
	ID               string
	OrganizationID   string
	Name             string
	Status           string
	LastSeenAt       *time.Time
	Capabilities     NodeCapabilities
	IsLocal          bool
	CPUUsagePercent  float64
	MemoryFreeMB     int64
	DiskFreeMB       int64
	Role             string
	Cordoned         bool
	Draining         bool
	Zone             string
	Labels           map[string]string
}

type JoinToken struct {
	Token     string
	ExpiresAt time.Time
}

func CreateJoinToken(ctx context.Context, pool *pgxpool.Pool, organizationID, nodeName string, ttl time.Duration, tokenHash []byte, plainToken string, actor AuditActor) (JoinToken, error) {
	if ttl <= 0 {
		ttl = time.Hour
	}
	if ttl > 24*time.Hour {
		ttl = 24 * time.Hour
	}
	expiresAt := time.Now().UTC().Add(ttl)
	nodeName = strings.TrimSpace(nodeName)

	tx, err := pool.Begin(ctx)
	if err != nil {
		return JoinToken{}, fmt.Errorf("begin join token transaction: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := WithTenant(ctx, tx, organizationID); err != nil {
		return JoinToken{}, err
	}

	var tokenID string
	if err := tx.QueryRow(ctx, `
		INSERT INTO join_tokens (organization_id, token_hash, node_name, expires_at, created_by_user_id)
		VALUES ($1::uuid, $2, NULLIF($3, ''), $4, NULLIF($5, '')::uuid)
		RETURNING id::text`, organizationID, tokenHash, nodeName, expiresAt, actor.UserID).Scan(&tokenID); err != nil {
		return JoinToken{}, fmt.Errorf("create join token: %w", err)
	}
	if err := writeAudit(ctx, tx, organizationID, "node.join_token_created", "join_token", tokenID, actor, map[string]string{
		"node_name": nodeName,
	}); err != nil {
		return JoinToken{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return JoinToken{}, fmt.Errorf("commit join token: %w", err)
	}
	return JoinToken{Token: plainToken, ExpiresAt: expiresAt}, nil
}

func EnrollNode(ctx context.Context, pool *pgxpool.Pool, joinTokenHash, nodeTokenHash []byte, nodeName string, caps NodeCapabilities) (Node, string, error) {
	nodeName = strings.TrimSpace(nodeName)
	if nodeName == "" {
		nodeName = "node"
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return Node{}, "", fmt.Errorf("begin enroll transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	var organizationID string
	var suggestedName *string
	var tokenID string
	err = tx.QueryRow(ctx, `
		SELECT id::text, organization_id::text, node_name
		FROM join_tokens
		WHERE token_hash = $1
		  AND consumed_at IS NULL
		  AND expires_at > NOW()
		FOR UPDATE`, joinTokenHash).Scan(&tokenID, &organizationID, &suggestedName)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Node{}, "", fmt.Errorf("join token is invalid or expired")
		}
		return Node{}, "", fmt.Errorf("lookup join token: %w", err)
	}
	if suggestedName != nil && strings.TrimSpace(*suggestedName) != "" && nodeName == "node" {
		nodeName = strings.TrimSpace(*suggestedName)
	}

	if err := WithTenant(ctx, tx, organizationID); err != nil {
		return Node{}, "", err
	}

	nodeName, err = uniqueNodeName(ctx, tx, organizationID, nodeName)
	if err != nil {
		return Node{}, "", err
	}

	now := time.Now().UTC()
	var node Node
	if err := tx.QueryRow(ctx, `
		INSERT INTO nodes (
			organization_id, name, status, token_hash, last_seen_at,
			cpu_cores, memory_total_mb, docker_socket, containerd_socket,
			kvm_level, hostname, agent_version, updated_at
		) VALUES (
			$1::uuid, $2, 'online', $3, $4,
			$5, $6, $7, $8,
			$9, $10, $11, $4
		)
		RETURNING id::text, organization_id::text, name, status, last_seen_at`,
		organizationID, nodeName, nodeTokenHash, now,
		caps.CPUCores, caps.MemoryTotalMB, caps.DockerSocket, caps.ContainerdSocket,
		int32(caps.KVMLevel), caps.Hostname, caps.AgentVersion,
	).Scan(&node.ID, &node.OrganizationID, &node.Name, &node.Status, &node.LastSeenAt); err != nil {
		return Node{}, "", fmt.Errorf("create node: %w", err)
	}
	node.Capabilities = caps

	tag, err := tx.Exec(ctx, `
		UPDATE join_tokens
		SET consumed_at = $2
		WHERE id = $1::uuid AND consumed_at IS NULL`, tokenID, now)
	if err != nil {
		return Node{}, "", fmt.Errorf("consume join token: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return Node{}, "", fmt.Errorf("join token is invalid or expired")
	}

	if err := writeAudit(ctx, tx, organizationID, "node.enrolled", "node", node.ID, AuditActor{}, map[string]string{
		"name": node.Name,
	}); err != nil {
		return Node{}, "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return Node{}, "", fmt.Errorf("commit enroll: %w", err)
	}
	return node, organizationID, nil
}

func uniqueNodeName(ctx context.Context, tx pgx.Tx, organizationID, base string) (string, error) {
	base = strings.TrimSpace(base)
	if base == "" {
		base = "node"
	}
	candidate := base
	for i := 2; i < 1000; i++ {
		var exists bool
		if err := tx.QueryRow(ctx, `
			SELECT EXISTS(
				SELECT 1 FROM nodes WHERE organization_id = $1::uuid AND name = $2
			)`, organizationID, candidate).Scan(&exists); err != nil {
			return "", fmt.Errorf("check node name: %w", err)
		}
		if !exists {
			return candidate, nil
		}
		candidate = fmt.Sprintf("%s-%d", base, i)
	}
	return "", fmt.Errorf("could not allocate unique node name")
}

func FindNodeByTokenHash(ctx context.Context, pool *pgxpool.Pool, tokenHash []byte) (Node, error) {
	var node Node
	var lastSeen *time.Time
	err := pool.QueryRow(ctx, `
		SELECT id::text, organization_id::text, name, status, last_seen_at,
		       cpu_cores, memory_total_mb, docker_socket, containerd_socket,
		       kvm_level, hostname, agent_version, is_local
		FROM nodes
		WHERE token_hash = $1`, tokenHash).Scan(
		&node.ID, &node.OrganizationID, &node.Name, &node.Status, &lastSeen,
		&node.Capabilities.CPUCores, &node.Capabilities.MemoryTotalMB,
		&node.Capabilities.DockerSocket, &node.Capabilities.ContainerdSocket,
		&node.Capabilities.KVMLevel, &node.Capabilities.Hostname, &node.Capabilities.AgentVersion,
		&node.IsLocal,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Node{}, fmt.Errorf("node credential not found")
		}
		return Node{}, fmt.Errorf("lookup node credential: %w", err)
	}
	node.LastSeenAt = lastSeen
	return node, nil
}

func ListNodes(ctx context.Context, pool *pgxpool.Pool, organizationID string) ([]Node, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin list nodes: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := WithTenant(ctx, tx, organizationID); err != nil {
		return nil, err
	}

	rows, err := tx.Query(ctx, `
		SELECT id::text, organization_id::text, name, status, last_seen_at,
		       cpu_cores, memory_total_mb, docker_socket, containerd_socket,
		       kvm_level, hostname, agent_version, is_local,
		       cpu_usage_percent, memory_free_mb, disk_free_mb,
		       role, cordoned, draining, zone, labels::text
		FROM nodes
		ORDER BY is_local DESC, COALESCE(last_seen_at, created_at) DESC, name`)
	if err != nil {
		return nil, fmt.Errorf("list nodes: %w", err)
	}
	defer rows.Close()

	nodes := make([]Node, 0)
	for rows.Next() {
		var node Node
		var lastSeen *time.Time
		var labelsText string
		if err := rows.Scan(
			&node.ID, &node.OrganizationID, &node.Name, &node.Status, &lastSeen,
			&node.Capabilities.CPUCores, &node.Capabilities.MemoryTotalMB,
			&node.Capabilities.DockerSocket, &node.Capabilities.ContainerdSocket,
			&node.Capabilities.KVMLevel, &node.Capabilities.Hostname, &node.Capabilities.AgentVersion,
			&node.IsLocal, &node.CPUUsagePercent, &node.MemoryFreeMB, &node.DiskFreeMB,
			&node.Role, &node.Cordoned, &node.Draining, &node.Zone, &labelsText,
		); err != nil {
			return nil, fmt.Errorf("scan node: %w", err)
		}
		node.LastSeenAt = lastSeen
		node.Labels = decodeLabels([]byte(labelsText))
		if node.Role == "" {
			node.Role = "worker"
		}
		if node.Zone == "" {
			node.Zone = "default"
		}
		nodes = append(nodes, node)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate nodes: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit list nodes: %w", err)
	}
	return nodes, nil
}

func HasEnrolledNode(ctx context.Context, pool *pgxpool.Pool, organizationID string) (bool, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("begin has node: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := WithTenant(ctx, tx, organizationID); err != nil {
		return false, err
	}
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM nodes)`).Scan(&exists); err != nil {
		return false, fmt.Errorf("check enrolled node: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("commit has node: %w", err)
	}
	return exists, nil
}

func RecordHeartbeat(ctx context.Context, pool *pgxpool.Pool, organizationID, nodeID, status string, caps *NodeCapabilities, cpuUsage float64, memoryFreeMB, diskFreeMB int64) error {
	if status == "" {
		status = "online"
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin heartbeat: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := WithTenant(ctx, tx, organizationID); err != nil {
		return err
	}

	now := time.Now().UTC()
	var affected int64
	if caps != nil {
		commandTag, execErr := tx.Exec(ctx, `
			UPDATE nodes SET
				status = $3,
				last_seen_at = $4,
				updated_at = $4,
				cpu_cores = $5,
				memory_total_mb = $6,
				docker_socket = $7,
				containerd_socket = $8,
				kvm_level = $9,
				hostname = CASE WHEN $10 = '' THEN hostname ELSE $10 END,
				agent_version = CASE WHEN $11 = '' THEN agent_version ELSE $11 END,
				cpu_usage_percent = $12,
				memory_free_mb = $13,
				disk_free_mb = $14
			WHERE id = $1::uuid AND organization_id = $2::uuid`,
			nodeID, organizationID, status, now,
			caps.CPUCores, caps.MemoryTotalMB, caps.DockerSocket, caps.ContainerdSocket,
			int32(caps.KVMLevel), caps.Hostname, caps.AgentVersion,
			cpuUsage, memoryFreeMB, diskFreeMB,
		)
		if execErr != nil {
			return fmt.Errorf("update heartbeat: %w", execErr)
		}
		affected = commandTag.RowsAffected()
	} else {
		commandTag, execErr := tx.Exec(ctx, `
			UPDATE nodes SET status = $3, last_seen_at = $4, updated_at = $4,
				cpu_usage_percent = $5, memory_free_mb = $6, disk_free_mb = $7
			WHERE id = $1::uuid AND organization_id = $2::uuid`,
			nodeID, organizationID, status, now, cpuUsage, memoryFreeMB, diskFreeMB,
		)
		if execErr != nil {
			return fmt.Errorf("update heartbeat: %w", execErr)
		}
		affected = commandTag.RowsAffected()
	}
	if affected != 1 {
		return fmt.Errorf("node not found")
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit heartbeat: %w", err)
	}
	return nil
}

func MarkStaleNodesOffline(ctx context.Context, pool *pgxpool.Pool, staleAfter time.Duration) (int64, error) {
	if staleAfter <= 0 {
		staleAfter = 30 * time.Second
	}
	seconds := int64(staleAfter.Seconds())
	if seconds < 1 {
		seconds = 30
	}
	tag, err := pool.Exec(ctx, `
		UPDATE nodes
		SET status = 'offline', updated_at = NOW()
		WHERE status = 'online'
		  AND NOT is_local
		  AND (last_seen_at IS NULL OR last_seen_at < NOW() - make_interval(secs => $1))`, seconds)
	if err != nil {
		return 0, fmt.Errorf("mark stale nodes: %w", err)
	}
	return tag.RowsAffected(), nil
}
