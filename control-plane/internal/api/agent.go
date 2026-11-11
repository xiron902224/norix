package api

import (
	"context"
	"errors"
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
	desired, err := db.ListDesiredWorkloadsForNode(ctx, service.pool, node.OrganizationID, node.ID)
	if err != nil {
		return nil, internalError("list desired workloads", err)
	}
	items := make([]*paasv1.DesiredWorkload, 0, len(desired))
	for _, item := range desired {
		items = append(items, &paasv1.DesiredWorkload{
			Id: item.ID, ProjectId: item.ProjectID, Name: item.Name, Image: item.Image,
			EnvVars: item.Environment, CpuShares: item.CPUShares, MemoryLimitMb: item.MemoryLimitMB,
			HostPort: item.HostPort, ContainerPort: item.ContainerPort,
			DesiredGeneration: item.DesiredGeneration, Action: item.Action,
		})
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
	return &paasv1.HeartbeatResponse{
		NodeId: node.ID, DesiredWorkloads: items, DesiredBuilds: desiredBuilds, DesiredEdge: desiredEdge,
	}, nil
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
		if err := db.RecordHeartbeat(ctx, service.pool, node.OrganizationID, node.ID, status, caps, cpuUsage, memoryFree, 0); err != nil {
			agentLogger.Error("stream heartbeat failed", "error", err, "node_id", node.ID)
			return internalError("connect stream", err)
		}
		desired, err := db.ListDesiredWorkloadsForNode(ctx, service.pool, node.OrganizationID, node.ID)
		if err != nil {
			return internalError("list desired workloads", err)
		}
		items := make([]*paasv1.DesiredWorkload, 0, len(desired))
		for _, item := range desired {
			items = append(items, &paasv1.DesiredWorkload{
				Id: item.ID, ProjectId: item.ProjectID, Name: item.Name, Image: item.Image,
				EnvVars: item.Environment, CpuShares: item.CPUShares, MemoryLimitMb: item.MemoryLimitMB,
				HostPort: item.HostPort, ContainerPort: item.ContainerPort,
				DesiredGeneration: item.DesiredGeneration, Action: item.Action,
			})
		}
		if err := stream.Send(&paasv1.ConnectStreamResponse{
			NodeId:            node.ID,
			DesiredWorkloads:  items,
			Command:           "reconcile",
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
	return &paasv1.Node{
		Id:             node.ID,
		OrganizationId: node.OrganizationID,
		Name:           node.Name,
		Status:         node.Status,
		LastSeenAt:     lastSeen,
		IsLocal:        node.IsLocal,
		CpuUsagePercent: node.CPUUsagePercent,
		MemoryFreeMb:    node.MemoryFreeMB,
		DiskFreeMb:      node.DiskFreeMB,
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
