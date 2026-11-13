package db

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/xiron902224/norix/control-plane/internal/scheduler"
)

type WorkloadReplica struct {
	ID                 string
	OrganizationID     string
	WorkloadID         string
	ReplicaIndex       int32
	AssignedNodeID     string
	HostPort           int32
	ContainerPort      int32
	Status             string
	DesiredGeneration  int64
	ObservedGeneration int64
	DesiredAction      string
	ContainerID        string
	LastError          string
}

func decodeLabels(raw []byte) map[string]string {
	out := map[string]string{}
	if len(raw) == 0 {
		return out
	}
	_ = json.Unmarshal(raw, &out)
	if out == nil {
		return map[string]string{}
	}
	return out
}

func encodeLabels(values map[string]string) []byte {
	if len(values) == 0 {
		return []byte(`{}`)
	}
	raw, err := json.Marshal(values)
	if err != nil {
		return []byte(`{}`)
	}
	return raw
}

func RequireKVMForRuntime(runtime string) bool {
	switch strings.ToLower(strings.TrimSpace(runtime)) {
	case "microvm", "runtime_type_microvm", "3":
		return true
	default:
		return false
	}
}

func RequireKVMForIsolation(mode string) bool {
	return strings.ToLower(strings.TrimSpace(mode)) == "microvm_required"
}

// ListSchedulableNodes returns online inventory with assignment pressure for scoring.
func ListSchedulableNodes(ctx context.Context, pool *pgxpool.Pool, tenantID string) ([]scheduler.Node, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	if err := WithTenant(ctx, tx, tenantID); err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `
		SELECT n.id::text, n.name, n.status, n.role, n.cordoned, n.draining, n.zone, n.labels::text, n.is_local,
		       n.cpu_cores, n.memory_total_mb, n.memory_free_mb, n.docker_socket, n.containerd_socket,
		       n.kvm_level, n.cpu_usage_percent,
		       COALESCE((
		         SELECT COUNT(*) FROM workload_replicas wr
		         WHERE wr.assigned_node_id = n.id AND wr.desired_action IN ('run', 'stop')
		       ), 0),
		       COALESCE((
		         SELECT COALESCE(SUM(w.memory_limit_mb), 0)
		         FROM workload_replicas wr
		         JOIN workloads w ON w.id = wr.workload_id
		         WHERE wr.assigned_node_id = n.id AND wr.desired_action IN ('run', 'stop')
		       ), 0)
		FROM nodes n
		ORDER BY n.is_local ASC, n.name`)
	if err != nil {
		return nil, fmt.Errorf("list schedulable nodes: %w", err)
	}
	defer rows.Close()
	result := make([]scheduler.Node, 0)
	for rows.Next() {
		var node scheduler.Node
		var labelsText string
		if err := rows.Scan(
			&node.ID, &node.Name, &node.Status, &node.Role, &node.Cordoned, &node.Draining, &node.Zone, &labelsText, &node.IsLocal,
			&node.CPUCores, &node.MemoryTotalMB, &node.MemoryFreeMB, &node.DockerSocket, &node.ContainerdSocket,
			&node.KVMLevel, &node.CPUUsagePercent, &node.AssignedCount, &node.AssignedMemoryMB,
		); err != nil {
			return nil, err
		}
		node.Labels = decodeLabels([]byte(labelsText))
		result = append(result, node)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, tx.Commit(ctx)
}

func ScheduleRequestFromWorkload(workload Workload) scheduler.Request {
	requireKVM := workload.RequireKVM || RequireKVMForRuntime(workload.Runtime) || RequireKVMForIsolation(workload.IsolationMode)
	strategy := workload.ScheduleStrategy
	if strategy == "" {
		strategy = "spread"
	}
	return scheduler.Request{
		WorkloadID:         workload.ID,
		MemoryLimitMB:      workload.MemoryLimitMB,
		RequireKVM:         requireKVM,
		Strategy:           strategy,
		PreferredZone:      workload.PreferredZone,
		AffinityLabels:     workload.AffinityLabels,
		AntiAffinityLabels: workload.AntiAffinityLabels,
		PreferRemote:       true,
	}
}

// PickScheduledNode selects a node via filter+score. Falls back to ensuring local node.
func PickScheduledNode(ctx context.Context, pool *pgxpool.Pool, tenantID string, req scheduler.Request) (Node, error) {
	candidates, err := ListSchedulableNodes(ctx, pool, tenantID)
	if err != nil {
		return Node{}, err
	}
	if req.RequireKVM {
		hasKVM := false
		for _, node := range candidates {
			if node.KVMLevel >= 1 {
				hasKVM = true
				break
			}
		}
		if !hasKVM && len(candidates) > 0 {
			return Node{}, fmt.Errorf("unschedulable: microvm_required but no node with KVM")
		}
	}
	if len(candidates) == 0 {
		local, ensureErr := EnsureLocalNode(ctx, pool, tenantID, DetectHostCapabilities())
		if ensureErr != nil {
			return Node{}, fmt.Errorf("no nodes enrolled: %w", ensureErr)
		}
		candidates, err = ListSchedulableNodes(ctx, pool, tenantID)
		if err != nil {
			return Node{}, err
		}
		_ = local
	}
	picked, err := scheduler.Select(candidates, req)
	if err != nil {
		// Last resort: ensure local and retry once if it has a runtime.
		local, ensureErr := EnsureLocalNode(ctx, pool, tenantID, DetectHostCapabilities())
		if ensureErr != nil {
			return Node{}, err
		}
		if !local.Capabilities.DockerSocket && !local.Capabilities.ContainerdSocket {
			return Node{}, err
		}
		candidates, listErr := ListSchedulableNodes(ctx, pool, tenantID)
		if listErr != nil {
			return Node{}, err
		}
		picked, err = scheduler.Select(candidates, req)
		if err != nil {
			return Node{}, err
		}
	}
	return GetNode(ctx, pool, tenantID, picked.ID)
}

func GetNode(ctx context.Context, pool *pgxpool.Pool, tenantID, nodeID string) (Node, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return Node{}, err
	}
	defer tx.Rollback(ctx)
	if err := WithTenant(ctx, tx, tenantID); err != nil {
		return Node{}, err
	}
	var node Node
	var labelsText string
	err = tx.QueryRow(ctx, `
		SELECT id::text, organization_id::text, name, status, last_seen_at, is_local,
		       cpu_cores, memory_total_mb, docker_socket, containerd_socket,
		       kvm_level, hostname, agent_version,
		       cpu_usage_percent, memory_free_mb, disk_free_mb,
		       role, cordoned, draining, zone, labels::text
		FROM nodes WHERE id = $1::uuid`, nodeID).Scan(
		&node.ID, &node.OrganizationID, &node.Name, &node.Status, &node.LastSeenAt, &node.IsLocal,
		&node.Capabilities.CPUCores, &node.Capabilities.MemoryTotalMB,
		&node.Capabilities.DockerSocket, &node.Capabilities.ContainerdSocket,
		&node.Capabilities.KVMLevel, &node.Capabilities.Hostname, &node.Capabilities.AgentVersion,
		&node.CPUUsagePercent, &node.MemoryFreeMB, &node.DiskFreeMB,
		&node.Role, &node.Cordoned, &node.Draining, &node.Zone, &labelsText,
	)
	if err != nil {
		return Node{}, fmt.Errorf("get node: %w", err)
	}
	node.Labels = decodeLabels([]byte(labelsText))
	return node, tx.Commit(ctx)
}

func SetNodeCordon(ctx context.Context, pool *pgxpool.Pool, tenantID, nodeID string, cordoned bool, actor AuditActor) (Node, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return Node{}, err
	}
	defer tx.Rollback(ctx)
	if err := WithTenant(ctx, tx, tenantID); err != nil {
		return Node{}, err
	}
	tag, err := tx.Exec(ctx, `
		UPDATE nodes SET cordoned = $2, updated_at = NOW()
		WHERE id = $1::uuid`, nodeID, cordoned)
	if err != nil {
		return Node{}, err
	}
	if tag.RowsAffected() != 1 {
		return Node{}, fmt.Errorf("node not found")
	}
	action := "node.uncordoned"
	if cordoned {
		action = "node.cordoned"
	}
	if err := writeAudit(ctx, tx, tenantID, action, "node", nodeID, actor, nil); err != nil {
		return Node{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Node{}, err
	}
	return GetNode(ctx, pool, tenantID, nodeID)
}

func SetNodeDrain(ctx context.Context, pool *pgxpool.Pool, tenantID, nodeID string, draining bool, actor AuditActor) (Node, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return Node{}, err
	}
	defer tx.Rollback(ctx)
	if err := WithTenant(ctx, tx, tenantID); err != nil {
		return Node{}, err
	}
	tag, err := tx.Exec(ctx, `
		UPDATE nodes SET draining = $2, cordoned = CASE WHEN $2 THEN TRUE ELSE cordoned END, updated_at = NOW()
		WHERE id = $1::uuid`, nodeID, draining)
	if err != nil {
		return Node{}, err
	}
	if tag.RowsAffected() != 1 {
		return Node{}, fmt.Errorf("node not found")
	}
	action := "node.undrained"
	if draining {
		action = "node.draining"
	}
	if err := writeAudit(ctx, tx, tenantID, action, "node", nodeID, actor, nil); err != nil {
		return Node{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Node{}, err
	}
	node, err := GetNode(ctx, pool, tenantID, nodeID)
	if err != nil {
		return Node{}, err
	}
	if draining {
		if _, rescheduleErr := RescheduleOrphanReplicas(ctx, pool, tenantID); rescheduleErr != nil {
			return node, rescheduleErr
		}
	}
	return GetNode(ctx, pool, tenantID, nodeID)
}

func UpdateNodeScheduleMeta(ctx context.Context, pool *pgxpool.Pool, tenantID, nodeID, role, zone string, labels map[string]string, actor AuditActor) (Node, error) {
	role = strings.ToLower(strings.TrimSpace(role))
	if role == "" {
		role = "worker"
	}
	if role != "worker" && role != "edge" {
		return Node{}, fmt.Errorf("role must be worker or edge")
	}
	if strings.TrimSpace(zone) == "" {
		zone = "default"
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return Node{}, err
	}
	defer tx.Rollback(ctx)
	if err := WithTenant(ctx, tx, tenantID); err != nil {
		return Node{}, err
	}
	tag, err := tx.Exec(ctx, `
		UPDATE nodes SET role = $2, zone = $3, labels = $4::jsonb, updated_at = NOW()
		WHERE id = $1::uuid`, nodeID, role, zone, encodeLabels(labels))
	if err != nil {
		return Node{}, err
	}
	if tag.RowsAffected() != 1 {
		return Node{}, fmt.Errorf("node not found")
	}
	if err := writeAudit(ctx, tx, tenantID, "node.schedule_updated", "node", nodeID, actor, map[string]string{
		"role": role, "zone": zone,
	}); err != nil {
		return Node{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Node{}, err
	}
	return GetNode(ctx, pool, tenantID, nodeID)
}

func EnsureWorkloadReplicas(ctx context.Context, tx pgx.Tx, tenantID string, workload Workload) error {
	count := workload.ReplicaCount
	if count <= 0 {
		count = 1
	}
	if count > 32 {
		count = 32
	}
	for i := int32(0); i < count; i++ {
		_, err := tx.Exec(ctx, `
			INSERT INTO workload_replicas (
				organization_id, workload_id, replica_index, container_port, status
			) VALUES ($1::uuid, $2::uuid, $3, $4, 'pending')
			ON CONFLICT (workload_id, replica_index) DO NOTHING`,
			tenantID, workload.ID, i, workload.ContainerPort)
		if err != nil {
			return fmt.Errorf("ensure replica %d: %w", i, err)
		}
	}
	_, err := tx.Exec(ctx, `
		DELETE FROM workload_replicas
		WHERE workload_id = $1::uuid AND replica_index >= $2`, workload.ID, count)
	return err
}

func listReplicaIDsOnNodes(ctx context.Context, tx pgx.Tx, workloadID string) (map[string]struct{}, error) {
	rows, err := tx.Query(ctx, `
		SELECT COALESCE(assigned_node_id::text, '')
		FROM workload_replicas
		WHERE workload_id = $1::uuid AND assigned_node_id IS NOT NULL`, workloadID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]struct{}{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		if id != "" {
			out[id] = struct{}{}
		}
	}
	return out, rows.Err()
}

func allocateHostPortTx(ctx context.Context, tx pgx.Tx) (int32, error) {
	var port int32
	err := tx.QueryRow(ctx, `
		SELECT COALESCE(
			(SELECT MIN(candidate)
			 FROM generate_series(18080, 18999) candidate
			 WHERE candidate NOT IN (
				SELECT host_port FROM workloads WHERE host_port IS NOT NULL
				UNION
				SELECT host_port FROM workload_replicas WHERE host_port IS NOT NULL
			 )), 18080)`).Scan(&port)
	if err != nil {
		return 0, fmt.Errorf("allocate host port: %w", err)
	}
	return port, nil
}

func syncParentFromPrimary(ctx context.Context, tx pgx.Tx, workloadID string) error {
	_, err := tx.Exec(ctx, `
		UPDATE workloads w SET
			assigned_node_id = wr.assigned_node_id,
			host_port = wr.host_port,
			container_port = wr.container_port,
			status = CASE WHEN w.status = 'deleting' THEN 'deleting' ELSE wr.status END,
			desired_generation = wr.desired_generation,
			observed_generation = wr.observed_generation,
			desired_action = CASE WHEN w.status = 'deleting' THEN 'stop' ELSE wr.desired_action END,
			container_id = wr.container_id,
			last_error = wr.last_error,
			updated_at = NOW()
		FROM workload_replicas wr
		WHERE w.id = $1::uuid AND wr.workload_id = w.id AND wr.replica_index = 0`, workloadID)
	return err
}

// DeployWorkloadReplicas schedules all replicas for a workload deploy.
func DeployWorkloadReplicas(ctx context.Context, pool *pgxpool.Pool, tenantID, workloadID string, containerPort int32, actor AuditActor) (Deployment, Workload, error) {
	if containerPort <= 0 {
		containerPort = 80
	}
	workload, err := GetWorkload(ctx, pool, tenantID, workloadID)
	if err != nil {
		return Deployment{}, Workload{}, err
	}
	image := strings.TrimSpace(workload.ImageOrSource)
	if image == "" || image == "pending-build" || workload.Status == "building" {
		return Deployment{}, Workload{}, fmt.Errorf("image is not ready yet (git build still pending); wait for the build to finish or open Build logs")
	}
	if RequireKVMForRuntime(workload.Runtime) || RequireKVMForIsolation(workload.IsolationMode) {
		workload.RequireKVM = true
	}
	if workload.ReplicaCount <= 0 {
		workload.ReplicaCount = 1
	}
	if workload.ScheduleStrategy == "" {
		workload.ScheduleStrategy = "spread"
	}
	if workload.ContainerPort <= 0 {
		workload.ContainerPort = containerPort
	} else {
		workload.ContainerPort = containerPort
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return Deployment{}, Workload{}, err
	}
	defer tx.Rollback(ctx)
	if err := WithTenant(ctx, tx, tenantID); err != nil {
		return Deployment{}, Workload{}, err
	}

	_, _ = tx.Exec(ctx, `
		UPDATE workloads SET
			replica_count = $2,
			schedule_strategy = $3,
			require_kvm = $4,
			preferred_zone = $5,
			affinity_labels = $6::jsonb,
			anti_affinity_labels = $7::jsonb,
			container_port = $8,
			status = 'deploying',
			last_error = '',
			updated_at = NOW()
		WHERE id = $1::uuid`,
		workloadID, workload.ReplicaCount, workload.ScheduleStrategy, workload.RequireKVM,
		workload.PreferredZone, encodeLabels(workload.AffinityLabels), encodeLabels(workload.AntiAffinityLabels),
		workload.ContainerPort,
	)

	if err := EnsureWorkloadReplicas(ctx, tx, tenantID, workload); err != nil {
		return Deployment{}, Workload{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Deployment{}, Workload{}, err
	}

	// Schedule outside the long transaction so candidate queries see committed replica rows.
	for idx := int32(0); idx < workload.ReplicaCount; idx++ {
		if err := scheduleOneReplica(ctx, pool, tenantID, workload, idx); err != nil {
			_ = markWorkloadUnschedulable(ctx, pool, tenantID, workloadID, err.Error())
			return Deployment{}, Workload{}, err
		}
	}

	tx2, err := pool.Begin(ctx)
	if err != nil {
		return Deployment{}, Workload{}, err
	}
	defer tx2.Rollback(ctx)
	if err := WithTenant(ctx, tx2, tenantID); err != nil {
		return Deployment{}, Workload{}, err
	}
	if err := syncParentFromPrimary(ctx, tx2, workloadID); err != nil {
		return Deployment{}, Workload{}, err
	}
	workload, err = scanWorkload(ctx, tx2, `
		SELECT id::text, organization_id::text, project_id::text, name, runtime, image_or_source, env_vars::text,
		       cpu_shares, memory_limit_mb, COALESCE(domain, ''), status,
		       desired_generation, observed_generation, COALESCE(host_port, 0), container_port,
		       COALESCE(assigned_node_id::text, ''), container_id, last_error, desired_action,
		       source_kind, git_url, git_ref, dockerfile_path, COALESCE(latest_build_id::text, ''),
		       replica_count, schedule_strategy, require_kvm, preferred_zone, affinity_labels::text, anti_affinity_labels::text,
		       COALESCE(isolation_mode, 'container'), COALESCE(execution_runtime, 'docker')
		FROM workloads WHERE id = $1::uuid`, workloadID)
	if err != nil {
		return Deployment{}, Workload{}, err
	}
	var deployment Deployment
	err = tx2.QueryRow(ctx, `
		INSERT INTO deployments (organization_id, workload_id, node_id, status, host_port)
		VALUES ($1::uuid, $2::uuid, NULLIF($3, '')::uuid, 'deploying', $4)
		RETURNING id::text, status, COALESCE(host_port, 0), COALESCE(node_id::text, '')`,
		tenantID, workloadID, workload.AssignedNodeID, workload.HostPort,
	).Scan(&deployment.ID, &deployment.Status, &deployment.HostPort, &deployment.NodeID)
	if err != nil {
		return Deployment{}, Workload{}, fmt.Errorf("create deployment: %w", err)
	}
	if err := writeAudit(ctx, tx2, tenantID, "workload.deployed", "workload", workloadID, actor, map[string]string{
		"deployment_id": deployment.ID,
		"node_id":       workload.AssignedNodeID,
		"replicas":      fmt.Sprintf("%d", workload.ReplicaCount),
	}); err != nil {
		return Deployment{}, Workload{}, err
	}
	if err := tx2.Commit(ctx); err != nil {
		return Deployment{}, Workload{}, err
	}
	return deployment, workload, nil
}

func markWorkloadUnschedulable(ctx context.Context, pool *pgxpool.Pool, tenantID, workloadID, message string) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := WithTenant(ctx, tx, tenantID); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		UPDATE workloads SET status = 'failed', last_error = $2, updated_at = NOW()
		WHERE id = $1::uuid`, workloadID, truncate(message, 500))
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func scheduleOneReplica(ctx context.Context, pool *pgxpool.Pool, tenantID string, workload Workload, index int32) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := WithTenant(ctx, tx, tenantID); err != nil {
		return err
	}
	var replica WorkloadReplica
	var assigned string
	err = tx.QueryRow(ctx, `
		SELECT id::text, organization_id::text, workload_id::text, replica_index,
		       COALESCE(assigned_node_id::text, ''), COALESCE(host_port, 0), container_port,
		       status, desired_generation, observed_generation, desired_action, container_id, last_error
		FROM workload_replicas
		WHERE workload_id = $1::uuid AND replica_index = $2
		FOR UPDATE`, workload.ID, index).Scan(
		&replica.ID, &replica.OrganizationID, &replica.WorkloadID, &replica.ReplicaIndex,
		&assigned, &replica.HostPort, &replica.ContainerPort,
		&replica.Status, &replica.DesiredGeneration, &replica.ObservedGeneration, &replica.DesiredAction,
		&replica.ContainerID, &replica.LastError,
	)
	if err != nil {
		return fmt.Errorf("load replica: %w", err)
	}
	replica.AssignedNodeID = assigned
	avoid, err := listReplicaIDsOnNodes(ctx, tx, workload.ID)
	if err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}

	req := ScheduleRequestFromWorkload(workload)
	req.AvoidNodesWithSame = avoid
	delete(req.AvoidNodesWithSame, replica.AssignedNodeID) // may re-pick same if only option

	node, err := PickScheduledNode(ctx, pool, tenantID, req)
	if err != nil {
		return err
	}
	hostPort := replica.HostPort
	if hostPort <= 0 {
		hostPort, err = AllocateHostPort(ctx, pool, tenantID)
		if err != nil {
			return err
		}
	}

	tx2, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx2.Rollback(ctx)
	if err := WithTenant(ctx, tx2, tenantID); err != nil {
		return err
	}
	_, err = tx2.Exec(ctx, `
		UPDATE workload_replicas SET
			assigned_node_id = $2::uuid,
			host_port = $3,
			container_port = $4,
			desired_generation = desired_generation + 1,
			desired_action = 'run',
			status = 'deploying',
			last_error = '',
			updated_at = NOW()
		WHERE id = $1::uuid`, replica.ID, node.ID, hostPort, workload.ContainerPort)
	if err != nil {
		return err
	}
	return tx2.Commit(ctx)
}

// RescheduleOrphanReplicas moves replicas off offline/draining nodes.
// Online draining nodes get an ordered stop first; only after the agent reports
// stopped (or there is no container) is the replica reassigned elsewhere.
func RescheduleOrphanReplicas(ctx context.Context, pool *pgxpool.Pool, tenantID string) (int64, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	if err := WithTenant(ctx, tx, tenantID); err != nil {
		return 0, err
	}
	rows, err := tx.Query(ctx, `
		SELECT wr.id::text, wr.workload_id::text, wr.replica_index, COALESCE(wr.assigned_node_id::text, ''),
		       COALESCE(wr.host_port, 0), wr.container_port, wr.desired_action, wr.status, wr.container_id,
		       COALESCE(n.status, ''), COALESCE(n.draining, FALSE)
		FROM workload_replicas wr
		LEFT JOIN nodes n ON n.id = wr.assigned_node_id
		WHERE (
			wr.assigned_node_id IS NULL
			OR n.id IS NULL
			OR n.status <> 'online'
			OR n.draining = TRUE
		)
		AND (
			wr.desired_action IN ('run', 'stop')
			OR (wr.desired_action = 'none' AND wr.status IN ('stopped', 'failed', 'pending'))
			OR (n.draining = TRUE AND wr.status IN ('running', 'deploying', 'stopped', 'failed', 'pending'))
		)
		ORDER BY wr.updated_at`)
	if err != nil {
		return 0, err
	}
	type orphan struct {
		ID, WorkloadID, NodeID string
		Index                  int32
		HostPort, ContainerPort int32
		DesiredAction, Status, ContainerID string
		NodeStatus             string
		NodeDraining           bool
	}
	orphans := make([]orphan, 0)
	for rows.Next() {
		var item orphan
		if err := rows.Scan(
			&item.ID, &item.WorkloadID, &item.Index, &item.NodeID,
			&item.HostPort, &item.ContainerPort, &item.DesiredAction, &item.Status, &item.ContainerID,
			&item.NodeStatus, &item.NodeDraining,
		); err != nil {
			rows.Close()
			return 0, err
		}
		orphans = append(orphans, item)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}

	var moved int64
	for _, item := range orphans {
		nodeOnline := item.NodeID != "" && item.NodeStatus == "online"
		// Ordered drain: ask the still-online agent to stop before reassigning.
		if nodeOnline && item.NodeDraining {
			needsStop := item.DesiredAction == "run" ||
				(item.DesiredAction == "none" && (item.Status == "running" || item.Status == "deploying")) ||
				(item.DesiredAction != "stop" && item.ContainerID != "" && item.Status != "stopped")
			if needsStop {
				txStop, err := pool.Begin(ctx)
				if err != nil {
					continue
				}
				if err := WithTenant(ctx, txStop, tenantID); err != nil {
					_ = txStop.Rollback(ctx)
					continue
				}
				_, err = txStop.Exec(ctx, `
					UPDATE workload_replicas SET
						desired_action = 'stop',
						desired_generation = desired_generation + 1,
						updated_at = NOW()
					WHERE id = $1::uuid AND desired_action <> 'stop'`, item.ID)
				if err != nil {
					_ = txStop.Rollback(ctx)
					continue
				}
				_ = syncParentFromPrimary(ctx, txStop, item.WorkloadID)
				_ = txStop.Commit(ctx)
				continue
			}
			waiting := item.DesiredAction == "stop" && item.Status != "stopped" && item.ContainerID != ""
			if waiting {
				continue
			}
		}

		workload, err := GetWorkload(ctx, pool, tenantID, item.WorkloadID)
		if err != nil {
			continue
		}
		req := ScheduleRequestFromWorkload(workload)
		req.ExcludeNodeIDs = map[string]struct{}{}
		if item.NodeID != "" {
			req.ExcludeNodeIDs[item.NodeID] = struct{}{}
		}
		txAvoid, err := pool.Begin(ctx)
		if err != nil {
			continue
		}
		_ = WithTenant(ctx, txAvoid, tenantID)
		avoid, _ := listReplicaIDsOnNodes(ctx, txAvoid, workload.ID)
		_ = txAvoid.Rollback(ctx)
		req.AvoidNodesWithSame = avoid

		node, err := PickScheduledNode(ctx, pool, tenantID, req)
		if err != nil {
			continue
		}
		hostPort, err := AllocateHostPort(ctx, pool, tenantID)
		if err != nil {
			continue
		}
		tx2, err := pool.Begin(ctx)
		if err != nil {
			continue
		}
		if err := WithTenant(ctx, tx2, tenantID); err != nil {
			_ = tx2.Rollback(ctx)
			continue
		}
		_, err = tx2.Exec(ctx, `
			UPDATE workload_replicas SET
				assigned_node_id = $2::uuid,
				host_port = $3,
				container_id = '',
				desired_generation = desired_generation + 1,
				observed_generation = 0,
				desired_action = 'run',
				status = 'deploying',
				last_error = '',
				updated_at = NOW()
			WHERE id = $1::uuid`, item.ID, node.ID, hostPort)
		if err != nil {
			_ = tx2.Rollback(ctx)
			continue
		}
		_ = syncParentFromPrimary(ctx, tx2, item.WorkloadID)
		if err := tx2.Commit(ctx); err != nil {
			continue
		}
		moved++
	}
	return moved, nil
}

// RescheduleAllOrphans runs across every organization (bootstrap/admin pool).
func RescheduleAllOrphans(ctx context.Context, pool *pgxpool.Pool) (int64, error) {
	orgs, err := ListOrganizationIDs(ctx, pool)
	if err != nil {
		return 0, err
	}
	var total int64
	for _, orgID := range orgs {
		n, err := RescheduleOrphanReplicas(ctx, pool, orgID)
		if err != nil {
			continue
		}
		total += n
	}
	return total, nil
}
