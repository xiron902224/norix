package db

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const previousTokenGrace = 15 * time.Minute

// RequestNodeTokenRotation marks the node so the next Heartbeat issues a new bearer (#13).
func RequestNodeTokenRotation(ctx context.Context, pool *pgxpool.Pool, organizationID, nodeID string, actor AuditActor) (Node, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return Node{}, fmt.Errorf("begin rotate request: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := WithTenant(ctx, tx, organizationID); err != nil {
		return Node{}, err
	}
	tag, err := tx.Exec(ctx, `
		UPDATE nodes
		SET credential_rotate_requested_at = NOW(),
		    token_revoked_at = NULL,
		    updated_at = NOW()
		WHERE id = $1::uuid AND organization_id = $2::uuid AND COALESCE(is_local, false) = false`,
		nodeID, organizationID)
	if err != nil {
		return Node{}, fmt.Errorf("request token rotation: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return Node{}, fmt.Errorf("node not found or is local (no agent token)")
	}
	if err := writeAudit(ctx, tx, organizationID, "node.token_rotate_requested", "node", nodeID, actor, nil); err != nil {
		return Node{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Node{}, err
	}
	return GetNode(ctx, pool, organizationID, nodeID)
}

// RevokeNodeToken invalidates the node bearer immediately (#13).
func RevokeNodeToken(ctx context.Context, pool *pgxpool.Pool, organizationID, nodeID string, actor AuditActor) (Node, error) {
	junk := make([]byte, 32)
	if _, err := rand.Read(junk); err != nil {
		return Node{}, fmt.Errorf("generate revoked token hash: %w", err)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return Node{}, fmt.Errorf("begin revoke: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := WithTenant(ctx, tx, organizationID); err != nil {
		return Node{}, err
	}
	tag, err := tx.Exec(ctx, `
		UPDATE nodes
		SET token_hash = $3,
		    previous_token_hash = NULL,
		    previous_token_expires_at = NULL,
		    credential_rotate_requested_at = NULL,
		    token_revoked_at = NOW(),
		    status = 'offline',
		    updated_at = NOW()
		WHERE id = $1::uuid AND organization_id = $2::uuid AND COALESCE(is_local, false) = false`,
		nodeID, organizationID, junk)
	if err != nil {
		return Node{}, fmt.Errorf("revoke node token: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return Node{}, fmt.Errorf("node not found or is local (no agent token)")
	}
	if err := writeAudit(ctx, tx, organizationID, "node.token_revoked", "node", nodeID, actor, nil); err != nil {
		return Node{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Node{}, err
	}
	return GetNode(ctx, pool, organizationID, nodeID)
}

// SetNodeClientCertSHA256 binds optional mTLS client-cert fingerprint (#13).
func SetNodeClientCertSHA256(ctx context.Context, pool *pgxpool.Pool, organizationID, nodeID, sha256Hex string, actor AuditActor) (Node, error) {
	sha256Hex = strings.ToLower(strings.TrimSpace(sha256Hex))
	if sha256Hex != "" {
		if len(sha256Hex) != 64 {
			return Node{}, fmt.Errorf("certificate_sha256 must be 64 hex characters")
		}
		if _, err := hex.DecodeString(sha256Hex); err != nil {
			return Node{}, fmt.Errorf("certificate_sha256 must be hex")
		}
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return Node{}, fmt.Errorf("begin set client cert: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := WithTenant(ctx, tx, organizationID); err != nil {
		return Node{}, err
	}
	var certValue *string
	if sha256Hex != "" {
		certValue = &sha256Hex
	}
	tag, err := tx.Exec(ctx, `
		UPDATE nodes
		SET client_cert_sha256 = $3,
		    updated_at = NOW()
		WHERE id = $1::uuid AND organization_id = $2::uuid`,
		nodeID, organizationID, certValue)
	if err != nil {
		return Node{}, fmt.Errorf("set client cert: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return Node{}, fmt.Errorf("node not found")
	}
	action := "node.client_cert_cleared"
	if sha256Hex != "" {
		action = "node.client_cert_set"
	}
	if err := writeAudit(ctx, tx, organizationID, action, "node", nodeID, actor, map[string]string{
		"certificate_sha256_prefix": prefix(sha256Hex, 8),
	}); err != nil {
		return Node{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Node{}, err
	}
	return GetNode(ctx, pool, organizationID, nodeID)
}

// CompleteNodeTokenRotationIfRequested issues a new bearer when rotation was requested.
// Returns the plaintext token when a rotation completed; empty string otherwise.
func CompleteNodeTokenRotationIfRequested(ctx context.Context, pool *pgxpool.Pool, organizationID, nodeID string, newTokenHash []byte) (plainDelivered bool, err error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("begin complete rotate: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := WithTenant(ctx, tx, organizationID); err != nil {
		return false, err
	}

	var requested *time.Time
	var currentHash []byte
	err = tx.QueryRow(ctx, `
		SELECT credential_rotate_requested_at, token_hash
		FROM nodes
		WHERE id = $1::uuid AND organization_id = $2::uuid
		FOR UPDATE`, nodeID, organizationID).Scan(&requested, &currentHash)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, fmt.Errorf("node not found")
		}
		return false, fmt.Errorf("load rotate state: %w", err)
	}
	if requested == nil {
		return false, nil
	}

	expires := time.Now().UTC().Add(previousTokenGrace)
	tag, err := tx.Exec(ctx, `
		UPDATE nodes
		SET token_hash = $3,
		    previous_token_hash = $4,
		    previous_token_expires_at = $5,
		    credential_rotate_requested_at = NULL,
		    token_revoked_at = NULL,
		    updated_at = NOW()
		WHERE id = $1::uuid AND organization_id = $2::uuid`,
		nodeID, organizationID, newTokenHash, currentHash, expires)
	if err != nil {
		return false, fmt.Errorf("complete token rotation: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return false, fmt.Errorf("node not found")
	}
	if err := writeAudit(ctx, tx, organizationID, "node.token_rotated", "node", nodeID, AuditActor{}, map[string]string{
		"grace_until": expires.Format(time.RFC3339),
	}); err != nil {
		return false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return false, err
	}
	return true, nil
}

// FindNodeByClientCertSHA256 authenticates optional mTLS by fingerprint (#13).
func FindNodeByClientCertSHA256(ctx context.Context, pool *pgxpool.Pool, sha256Hex string) (Node, error) {
	sha256Hex = strings.ToLower(strings.TrimSpace(sha256Hex))
	if sha256Hex == "" {
		return Node{}, fmt.Errorf("empty client cert fingerprint")
	}
	var node Node
	var lastSeen *time.Time
	var rotateAt *time.Time
	var cert *string
	err := pool.QueryRow(ctx, `
		SELECT id::text, organization_id::text, name, status, last_seen_at,
		       cpu_cores, memory_total_mb, docker_socket, containerd_socket,
		       kvm_level, hostname, agent_version, is_local,
		       COALESCE(desired_agent_version, ''),
		       credential_rotate_requested_at,
		       client_cert_sha256
		FROM nodes
		WHERE lower(client_cert_sha256) = $1
		  AND token_revoked_at IS NULL`, sha256Hex).Scan(
		&node.ID, &node.OrganizationID, &node.Name, &node.Status, &lastSeen,
		&node.Capabilities.CPUCores, &node.Capabilities.MemoryTotalMB,
		&node.Capabilities.DockerSocket, &node.Capabilities.ContainerdSocket,
		&node.Capabilities.KVMLevel, &node.Capabilities.Hostname, &node.Capabilities.AgentVersion,
		&node.IsLocal, &node.DesiredAgentVersion, &rotateAt, &cert,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Node{}, fmt.Errorf("node credential not found")
		}
		return Node{}, fmt.Errorf("lookup node by client cert: %w", err)
	}
	node.LastSeenAt = lastSeen
	node.CredentialRotatePending = rotateAt != nil
	node.ClientCertConfigured = cert != nil && strings.TrimSpace(*cert) != ""
	return node, nil
}

func prefix(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
