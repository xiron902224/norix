package db

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/xiron902224/norix/control-plane/internal/network"
	"github.com/xiron902224/norix/control-plane/internal/secrets"
)

type NetworkPeer struct {
	NodeID     string
	NodeName   string
	PublicKey  string
	Address    string
	Endpoint   string
	AllowedIPs string
}

type NetworkDNSRecord struct {
	Name  string
	Type  string
	Value string
	TTL   int32
}

type NetworkCertificate struct {
	Hostname  string
	CertPEM   string
	KeyPEM    string
	NotAfter  time.Time
	Issuer    string
}

type DesiredNetworkPlan struct {
	Generation   int64
	Interface    string
	Address      string
	ListenPort   int32
	PrivateKey   string
	PublicKey    string
	Peers        []NetworkPeer
	DNSRecords   []NetworkDNSRecord
	Nftables     string
	WGConfig     string
	Certificates []NetworkCertificate
	Apply        bool
}

type ObservedNetwork struct {
	Generation int64
	Status     string
	Message    string
	LogLines   []string
}

type EgressPolicy struct {
	ID           string
	OrganizationID string
	WorkloadID   string
	Mode         string
	Allowlist    []string
}

type NetworkStatus struct {
	NodesReady   int
	NodesTotal   int
	DNSRecords   int
	Certificates int
	OverlayCIDR  string
	DNSSuffix    string
	ApplyEnabled bool
	Nodes        []NodeNetworkView
}

type NodeNetworkView struct {
	NodeID        string
	Name          string
	Role          string
	Status        string
	WGAddress     string
	WGPublicKey   string
	NetworkStatus string
	LastError     string
	Generation    int64
	ObservedGen   int64
}

// NetworkApplyEnabled gates WireGuard/nft apply on the local runtime and sets DesiredNetworkPlan.apply
// in agent heartbeats. Default is dry-run (false): configs are written under NORIX_NETWORK_STATE_DIR
// (control plane) or <agent-workdir>/network (agent) without calling wg-quick/nft.
// Enable live apply on a single node with NORIX_APPLY_WIREGUARD=1|true|yes on the control plane process.
// GetNetworkStatusResponse.apply_enabled mirrors this flag for the dashboard.
func NetworkApplyEnabled() bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv("NORIX_APPLY_WIREGUARD")))
	return value == "1" || value == "true" || value == "yes"
}

func EnsureNodeNetworkIdentity(ctx context.Context, pool *pgxpool.Pool, tenantID, nodeID string) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := WithTenant(ctx, tx, tenantID); err != nil {
		return err
	}
	var pub, addr string
	err = tx.QueryRow(ctx, `SELECT wg_public_key, wg_address FROM nodes WHERE id = $1::uuid`, nodeID).Scan(&pub, &addr)
	if err != nil {
		return err
	}
	if pub != "" && addr != "" {
		return tx.Commit(ctx)
	}
	pair, err := network.GenerateKeyPair()
	if err != nil {
		return err
	}
	var index uint32
	if err := tx.QueryRow(ctx, `
		SELECT COALESCE(COUNT(*) FILTER (WHERE wg_address <> ''), 0) + 1
		FROM nodes`).Scan(&index); err != nil {
		index = uint32(time.Now().UnixNano()%200 + 1)
	}
	if index == 0 || index > 65000 {
		index = 1
	}
	address := network.NodeAddress(index)
	key, keyErr := secrets.KeyFromEnv()
	var enc, nonce []byte
	if keyErr == nil {
		enc, nonce, err = secrets.Encrypt(key, []byte(pair.PrivateKey))
		if err != nil {
			return err
		}
	} else {
		// Dev fallback: store plaintext bytes when secrets key is unset.
		enc = []byte(pair.PrivateKey)
		nonce = []byte("dev-nonce")
	}
	_, err = tx.Exec(ctx, `
		UPDATE nodes SET
			wg_public_key = $2,
			wg_private_key_enc = $3,
			wg_private_key_nonce = $4,
			wg_address = $5,
			wg_listen_port = $6,
			network_status = CASE WHEN network_status = '' THEN 'pending' ELSE network_status END,
			updated_at = NOW()
		WHERE id = $1::uuid`, nodeID, pair.PublicKey, enc, nonce, address, network.DefaultWGPort)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func EnsureOrgNetwork(ctx context.Context, pool *pgxpool.Pool, tenantID string) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := WithTenant(ctx, tx, tenantID); err != nil {
		return err
	}
	rows, err := tx.Query(ctx, `SELECT id::text, name FROM projects ORDER BY created_at`)
	if err != nil {
		return err
	}
	type project struct{ ID, Name string }
	projects := make([]project, 0)
	for rows.Next() {
		var item project
		if err := rows.Scan(&item.ID, &item.Name); err != nil {
			rows.Close()
			return err
		}
		projects = append(projects, item)
	}
	rows.Close()
	for i, project := range projects {
		mark := network.ProjectMark(i + 1)
		_, err = tx.Exec(ctx, `
			INSERT INTO project_network (organization_id, project_id, isolation_mark, subnet_cidr)
			VALUES ($1::uuid, $2::uuid, $3, $4)
			ON CONFLICT (project_id) DO UPDATE SET isolation_mark = EXCLUDED.isolation_mark, updated_at = NOW()`,
			tenantID, project.ID, mark, network.OverlayCIDR)
		if err != nil {
			return err
		}
	}
	if err := ensurePlatformCertTx(ctx, tx, tenantID); err != nil {
		return err
	}
	if err := syncWorkloadDNSTx(ctx, tx, tenantID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func ensurePlatformCertTx(ctx context.Context, tx pgx.Tx, tenantID string) error {
	var count int
	if err := tx.QueryRow(ctx, `
		SELECT COUNT(*) FROM platform_certificates
		WHERE hostname = $1 AND not_after > NOW() + INTERVAL '7 days'`, network.WildcardHostname()).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	bundle, err := network.IssueSelfSignedWildcard(network.WildcardHostname(), 0)
	if err != nil {
		return err
	}
	key, keyErr := secrets.KeyFromEnv()
	var enc, nonce []byte
	if keyErr == nil {
		enc, nonce, err = secrets.Encrypt(key, []byte(bundle.KeyPEM))
		if err != nil {
			return err
		}
	} else {
		enc = []byte(bundle.KeyPEM)
		nonce = []byte("dev-nonce")
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO platform_certificates (organization_id, hostname, cert_pem, key_pem_enc, key_nonce, not_before, not_after, issuer)
		VALUES ($1::uuid, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (organization_id, hostname) DO UPDATE SET
			cert_pem = EXCLUDED.cert_pem,
			key_pem_enc = EXCLUDED.key_pem_enc,
			key_nonce = EXCLUDED.key_nonce,
			not_before = EXCLUDED.not_before,
			not_after = EXCLUDED.not_after,
			issuer = EXCLUDED.issuer,
			updated_at = NOW()`,
		tenantID, bundle.Hostname, bundle.CertPEM, enc, nonce, bundle.NotBefore, bundle.NotAfter, bundle.Issuer)
	return err
}

func syncWorkloadDNSTx(ctx context.Context, tx pgx.Tx, tenantID string) error {
	rows, err := tx.Query(ctx, `
		SELECT w.id::text, w.project_id::text, w.name, p.name, COALESCE(n.wg_address, ''), COALESCE(w.host_port, 0)
		FROM workloads w
		JOIN projects p ON p.id = w.project_id
		LEFT JOIN nodes n ON n.id = w.assigned_node_id
		WHERE w.status <> 'deleting'`)
	if err != nil {
		return err
	}
	type dnsRow struct {
		WorkloadID, ProjectID, WorkloadName, ProjectName, WGAddress string
		HostPort                                                    int32
	}
	items := make([]dnsRow, 0)
	for rows.Next() {
		var item dnsRow
		if err := rows.Scan(&item.WorkloadID, &item.ProjectID, &item.WorkloadName, &item.ProjectName, &item.WGAddress, &item.HostPort); err != nil {
			rows.Close()
			return err
		}
		items = append(items, item)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, item := range items {
		fqdn := network.ServiceFQDN(item.ProjectName, item.WorkloadName)
		value := strings.TrimSuffix(item.WGAddress, "/32")
		if value == "" {
			value = "127.0.0.1"
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO network_dns_records (organization_id, project_id, workload_id, name, record_type, value, ttl_seconds)
			VALUES ($1::uuid, $2::uuid, $3::uuid, $4, 'A', $5, 30)
			ON CONFLICT (organization_id, name, record_type) DO UPDATE SET
				value = EXCLUDED.value, workload_id = EXCLUDED.workload_id, updated_at = NOW()`,
			tenantID, item.ProjectID, item.WorkloadID, fqdn, value)
		if err != nil {
			return err
		}
	}
	return nil
}

func GetDesiredNetworkPlan(ctx context.Context, pool *pgxpool.Pool, tenantID, nodeID string) (DesiredNetworkPlan, error) {
	if err := EnsureNodeNetworkIdentity(ctx, pool, tenantID, nodeID); err != nil {
		return DesiredNetworkPlan{}, err
	}
	if err := EnsureOrgNetwork(ctx, pool, tenantID); err != nil {
		return DesiredNetworkPlan{}, err
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return DesiredNetworkPlan{}, err
	}
	defer tx.Rollback(ctx)
	if err := WithTenant(ctx, tx, tenantID); err != nil {
		return DesiredNetworkPlan{}, err
	}

	var plan DesiredNetworkPlan
	var enc, nonce []byte
	var generation int64
	err = tx.QueryRow(ctx, `
		SELECT wg_public_key, wg_private_key_enc, wg_private_key_nonce, wg_address, wg_listen_port, network_generation
		FROM nodes WHERE id = $1::uuid`, nodeID).Scan(
		&plan.PublicKey, &enc, &nonce, &plan.Address, &plan.ListenPort, &generation,
	)
	if err != nil {
		return DesiredNetworkPlan{}, err
	}
	plan.PrivateKey = decryptNetworkSecret(enc, nonce)
	plan.Interface = "wg-norix"
	plan.Apply = NetworkApplyEnabled()
	plan.Generation = generation
	if plan.Generation <= 0 {
		plan.Generation = time.Now().Unix()
		_, _ = tx.Exec(ctx, `UPDATE nodes SET network_generation = $2 WHERE id = $1::uuid`, nodeID, plan.Generation)
	}

	rows, err := tx.Query(ctx, `
		SELECT id::text, name, wg_public_key, wg_address, wg_listen_port, COALESCE(hostname, '')
		FROM nodes
		WHERE id <> $1::uuid AND status = 'online' AND wg_public_key <> '' AND wg_address <> ''`, nodeID)
	if err != nil {
		return DesiredNetworkPlan{}, err
	}
	peers := make([]NetworkPeer, 0)
	wgPeers := make([]network.PeerConfig, 0)
	for rows.Next() {
		var peer NetworkPeer
		var listenPort int32
		var hostHint string
		if err := rows.Scan(&peer.NodeID, &peer.NodeName, &peer.PublicKey, &peer.Address, &listenPort, &hostHint); err != nil {
			rows.Close()
			return DesiredNetworkPlan{}, err
		}
		peer.AllowedIPs = peer.Address
		if hostHint != "" && listenPort > 0 {
			peer.Endpoint = fmt.Sprintf("%s:%d", hostHint, listenPort)
		}
		peers = append(peers, peer)
		wgPeers = append(wgPeers, network.PeerConfig{
			PublicKey: peer.PublicKey, AllowedIPs: peer.AllowedIPs, Endpoint: peer.Endpoint,
			NodeID: peer.NodeID, NodeName: peer.NodeName, Address: peer.Address,
		})
	}
	rows.Close()
	plan.Peers = peers
	plan.WGConfig = network.RenderWGQuick(plan.Interface, plan.PrivateKey, plan.Address, int(plan.ListenPort), wgPeers)

	dnsRows, err := tx.Query(ctx, `
		SELECT name, record_type, value, ttl_seconds FROM network_dns_records ORDER BY name`)
	if err != nil {
		return DesiredNetworkPlan{}, err
	}
	for dnsRows.Next() {
		var item NetworkDNSRecord
		if err := dnsRows.Scan(&item.Name, &item.Type, &item.Value, &item.TTL); err != nil {
			dnsRows.Close()
			return DesiredNetworkPlan{}, err
		}
		plan.DNSRecords = append(plan.DNSRecords, item)
	}
	dnsRows.Close()

	projectRows, err := tx.Query(ctx, `
		SELECT pn.project_id::text, p.name, pn.isolation_mark
		FROM project_network pn JOIN projects p ON p.id = pn.project_id
		ORDER BY pn.isolation_mark`)
	if err != nil {
		return DesiredNetworkPlan{}, err
	}
	isolations := make([]network.ProjectIsolation, 0)
	for projectRows.Next() {
		var item network.ProjectIsolation
		if err := projectRows.Scan(&item.ProjectID, &item.ProjectName, &item.Mark); err != nil {
			projectRows.Close()
			return DesiredNetworkPlan{}, err
		}
		item.NodeAddrs = []string{plan.Address}
		isolations = append(isolations, item)
	}
	projectRows.Close()

	egressRows, err := tx.Query(ctx, `
		SELECT e.workload_id::text, w.name, e.mode, e.allowlist::text
		FROM workload_egress_policies e
		JOIN workloads w ON w.id = e.workload_id`)
	if err != nil {
		return DesiredNetworkPlan{}, err
	}
	egress := make([]network.EgressRule, 0)
	for egressRows.Next() {
		var item network.EgressRule
		var allowText string
		if err := egressRows.Scan(&item.WorkloadID, &item.WorkloadName, &item.Mode, &allowText); err != nil {
			egressRows.Close()
			return DesiredNetworkPlan{}, err
		}
		_ = json.Unmarshal([]byte(allowText), &item.Allowlist)
		egress = append(egress, item)
	}
	egressRows.Close()
	plan.Nftables = network.RenderNftables(isolations, egress)

	certRows, err := tx.Query(ctx, `
		SELECT hostname, cert_pem, key_pem_enc, key_nonce, not_after, issuer
		FROM platform_certificates WHERE not_after > NOW()`)
	if err != nil {
		return DesiredNetworkPlan{}, err
	}
	for certRows.Next() {
		var item NetworkCertificate
		var keyEnc, keyNonce []byte
		if err := certRows.Scan(&item.Hostname, &item.CertPEM, &keyEnc, &keyNonce, &item.NotAfter, &item.Issuer); err != nil {
			certRows.Close()
			return DesiredNetworkPlan{}, err
		}
		item.KeyPEM = decryptNetworkSecret(keyEnc, keyNonce)
		plan.Certificates = append(plan.Certificates, item)
	}
	certRows.Close()

	if err := tx.Commit(ctx); err != nil {
		return DesiredNetworkPlan{}, err
	}
	return plan, nil
}

func decryptNetworkSecret(enc, nonce []byte) string {
	if len(enc) == 0 {
		return ""
	}
	key, err := secrets.KeyFromEnv()
	if err != nil {
		return string(enc)
	}
	plain, err := secrets.Decrypt(key, enc, nonce)
	if err != nil {
		return string(enc)
	}
	return string(plain)
}

func ApplyObservedNetwork(ctx context.Context, pool *pgxpool.Pool, tenantID, nodeID string, observed ObservedNetwork) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := WithTenant(ctx, tx, tenantID); err != nil {
		return err
	}
	status := strings.TrimSpace(observed.Status)
	if status == "" {
		status = "ready"
	}
	_, err = tx.Exec(ctx, `
		UPDATE nodes SET
			network_status = $2,
			network_last_error = COALESCE($3, ''),
			network_observed_generation = $4,
			updated_at = NOW()
		WHERE id = $1::uuid`, nodeID, status, observed.Message, observed.Generation)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func UpsertEgressPolicy(ctx context.Context, pool *pgxpool.Pool, tenantID, workloadID, mode string, allowlist []string, actor AuditActor) (EgressPolicy, error) {
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode == "" {
		mode = "allow"
	}
	if mode != "allow" && mode != "deny" && mode != "allowlist" {
		return EgressPolicy{}, fmt.Errorf("mode must be allow, deny, or allowlist")
	}
	if allowlist == nil {
		allowlist = []string{}
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return EgressPolicy{}, err
	}
	defer tx.Rollback(ctx)
	if err := WithTenant(ctx, tx, tenantID); err != nil {
		return EgressPolicy{}, err
	}
	raw, _ := json.Marshal(allowlist)
	var policy EgressPolicy
	var allowText string
	err = tx.QueryRow(ctx, `
		INSERT INTO workload_egress_policies (organization_id, workload_id, mode, allowlist)
		VALUES ($1::uuid, $2::uuid, $3, $4::jsonb)
		ON CONFLICT (workload_id) DO UPDATE SET mode = EXCLUDED.mode, allowlist = EXCLUDED.allowlist, updated_at = NOW()
		RETURNING id::text, organization_id::text, workload_id::text, mode, allowlist::text`,
		tenantID, workloadID, mode, raw,
	).Scan(&policy.ID, &policy.OrganizationID, &policy.WorkloadID, &policy.Mode, &allowText)
	if err != nil {
		return EgressPolicy{}, err
	}
	_ = json.Unmarshal([]byte(allowText), &policy.Allowlist)
	_ = writeAudit(ctx, tx, tenantID, "network.egress_updated", "workload", workloadID, actor, map[string]string{"mode": mode})
	_, _ = tx.Exec(ctx, `UPDATE nodes SET network_generation = network_generation + 1, updated_at = NOW()`)
	return policy, tx.Commit(ctx)
}

func ListEgressPolicies(ctx context.Context, pool *pgxpool.Pool, tenantID string) ([]EgressPolicy, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	if err := WithTenant(ctx, tx, tenantID); err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `
		SELECT id::text, organization_id::text, workload_id::text, mode, allowlist::text
		FROM workload_egress_policies ORDER BY updated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]EgressPolicy, 0)
	for rows.Next() {
		var item EgressPolicy
		var allowText string
		if err := rows.Scan(&item.ID, &item.OrganizationID, &item.WorkloadID, &item.Mode, &allowText); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(allowText), &item.Allowlist)
		result = append(result, item)
	}
	return result, rows.Err()
}

func ListDNSRecords(ctx context.Context, pool *pgxpool.Pool, tenantID string) ([]NetworkDNSRecord, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	if err := WithTenant(ctx, tx, tenantID); err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `SELECT name, record_type, value, ttl_seconds FROM network_dns_records ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]NetworkDNSRecord, 0)
	for rows.Next() {
		var item NetworkDNSRecord
		if err := rows.Scan(&item.Name, &item.Type, &item.Value, &item.TTL); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func GetNetworkStatus(ctx context.Context, pool *pgxpool.Pool, tenantID string) (NetworkStatus, error) {
	_ = EnsureOrgNetwork(ctx, pool, tenantID)
	tx, err := pool.Begin(ctx)
	if err != nil {
		return NetworkStatus{}, err
	}
	defer tx.Rollback(ctx)
	if err := WithTenant(ctx, tx, tenantID); err != nil {
		return NetworkStatus{}, err
	}
	status := NetworkStatus{
		OverlayCIDR:  network.OverlayCIDR,
		DNSSuffix:    network.DNSSuffix,
		ApplyEnabled: NetworkApplyEnabled(),
	}
	rows, err := tx.Query(ctx, `
		SELECT id::text, name, role, status, wg_address, wg_public_key, network_status, network_last_error,
		       network_generation, network_observed_generation
		FROM nodes ORDER BY is_local DESC, name`)
	if err != nil {
		return NetworkStatus{}, err
	}
	for rows.Next() {
		var item NodeNetworkView
		if err := rows.Scan(
			&item.NodeID, &item.Name, &item.Role, &item.Status, &item.WGAddress, &item.WGPublicKey,
			&item.NetworkStatus, &item.LastError, &item.Generation, &item.ObservedGen,
		); err != nil {
			rows.Close()
			return NetworkStatus{}, err
		}
		status.NodesTotal++
		if item.NetworkStatus == "ready" {
			status.NodesReady++
		}
		status.Nodes = append(status.Nodes, item)
	}
	rows.Close()
	_ = tx.QueryRow(ctx, `SELECT COUNT(*) FROM network_dns_records`).Scan(&status.DNSRecords)
	_ = tx.QueryRow(ctx, `SELECT COUNT(*) FROM platform_certificates WHERE not_after > NOW()`).Scan(&status.Certificates)
	_ = tx.Commit(ctx)
	return status, nil
}

func BumpNetworkGeneration(ctx context.Context, pool *pgxpool.Pool, tenantID string) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := WithTenant(ctx, tx, tenantID); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE nodes SET network_generation = network_generation + 1, updated_at = NOW()`)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}
