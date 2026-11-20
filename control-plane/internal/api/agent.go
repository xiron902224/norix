package api

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"time"

	"connectrpc.com/connect/v2"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/xiron902224/norix/control-plane/internal/auth"
	"github.com/xiron902224/norix/control-plane/internal/db"
	paasv1 "github.com/xiron902224/norix/control-plane/internal/gen/v1"
	"github.com/xiron902224/norix/control-plane/internal/gen/v1/paasv1connect"
	versionpkg "github.com/xiron902224/norix/control-plane/internal/version"
)

var agentLogger = slog.New(slog.NewTextHandler(os.Stdout, nil))

type AgentService struct {
	pool *pgxpool.Pool
}

func NewAgentService(pool *pgxpool.Pool) AgentService {
	return AgentService{pool: pool}
}

func (service AgentService) CreateJoinToken(ctx context.Context, request *paasv1.CreateJoinTokenRequest) (*paasv1.CreateJoinTokenResponse, error) {
	identity, ok := SessionFromContext(ctx)
	if err := requireSession(identity, ok); err != nil {
		return nil, err
	}
	if err := requireMutator(identity); err != nil {
		return nil, err
	}

	plain, hash, err := auth.NewSessionToken()
	if err != nil {
		return nil, internalError("create join token", err)
	}
	ttl := time.Duration(request.GetTtlSeconds()) * time.Second
	token, err := db.CreateJoinToken(ctx, service.pool, identity.OrganizationID, request.GetNodeName(), ttl, hash, plain, db.AuditActor{
		UserID: identity.UserID,
		Email:  identity.Email,
	})
	if err != nil {
		return nil, internalError("create join token", err)
	}
	return &paasv1.CreateJoinTokenResponse{
		Token:     token.Token,
		ExpiresAt: token.ExpiresAt.UTC().Format(time.RFC3339),
	}, nil
}

func (service AgentService) EnrollNode(ctx context.Context, request *paasv1.EnrollNodeRequest) (*paasv1.EnrollNodeResponse, error) {
	joinToken := strings.TrimSpace(request.GetJoinToken())
	if joinToken == "" {
		return nil, invalidArgument("join_token is required")
	}
	nodeToken, nodeHash, err := auth.NewSessionToken()
	if err != nil {
		return nil, internalError("create node token", err)
	}
	// Node cap is checked inside EnrollNode after the join token resolves the org (#45).
	node, organizationID, err := db.EnrollNode(
		ctx,
		service.pool,
		auth.HashSessionToken(joinToken),
		nodeHash,
		request.GetNodeName(),
		capabilitiesFromProto(request.GetCapabilities()),
	)
	if err != nil {
		if strings.Contains(err.Error(), "join token is invalid") {
			return nil, connect.NewError(connect.CodeUnauthenticated, "join token is invalid or expired")
		}
		if strings.Contains(err.Error(), "NORIX_MAX_NODES_PER_ORG") {
			return nil, invalidArgument(err.Error())
		}
		return nil, internalError("enroll node", err)
	}
	return &paasv1.EnrollNodeResponse{
		NodeId:         node.ID,
		NodeToken:      nodeToken,
		OrganizationId: organizationID,
	}, nil
}

func (service AgentService) Heartbeat(ctx context.Context, request *paasv1.HeartbeatRequest) (*paasv1.HeartbeatResponse, error) {
	node, ok := NodeFromContext(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, "node credential required")
	}
	nodeID := strings.TrimSpace(request.GetNodeId())
	if nodeID == "" {
		nodeID = node.ID
	}
	if nodeID != node.ID {
		return nil, connect.NewError(connect.CodePermissionDenied, "node_id does not match credential")
	}

	var caps *db.NodeCapabilities
	if request.GetCapabilities() != nil {
		value := capabilitiesFromProto(request.GetCapabilities())
		caps = &value
	}
	rotatedToken := ""
	if node.CredentialRotatePending {
		plain, hash, err := auth.NewSessionToken()
		if err != nil {
			return nil, internalError("rotate node token", err)
		}
		okRotate, err := db.CompleteNodeTokenRotationIfRequested(ctx, service.pool, node.OrganizationID, node.ID, hash)
		if err != nil {
			return nil, internalError("complete node token rotation", err)
		}
		if okRotate {
			rotatedToken = plain
		}
	}

	if err := db.RecordHeartbeat(
		ctx,
		service.pool,
		node.OrganizationID,
		node.ID,
		request.GetStatus(),
		caps,
		request.GetCpuUsagePercent(),
		request.GetMemoryFreeMb(),
		0,
		request.GetRunningWorkloadIds(),
	); err != nil {
		if strings.Contains(err.Error(), "node not found") {
			return nil, connect.NewError(connect.CodeNotFound, "node not found")
		}
		return nil, internalError("heartbeat", err)
	}
	if _, err := db.ReclaimDesiredWorkloads(ctx, service.pool, node.OrganizationID, node.ID); err != nil {
		return nil, internalError("reclaim desired workloads", err)
	}
	if _, err := db.ReclaimPendingBuilds(ctx, service.pool, node.OrganizationID, node.ID); err != nil {
		return nil, internalError("reclaim pending builds", err)
	}
	if observedDBs := request.GetObservedManagedDatabases(); len(observedDBs) > 0 {
		items := make([]db.ObservedManagedDatabase, 0, len(observedDBs))
		for _, item := range observedDBs {
			items = append(items, db.ObservedManagedDatabase{
				ID: item.GetId(), Status: item.GetStatus(), ContainerID: item.GetContainerId(),
				Message: item.GetMessage(), HostPort: item.GetHostPort(),
			})
		}
		if err := db.ApplyObservedManagedDatabases(ctx, service.pool, node.OrganizationID, items); err != nil {
			return nil, internalError("apply managed databases", err)
		}
	}
	_, _ = db.EnsureProvisioningDatabases(ctx, service.pool, node.OrganizationID, node.ID)
	desired, err := db.ListDesiredWorkloadsForNode(ctx, service.pool, node.OrganizationID, node.ID)
	if err != nil {
		return nil, internalError("list desired workloads", err)
	}
	items := make([]*paasv1.DesiredWorkload, 0, len(desired))
	for _, item := range desired {
		items = append(items, desiredWorkloadToProto(item))
	}
	builds, err := db.ListDesiredBuildsForNode(ctx, service.pool, node.OrganizationID, node.ID)
	if err != nil {
		return nil, internalError("list desired builds", err)
	}
	desiredBuilds := make([]*paasv1.DesiredBuild, 0, len(builds))
	for _, item := range builds {
		desiredBuilds = append(desiredBuilds, &paasv1.DesiredBuild{
			Id: item.ID, WorkloadId: item.WorkloadID, GitUrl: item.GitURL, GitRef: item.GitRef,
			DockerfilePath: item.DockerfilePath, ImageTag: item.ImageTag, BuildArgs: item.BuildArgs,
			Registry: item.Registry,
		})
	}
	var desiredEdge *paasv1.DesiredEdgePlan
	edgePlan, err := db.GetDesiredEdgePlan(ctx, service.pool, node.OrganizationID, node.ID)
	if err != nil {
		return nil, internalError("desired edge plan", err)
	}
	if edgePlan.Generation > 0 && len(edgePlan.Routes) > 0 {
		routes := make([]*paasv1.DesiredEdgeRoute, 0, len(edgePlan.Routes))
		for _, route := range edgePlan.Routes {
			routes = append(routes, &paasv1.DesiredEdgeRoute{
				DomainId: route.DomainID, Hostname: route.Hostname, Upstream: route.Upstream,
				TlsMode: route.TLSMode, EdgeGeneration: route.EdgeGeneration, Action: route.Action,
			})
		}
		desiredEdge = &paasv1.DesiredEdgePlan{
			Generation: edgePlan.Generation, Routes: routes,
			AcmeCa: edgePlan.ACMECA, CaddyAdminUrl: edgePlan.CaddyAdminURL,
		}
	}
	var desiredNetwork *paasv1.DesiredNetworkPlan
	networkPlan, err := db.GetDesiredNetworkPlan(ctx, service.pool, node.OrganizationID, node.ID)
	if err != nil {
		return nil, internalError("desired network plan", err)
	}
	if networkPlan.Generation > 0 {
		desiredNetwork = desiredNetworkToProto(networkPlan)
	}
	var desiredWasm *paasv1.DesiredWasmPlan
	wasmFns, err := db.ListWasmFunctions(ctx, service.pool, node.OrganizationID, "")
	if err != nil {
		return nil, internalError("list wasm functions", err)
	}
	if len(wasmFns) > 0 {
		fns := make([]*paasv1.DesiredWasmFunction, 0, len(wasmFns))
		for _, fn := range wasmFns {
			fns = append(fns, &paasv1.DesiredWasmFunction{
				Id: fn.ID, ProjectId: fn.ProjectID, Name: fn.Name, ImageRef: fn.ImageRef,
				FuelLimit: fn.FuelLimit, MemoryLimitMb: fn.MemoryLimitMB,
			})
		}
		desiredWasm = &paasv1.DesiredWasmPlan{
			Generation: time.Now().UTC().Unix(),
			Functions:  fns,
		}
	}
	desiredAgentVersion := strings.TrimSpace(node.DesiredAgentVersion)
	// Prefer fresh DB value after heartbeat cleared a match.
	if refreshed, err := db.GetNode(ctx, service.pool, node.OrganizationID, node.ID); err == nil {
		desiredAgentVersion = strings.TrimSpace(refreshed.DesiredAgentVersion)
	}
	desiredDBs, err := db.ListDesiredManagedDatabasesForNode(ctx, service.pool, node.OrganizationID, node.ID)
	if err != nil {
		return nil, internalError("list desired managed databases", err)
	}
	if _, err := db.ClaimComposeStacksForNode(ctx, service.pool, node.OrganizationID, node.ID); err != nil {
		return nil, internalError("claim compose stacks", err)
	}
	stacks, err := db.ListDesiredComposeStacksForNode(ctx, service.pool, node.OrganizationID, node.ID)
	if err != nil {
		return nil, internalError("list desired compose stacks", err)
	}
	desiredStacks := make([]*paasv1.DesiredComposeStack, 0, len(stacks))
	for _, stack := range stacks {
		desiredStacks = append(desiredStacks, &paasv1.DesiredComposeStack{
			Id: stack.ID, ProjectId: stack.ProjectID, Name: stack.Name, GitUrl: stack.GitURL, GitRef: stack.GitRef,
			ComposePath: stack.ComposePath, ComposeProject: stack.ComposeProject(),
			Action: stack.DesiredAction, Generation: stack.DesiredGeneration,
			PublicServices: stack.PublicServices, Env: stack.Env, Domains: stack.Domains,
			PurgeVolumes: stack.PurgeVolumes,
		})
	}
	desiredManaged := make([]*paasv1.DesiredManagedDatabase, 0, len(desiredDBs))
	for _, item := range desiredDBs {
		desiredManaged = append(desiredManaged, &paasv1.DesiredManagedDatabase{
			Id: item.ID, ProjectId: item.ProjectID, Name: item.Name, Engine: item.Engine,
			Image: item.Image, EnvVars: item.EnvVars, Command: item.Command,
			HostPort: item.HostPort, ContainerPort: item.ContainerPort,
			VolumeName: item.VolumeName, VolumeMountPath: item.VolumeMountPath, Action: item.Action,
		})
	}
	return &paasv1.HeartbeatResponse{
		NodeId: node.ID, DesiredWorkloads: items, DesiredBuilds: desiredBuilds,
		DesiredEdge: desiredEdge, DesiredNetwork: desiredNetwork, DesiredWasm: desiredWasm,
		DesiredAgentVersion:     desiredAgentVersion,
		RotatedNodeToken:        rotatedToken,
		DesiredManagedDatabases: desiredManaged,
		DesiredComposeStacks:    desiredStacks,
	}, nil
}

func (service AgentService) ReportComposeStack(ctx context.Context, request *paasv1.ReportComposeStackRequest) (*paasv1.ReportComposeStackResponse, error) {
	node, ok := NodeFromContext(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, "node credential required")
	}
	if nodeID := strings.TrimSpace(request.GetNodeId()); nodeID != "" && nodeID != node.ID {
		return nil, connect.NewError(connect.CodePermissionDenied, "node_id does not match credential")
	}
	observed := request.GetStack()
	if observed == nil || strings.TrimSpace(observed.GetId()) == "" {
		return nil, invalidArgument("stack id is required")
	}
	services := make([]db.ComposeStackService, 0, len(observed.GetServices()))
	for _, svc := range observed.GetServices() {
		services = append(services, db.ComposeStackService{
			Name: svc.GetName(), State: svc.GetState(), Health: svc.GetHealth(),
			ExitCode: svc.GetExitCode(), Image: svc.GetImage(), Ports: svc.GetPorts(),
			ContainerPort: svc.GetContainerPort(),
		})
	}
	volumes := make([]db.ComposeVolume, 0, len(observed.GetVolumes()))
	for _, volume := range observed.GetVolumes() {
		volumes = append(volumes, db.ComposeVolume{Name: volume.GetName(), Kind: volume.GetKind(), Source: volume.GetSource()})
	}
	logs := make([]db.ComposeStackLog, 0, len(observed.GetLogLines()))
	for _, line := range observed.GetLogLines() {
		logs = append(logs, db.ComposeStackLog{Service: line.GetService(), Line: line.GetLine()})
	}
	err := db.ApplyObservedComposeStack(ctx, service.pool, node.OrganizationID, node.ID, db.ObservedComposeStack{
		ID: observed.GetId(), Status: observed.GetStatus(), ObservedGeneration: observed.GetObservedGeneration(),
		Message: observed.GetMessage(), Services: services, Logs: logs,
		GeneratedEnv: observed.GetGeneratedEnv(), Volumes: volumes,
	})
	if errors.Is(err, db.ErrComposeStackNotFound) {
		return nil, connect.NewError(connect.CodeNotFound, "compose stack not assigned to this node")
	}
	if err != nil {
		return nil, internalError("apply observed compose stack", err)
	}
	return &paasv1.ReportComposeStackResponse{}, nil
}

func (service AgentService) ReportWasm(ctx context.Context, request *paasv1.ReportWasmRequest) (*paasv1.ReportWasmResponse, error) {
	node, ok := NodeFromContext(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, "node credential required")
	}
	nodeID := strings.TrimSpace(request.GetNodeId())
	if nodeID == "" {
		nodeID = node.ID
	}
	if nodeID != node.ID {
		return nil, connect.NewError(connect.CodePermissionDenied, "node_id does not match credential")
	}
	wasm := request.GetWasm()
	if wasm == nil {
		return nil, invalidArgument("wasm is required")
	}
	observed := make([]db.ObservedWasmFunction, 0, len(wasm.GetFunctions()))
	for _, item := range wasm.GetFunctions() {
		observed = append(observed, db.ObservedWasmFunction{
			ID: item.GetId(), Status: item.GetStatus(),
		})
	}
	if err := db.ApplyObservedWasm(ctx, service.pool, node.OrganizationID, observed); err != nil {
		return nil, internalError("report wasm", err)
	}
	return &paasv1.ReportWasmResponse{}, nil
}

func (service AgentService) ReportEdge(ctx context.Context, request *paasv1.ReportEdgeRequest) (*paasv1.ReportEdgeResponse, error) {
	node, ok := NodeFromContext(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, "node credential required")
	}
	nodeID := strings.TrimSpace(request.GetNodeId())
	if nodeID == "" {
		nodeID = node.ID
	}
	if nodeID != node.ID {
		return nil, connect.NewError(connect.CodePermissionDenied, "node_id does not match credential")
	}
	edge := request.GetEdge()
	if edge == nil {
		return nil, invalidArgument("edge is required")
	}
	if err := db.ApplyObservedEdge(ctx, service.pool, node.OrganizationID, db.ObservedEdge{
		Generation:    edge.GetGeneration(),
		Status:        edge.GetStatus(),
		Message:       edge.GetMessage(),
		LogLines:      edge.GetLogLines(),
		RouteStatuses: edge.GetRouteStatuses(),
	}, db.AuditActor{Email: "node:" + node.ID}); err != nil {
		return nil, internalError("report edge", err)
	}
	return &paasv1.ReportEdgeResponse{}, nil
}

func (service AgentService) ReportNetwork(ctx context.Context, request *paasv1.ReportNetworkRequest) (*paasv1.ReportNetworkResponse, error) {
	node, ok := NodeFromContext(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, "node credential required")
	}
	nodeID := strings.TrimSpace(request.GetNodeId())
	if nodeID == "" {
		nodeID = node.ID
	}
	if nodeID != node.ID {
		return nil, connect.NewError(connect.CodePermissionDenied, "node_id does not match credential")
	}
	network := request.GetNetwork()
	if network == nil {
		return nil, invalidArgument("network is required")
	}
	if err := db.ApplyObservedNetwork(ctx, service.pool, node.OrganizationID, node.ID, db.ObservedNetwork{
		Generation: network.GetGeneration(),
		Status:     network.GetStatus(),
		Message:    network.GetMessage(),
		LogLines:   network.GetLogLines(),
	}); err != nil {
		return nil, internalError("report network", err)
	}
	return &paasv1.ReportNetworkResponse{}, nil
}

func (service AgentService) GetNetworkStatus(ctx context.Context, _ *paasv1.GetNetworkStatusRequest) (*paasv1.GetNetworkStatusResponse, error) {
	identity, ok := SessionFromContext(ctx)
	if err := requireSession(identity, ok); err != nil {
		return nil, err
	}
	status, err := db.GetNetworkStatus(ctx, service.pool, identity.OrganizationID)
	if err != nil {
		return nil, internalError("network status", err)
	}
	records, err := db.ListDNSRecords(ctx, service.pool, identity.OrganizationID)
	if err != nil {
		return nil, internalError("list dns records", err)
	}
	nodes := make([]*paasv1.NodeNetworkStatus, 0, len(status.Nodes))
	for _, item := range status.Nodes {
		nodes = append(nodes, &paasv1.NodeNetworkStatus{
			NodeId: item.NodeID, Name: item.Name, Role: item.Role, Status: item.Status,
			WgAddress: item.WGAddress, WgPublicKey: item.WGPublicKey,
			NetworkStatus: item.NetworkStatus, LastError: item.LastError,
			Generation: item.Generation, ObservedGeneration: item.ObservedGen,
		})
	}
	dns := make([]*paasv1.DesiredNetworkDNSRecord, 0, len(records))
	for _, item := range records {
		dns = append(dns, &paasv1.DesiredNetworkDNSRecord{
			Name: item.Name, Type: item.Type, Value: item.Value, TtlSeconds: item.TTL,
		})
	}
	// apply_enabled mirrors NORIX_APPLY_WIREGUARD on the control plane (false => mesh dry-run).
	return &paasv1.GetNetworkStatusResponse{
		NodesReady: int32(status.NodesReady), NodesTotal: int32(status.NodesTotal),
		DnsRecords: int32(status.DNSRecords), Certificates: int32(status.Certificates),
		OverlayCidr: status.OverlayCIDR, DnsSuffix: status.DNSSuffix,
		ApplyEnabled: status.ApplyEnabled, Nodes: nodes, Records: dns,
	}, nil
}

func (service AgentService) UpsertEgressPolicy(ctx context.Context, request *paasv1.UpsertEgressPolicyRequest) (*paasv1.UpsertEgressPolicyResponse, error) {
	identity, ok := SessionFromContext(ctx)
	if err := requireSession(identity, ok); err != nil {
		return nil, err
	}
	if err := requireMutator(identity); err != nil {
		return nil, err
	}
	workloadID := strings.TrimSpace(request.GetWorkloadId())
	if workloadID == "" {
		return nil, invalidArgument("workload_id is required")
	}
	policy, err := db.UpsertEgressPolicy(ctx, service.pool, identity.OrganizationID, workloadID, request.GetMode(), request.GetAllowlist(), db.AuditActor{
		UserID: identity.UserID, Email: identity.Email,
	})
	if err != nil {
		return nil, invalidArgument(err.Error())
	}
	return &paasv1.UpsertEgressPolicyResponse{
		WorkloadId: policy.WorkloadID, Mode: policy.Mode, Allowlist: policy.Allowlist,
	}, nil
}

func (service AgentService) ListEgressPolicies(ctx context.Context, _ *paasv1.ListEgressPoliciesRequest) (*paasv1.ListEgressPoliciesResponse, error) {
	identity, ok := SessionFromContext(ctx)
	if err := requireSession(identity, ok); err != nil {
		return nil, err
	}
	policies, err := db.ListEgressPolicies(ctx, service.pool, identity.OrganizationID)
	if err != nil {
		return nil, internalError("list egress policies", err)
	}
	items := make([]*paasv1.EgressPolicy, 0, len(policies))
	for _, item := range policies {
		items = append(items, &paasv1.EgressPolicy{
			Id: item.ID, WorkloadId: item.WorkloadID, Mode: item.Mode, Allowlist: item.Allowlist,
		})
	}
	return &paasv1.ListEgressPoliciesResponse{Policies: items}, nil
}

func desiredNetworkToProto(plan db.DesiredNetworkPlan) *paasv1.DesiredNetworkPlan {
	peers := make([]*paasv1.DesiredNetworkPeer, 0, len(plan.Peers))
	for _, peer := range plan.Peers {
		peers = append(peers, &paasv1.DesiredNetworkPeer{
			NodeId: peer.NodeID, NodeName: peer.NodeName, PublicKey: peer.PublicKey,
			Address: peer.Address, Endpoint: peer.Endpoint, AllowedIps: peer.AllowedIPs,
		})
	}
	dns := make([]*paasv1.DesiredNetworkDNSRecord, 0, len(plan.DNSRecords))
	for _, item := range plan.DNSRecords {
		dns = append(dns, &paasv1.DesiredNetworkDNSRecord{
			Name: item.Name, Type: item.Type, Value: item.Value, TtlSeconds: item.TTL,
		})
	}
	certs := make([]*paasv1.DesiredNetworkCertificate, 0, len(plan.Certificates))
	for _, item := range plan.Certificates {
		certs = append(certs, &paasv1.DesiredNetworkCertificate{
			Hostname: item.Hostname, CertPem: item.CertPEM, KeyPem: item.KeyPEM,
			NotAfter: item.NotAfter.UTC().Format(time.RFC3339), Issuer: item.Issuer,
		})
	}
	return &paasv1.DesiredNetworkPlan{
		Generation: plan.Generation, Interface: plan.Interface, Address: plan.Address,
		ListenPort: plan.ListenPort, PrivateKey: plan.PrivateKey, PublicKey: plan.PublicKey,
		Peers: peers, DnsRecords: dns, Nftables: plan.Nftables, WgConfig: plan.WGConfig,
		Certificates: certs, Apply: plan.Apply,
	}
}

func (service AgentService) ReportBuild(ctx context.Context, request *paasv1.ReportBuildRequest) (*paasv1.ReportBuildResponse, error) {
	node, ok := NodeFromContext(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, "node credential required")
	}
	nodeID := strings.TrimSpace(request.GetNodeId())
	if nodeID == "" {
		nodeID = node.ID
	}
	if nodeID != node.ID {
		return nil, connect.NewError(connect.CodePermissionDenied, "node_id does not match credential")
	}
	build := request.GetBuild()
	if build == nil || build.GetId() == "" {
		return nil, invalidArgument("build is required")
	}
	if err := db.ApplyObservedBuild(ctx, service.pool, node.OrganizationID, db.ObservedBuild{
		ID:       build.GetId(),
		Status:   build.GetStatus(),
		Image:    build.GetImage(),
		Message:  build.GetMessage(),
		LogLines: build.GetLogLines(),
	}, db.AuditActor{Email: "node:" + node.ID}); err != nil {
		if strings.Contains(err.Error(), "build not found") {
			return nil, connect.NewError(connect.CodeNotFound, "build not found")
		}
		return nil, internalError("report build", err)
	}
	return &paasv1.ReportBuildResponse{}, nil
}

func (service AgentService) ReportObserved(ctx context.Context, request *paasv1.ReportObservedRequest) (*paasv1.ReportObservedResponse, error) {
	node, ok := NodeFromContext(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, "node credential required")
	}
	nodeID := strings.TrimSpace(request.GetNodeId())
	if nodeID == "" {
		nodeID = node.ID
	}
	if nodeID != node.ID {
		return nil, connect.NewError(connect.CodePermissionDenied, "node_id does not match credential")
	}
	observed := make([]db.ObservedWorkload, 0, len(request.GetWorkloads()))
	for _, item := range request.GetWorkloads() {
		observed = append(observed, db.ObservedWorkload{
			ID:                 item.GetId(),
			Status:             item.GetStatus(),
			ContainerID:        item.GetContainerId(),
			ObservedGeneration: item.GetObservedGeneration(),
			Message:            item.GetMessage(),
			HostPort:           item.GetHostPort(),
			LogLines:           item.GetLogLines(),
			Ready:              item.GetReady(),
			ProbeStatusJSON:    item.GetProbeStatusJson(),
		})
	}
	if err := db.ApplyObservedWorkloads(ctx, service.pool, node.OrganizationID, observed); err != nil {
		return nil, internalError("report observed", err)
	}
	return &paasv1.ReportObservedResponse{}, nil
}

func (service AgentService) ListNodes(ctx context.Context, _ *paasv1.ListNodesRequest) (*paasv1.ListNodesResponse, error) {
	identity, ok := SessionFromContext(ctx)
	if err := requireSession(identity, ok); err != nil {
		return nil, err
	}
	if _, err := db.EnsureLocalNode(ctx, service.pool, identity.OrganizationID, db.DetectHostCapabilities()); err != nil {
		agentLogger.Warn("ensure local node failed", "error", err)
	}
	stored, err := db.ListNodes(ctx, service.pool, identity.OrganizationID)
	if err != nil {
		return nil, internalError("list nodes", err)
	}
	nodes := make([]*paasv1.Node, 0, len(stored))
	for _, item := range stored {
		nodes = append(nodes, nodeToProto(item))
	}
	return &paasv1.ListNodesResponse{Nodes: nodes}, nil
}

func (service AgentService) ConnectStream(ctx context.Context, stream paasv1connect.AgentServiceConnectStreamServerStream) error {
	node, ok := NodeFromContext(ctx)
	if !ok {
		return connect.NewError(connect.CodeUnauthenticated, "node credential required")
	}

	for {
		request, err := stream.Receive()
		if err != nil {
			if errors.Is(err, io.EOF) || connect.CodeOf(err) == connect.CodeCanceled {
				return nil
			}
			return err
		}
		nodeID := strings.TrimSpace(request.GetNodeId())
		if nodeID == "" {
			nodeID = node.ID
		}
		if nodeID != node.ID {
			return connect.NewError(connect.CodePermissionDenied, "node_id does not match credential")
		}
		status := request.GetStatus()
		if status == "" {
			status = "online"
		}
		var caps *db.NodeCapabilities
		var cpuUsage float64
		var memoryFree int64
		if hb := request.GetHeartbeat(); hb != nil {
			value := db.NodeCapabilities{KVMLevel: hb.GetKvmLevel()}
			caps = &value
			cpuUsage = hb.GetCpuUsagePercent()
			memoryFree = hb.GetMemoryFreeMb()
		}
		if err := db.RecordHeartbeat(ctx, service.pool, node.OrganizationID, node.ID, status, caps, cpuUsage, memoryFree, 0, request.GetRunningWorkloadIds()); err != nil {
			agentLogger.Error("stream heartbeat failed", "error", err, "node_id", node.ID)
			return internalError("connect stream", err)
		}
		desired, err := db.ListDesiredWorkloadsForNode(ctx, service.pool, node.OrganizationID, node.ID)
		if err != nil {
			return internalError("list desired workloads", err)
		}
		items := make([]*paasv1.DesiredWorkload, 0, len(desired))
		for _, item := range desired {
			items = append(items, desiredWorkloadToProto(item))
		}
		if err := stream.Send(&paasv1.ConnectStreamResponse{
			NodeId:           node.ID,
			DesiredWorkloads: items,
			Command:          "reconcile",
		}); err != nil {
			return err
		}
	}
}

func capabilitiesFromProto(input *paasv1.NodeCapabilities) db.NodeCapabilities {
	if input == nil {
		return db.NodeCapabilities{}
	}
	kvm := input.GetKvmLevel()
	if kvm > 2 {
		kvm = 2
	}
	return db.NodeCapabilities{
		CPUCores:         input.GetCpuCores(),
		MemoryTotalMB:    input.GetMemoryTotalMb(),
		DockerSocket:     input.GetDockerSocket(),
		ContainerdSocket: input.GetContainerdSocket(),
		KVMLevel:         kvm,
		Hostname:         strings.TrimSpace(input.GetHostname()),
		AgentVersion:     strings.TrimSpace(input.GetAgentVersion()),
	}
}

func nodeToProto(node db.Node) *paasv1.Node {
	lastSeen := ""
	if node.LastSeenAt != nil {
		lastSeen = node.LastSeenAt.UTC().Format(time.RFC3339)
	}
	role := node.Role
	if role == "" {
		role = "worker"
	}
	zone := node.Zone
	if zone == "" {
		zone = "default"
	}
	return &paasv1.Node{
		Id:                      node.ID,
		OrganizationId:          node.OrganizationID,
		Name:                    node.Name,
		Status:                  node.Status,
		LastSeenAt:              lastSeen,
		IsLocal:                 node.IsLocal,
		CpuUsagePercent:         node.CPUUsagePercent,
		MemoryFreeMb:            node.MemoryFreeMB,
		DiskFreeMb:              node.DiskFreeMB,
		Role:                    role,
		Cordoned:                node.Cordoned,
		Draining:                node.Draining,
		Zone:                    zone,
		Labels:                  node.Labels,
		ReportedWorkloadIds:     node.ReportedWorkloadIDs,
		DesiredAgentVersion:     node.DesiredAgentVersion,
		CredentialRotatePending: node.CredentialRotatePending,
		ClientCertConfigured:    node.ClientCertConfigured,
		Capabilities: &paasv1.NodeCapabilities{
			CpuCores:         node.Capabilities.CPUCores,
			MemoryTotalMb:    node.Capabilities.MemoryTotalMB,
			DockerSocket:     node.Capabilities.DockerSocket,
			ContainerdSocket: node.Capabilities.ContainerdSocket,
			KvmLevel:         node.Capabilities.KVMLevel,
			Hostname:         node.Capabilities.Hostname,
			AgentVersion:     node.Capabilities.AgentVersion,
		},
	}
}

func (service AgentService) UpdateNodeAgent(ctx context.Context, request *paasv1.UpdateNodeAgentRequest) (*paasv1.UpdateNodeAgentResponse, error) {
	identity, ok := SessionFromContext(ctx)
	if err := requireSession(identity, ok); err != nil {
		return nil, err
	}
	if err := requireMutator(identity); err != nil {
		return nil, err
	}
	nodeID := strings.TrimSpace(request.GetNodeId())
	if nodeID == "" {
		return nil, invalidArgument("node_id is required")
	}
	version := strings.TrimSpace(request.GetVersion())
	if version == "" {
		version = versionpkg.Current()
	}
	if version == "" {
		return nil, invalidArgument("version is required (or set NORIX_VERSION on the control plane)")
	}
	node, err := db.SetDesiredAgentVersion(ctx, service.pool, identity.OrganizationID, nodeID, version, db.AuditActor{
		UserID: identity.UserID, Email: identity.Email,
	})
	if err != nil {
		return nil, invalidArgument(err.Error())
	}
	return &paasv1.UpdateNodeAgentResponse{Node: nodeToProto(node)}, nil
}

func (service AgentService) CordonNode(ctx context.Context, request *paasv1.CordonNodeRequest) (*paasv1.CordonNodeResponse, error) {
	identity, ok := SessionFromContext(ctx)
	if err := requireSession(identity, ok); err != nil {
		return nil, err
	}
	if err := requireMutator(identity); err != nil {
		return nil, err
	}
	nodeID := strings.TrimSpace(request.GetNodeId())
	if nodeID == "" {
		return nil, invalidArgument("node_id is required")
	}
	node, err := db.SetNodeCordon(ctx, service.pool, identity.OrganizationID, nodeID, request.GetCordoned(), db.AuditActor{
		UserID: identity.UserID, Email: identity.Email,
	})
	if err != nil {
		return nil, invalidArgument(err.Error())
	}
	return &paasv1.CordonNodeResponse{Node: nodeToProto(node)}, nil
}

func (service AgentService) DrainNode(ctx context.Context, request *paasv1.DrainNodeRequest) (*paasv1.DrainNodeResponse, error) {
	identity, ok := SessionFromContext(ctx)
	if err := requireSession(identity, ok); err != nil {
		return nil, err
	}
	if err := requireMutator(identity); err != nil {
		return nil, err
	}
	nodeID := strings.TrimSpace(request.GetNodeId())
	if nodeID == "" {
		return nil, invalidArgument("node_id is required")
	}
	warnings := make([]string, 0)
	if request.GetDraining() {
		names, _ := db.ListStatefulWorkloadNamesOnNode(ctx, service.pool, identity.OrganizationID, nodeID)
		for _, name := range names {
			warnings = append(warnings, fmt.Sprintf(
				"stateful workload %q has volumes and will not be auto-moved; stop or backup before retiring this node",
				name,
			))
		}
	}
	node, err := db.SetNodeDrain(ctx, service.pool, identity.OrganizationID, nodeID, request.GetDraining(), db.AuditActor{
		UserID: identity.UserID, Email: identity.Email,
	})
	if err != nil {
		return nil, invalidArgument(err.Error())
	}
	return &paasv1.DrainNodeResponse{Node: nodeToProto(node), Warnings: warnings}, nil
}

func desiredWorkloadToProto(item db.DesiredWorkload) *paasv1.DesiredWorkload {
	runtime := strings.TrimSpace(item.Runtime)
	if runtime == "" {
		runtime = "container"
	}
	return &paasv1.DesiredWorkload{
		Id: item.ID, ProjectId: item.ProjectID, Name: item.Name, Image: item.Image,
		EnvVars: item.Environment, CpuShares: item.CPUShares, MemoryLimitMb: item.MemoryLimitMB,
		HostPort: item.HostPort, ContainerPort: item.ContainerPort,
		DesiredGeneration: item.DesiredGeneration, Action: item.Action, Runtime: runtime,
		NetworkName: item.NetworkName, NetworkAlias: item.NetworkAlias,
		UpdateStrategy: item.UpdateStrategy, SurgeHostPort: item.SurgeHostPort, ReadinessPath: item.ReadinessPath,
		ProbeSpecJson:    item.ProbeSpecJSON,
		VolumeMountsJson: item.VolumeMountsJSON, VolumePendingOpJson: item.VolumePendingOpJSON,
		WorkloadId: item.WorkloadID,
	}
}

func (service AgentService) UpdateNodeSchedule(ctx context.Context, request *paasv1.UpdateNodeScheduleRequest) (*paasv1.UpdateNodeScheduleResponse, error) {
	identity, ok := SessionFromContext(ctx)
	if err := requireSession(identity, ok); err != nil {
		return nil, err
	}
	if err := requireMutator(identity); err != nil {
		return nil, err
	}
	nodeID := strings.TrimSpace(request.GetNodeId())
	if nodeID == "" {
		return nil, invalidArgument("node_id is required")
	}
	node, err := db.UpdateNodeScheduleMeta(ctx, service.pool, identity.OrganizationID, nodeID, request.GetRole(), request.GetZone(), request.GetLabels(), db.AuditActor{
		UserID: identity.UserID, Email: identity.Email,
	})
	if err != nil {
		return nil, invalidArgument(err.Error())
	}
	return &paasv1.UpdateNodeScheduleResponse{Node: nodeToProto(node)}, nil
}

func (service AgentService) RotateNodeToken(ctx context.Context, request *paasv1.RotateNodeTokenRequest) (*paasv1.RotateNodeTokenResponse, error) {
	identity, ok := SessionFromContext(ctx)
	if err := requireSession(identity, ok); err != nil {
		return nil, err
	}
	if err := requireMutator(identity); err != nil {
		return nil, err
	}
	nodeID := strings.TrimSpace(request.GetNodeId())
	if nodeID == "" {
		return nil, invalidArgument("node_id is required")
	}
	node, err := db.RequestNodeTokenRotation(ctx, service.pool, identity.OrganizationID, nodeID, db.AuditActor{
		UserID: identity.UserID, Email: identity.Email,
	})
	if err != nil {
		return nil, invalidArgument(err.Error())
	}
	return &paasv1.RotateNodeTokenResponse{
		Node:    nodeToProto(node),
		Message: "Rotation queued. The next agent heartbeat receives a new token and persists it (15m grace for the old token).",
	}, nil
}

func (service AgentService) RevokeNodeToken(ctx context.Context, request *paasv1.RevokeNodeTokenRequest) (*paasv1.RevokeNodeTokenResponse, error) {
	identity, ok := SessionFromContext(ctx)
	if err := requireSession(identity, ok); err != nil {
		return nil, err
	}
	if err := requireMutator(identity); err != nil {
		return nil, err
	}
	nodeID := strings.TrimSpace(request.GetNodeId())
	if nodeID == "" {
		return nil, invalidArgument("node_id is required")
	}
	node, err := db.RevokeNodeToken(ctx, service.pool, identity.OrganizationID, nodeID, db.AuditActor{
		UserID: identity.UserID, Email: identity.Email,
	})
	if err != nil {
		return nil, invalidArgument(err.Error())
	}
	return &paasv1.RevokeNodeTokenResponse{Node: nodeToProto(node)}, nil
}

func (service AgentService) SetNodeClientCert(ctx context.Context, request *paasv1.SetNodeClientCertRequest) (*paasv1.SetNodeClientCertResponse, error) {
	identity, ok := SessionFromContext(ctx)
	if err := requireSession(identity, ok); err != nil {
		return nil, err
	}
	if err := requireMutator(identity); err != nil {
		return nil, err
	}
	nodeID := strings.TrimSpace(request.GetNodeId())
	if nodeID == "" {
		return nil, invalidArgument("node_id is required")
	}
	node, err := db.SetNodeClientCertSHA256(ctx, service.pool, identity.OrganizationID, nodeID, request.GetCertificateSha256(), db.AuditActor{
		UserID: identity.UserID, Email: identity.Email,
	})
	if err != nil {
		return nil, invalidArgument(err.Error())
	}
	return &paasv1.SetNodeClientCertResponse{Node: nodeToProto(node)}, nil
}
