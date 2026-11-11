package api

import (
	"context"
	"fmt"
	"strings"

	"connectrpc.com/connect/v2"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/xiron902224/norix/control-plane/internal/db"
	paasv1 "github.com/xiron902224/norix/control-plane/internal/gen/v1"
)

type WorkloadService struct{ pool *pgxpool.Pool }

func NewWorkloadService(pool *pgxpool.Pool) WorkloadService { return WorkloadService{pool: pool} }

func runtimeName(runtime paasv1.RuntimeType) (string, error) {
	values := map[paasv1.RuntimeType]string{
		paasv1.RuntimeType_RUNTIME_TYPE_CONTAINER:      "container",
		paasv1.RuntimeType_RUNTIME_TYPE_DOCKER_COMPOSE: "docker-compose",
		paasv1.RuntimeType_RUNTIME_TYPE_MICROVM:        "microvm",
		paasv1.RuntimeType_RUNTIME_TYPE_WASM:           "wasm",
		paasv1.RuntimeType_RUNTIME_TYPE_STATIC_SITE:    "static-site",
	}
	value, ok := values[runtime]
	if !ok {
		return "", invalidArgument("runtime is not supported")
	}
	return value, nil
}

func workloadFromProto(workload *paasv1.Workload) (db.Workload, error) {
	if workload == nil {
		return db.Workload{}, invalidArgument("workload is required")
	}
	runtime, err := runtimeName(workload.GetRuntime())
	if err != nil {
		return db.Workload{}, err
	}
	return db.Workload{
		ID:            workload.GetId(),
		ProjectID:     workload.GetProjectId(),
		Name:          strings.TrimSpace(workload.GetName()),
		Runtime:       runtime,
		ImageOrSource: workload.GetImageOrSource(),
		Environment:   workload.GetEnvVars(),
		CPUShares:     workload.GetCpuShares(),
		MemoryLimitMB: workload.GetMemoryLimitMb(),
		Domain:        strings.TrimSpace(workload.GetDomain()),
		ContainerPort: workload.GetContainerPort(),
	}, nil
}

func workloadToProto(workload db.Workload) *paasv1.Workload {
	status := workload.Status
	runtime := paasv1.RuntimeType_RUNTIME_TYPE_CONTAINER
	switch workload.Runtime {
	case "docker-compose":
		runtime = paasv1.RuntimeType_RUNTIME_TYPE_DOCKER_COMPOSE
	case "microvm":
		runtime = paasv1.RuntimeType_RUNTIME_TYPE_MICROVM
	case "wasm":
		runtime = paasv1.RuntimeType_RUNTIME_TYPE_WASM
	case "static-site":
		runtime = paasv1.RuntimeType_RUNTIME_TYPE_STATIC_SITE
	}
	endpoint := ""
	if workload.HostPort > 0 {
		endpoint = fmt.Sprintf("http://127.0.0.1:%d", workload.HostPort)
	}
	return &paasv1.Workload{
		Id: workload.ID, ProjectId: workload.ProjectID, Name: workload.Name, Runtime: runtime,
		ImageOrSource: workload.ImageOrSource, EnvVars: workload.Environment, CpuShares: workload.CPUShares,
		MemoryLimitMb: workload.MemoryLimitMB, Domain: workload.Domain, Status: status,
		DesiredGeneration: workload.DesiredGeneration, ObservedGeneration: workload.ObservedGeneration,
		HostPort: workload.HostPort, ContainerPort: workload.ContainerPort,
		AssignedNodeId: workload.AssignedNodeID, Endpoint: endpoint,
		LastError: workload.LastError, ContainerId: workload.ContainerID,
	}
}

func (service WorkloadService) CreateWorkload(ctx context.Context, request *paasv1.CreateWorkloadRequest) (*paasv1.CreateWorkloadResponse, error) {
	identity, ok := SessionFromContext(ctx)
	if err := requireSession(identity, ok); err != nil {
		return nil, err
	}
	if err := requireMutator(identity); err != nil {
		return nil, err
	}
	workload, err := workloadFromProto(request.GetWorkload())
	if err != nil {
		return nil, err
	}
	created, err := db.CreateWorkload(ctx, service.pool, identity.OrganizationID, workload, db.AuditActor{
		UserID: identity.UserID,
		Email:  identity.Email,
	})
	if err != nil {
		if strings.Contains(err.Error(), "required") {
			return nil, invalidArgument(err.Error())
		}
		return nil, internalError("create workload", err)
	}
	return &paasv1.CreateWorkloadResponse{Workload: workloadToProto(created)}, nil
}

func (service WorkloadService) ListWorkloads(ctx context.Context, request *paasv1.ListWorkloadsRequest) (*paasv1.ListWorkloadsResponse, error) {
	identity, ok := SessionFromContext(ctx)
	if err := requireSession(identity, ok); err != nil {
		return nil, err
	}
	if request.GetProjectId() == "" {
		return nil, invalidArgument("project id is required")
	}
	stored, err := db.ListWorkloads(ctx, service.pool, identity.OrganizationID, request.GetProjectId())
	if err != nil {
		return nil, internalError("list workloads", err)
	}
	result := make([]*paasv1.Workload, 0, len(stored))
	for _, workload := range stored {
		result = append(result, workloadToProto(workload))
	}
	return &paasv1.ListWorkloadsResponse{Workloads: result}, nil
}

func (service WorkloadService) UpdateWorkload(ctx context.Context, request *paasv1.UpdateWorkloadRequest) (*paasv1.UpdateWorkloadResponse, error) {
	identity, ok := SessionFromContext(ctx)
	if err := requireSession(identity, ok); err != nil {
		return nil, err
	}
	if err := requireMutator(identity); err != nil {
		return nil, err
	}
	workload, err := workloadFromProto(request.GetWorkload())
	if err != nil {
		return nil, err
	}
	updated, err := db.UpdateWorkload(ctx, service.pool, identity.OrganizationID, workload, db.AuditActor{
		UserID: identity.UserID,
		Email:  identity.Email,
	})
	if err != nil {
		if strings.Contains(err.Error(), "required") {
			return nil, invalidArgument(err.Error())
		}
		return nil, internalError("update workload", err)
	}
	return &paasv1.UpdateWorkloadResponse{Workload: workloadToProto(updated)}, nil
}

func (service WorkloadService) DeleteWorkload(ctx context.Context, request *paasv1.DeleteWorkloadRequest) (*paasv1.DeleteWorkloadResponse, error) {
	identity, ok := SessionFromContext(ctx)
	if err := requireSession(identity, ok); err != nil {
		return nil, err
	}
	if err := requireMutator(identity); err != nil {
		return nil, err
	}
	if request.GetId() == "" {
		return nil, invalidArgument("workload id is required")
	}
	if err := db.DeleteWorkload(ctx, service.pool, identity.OrganizationID, request.GetId(), db.AuditActor{
		UserID: identity.UserID,
		Email:  identity.Email,
	}); err != nil {
		if strings.Contains(err.Error(), "workload not found") {
			return nil, connect.NewError(connect.CodeNotFound, "workload not found")
		}
		return nil, internalError("delete workload", err)
	}
	return &paasv1.DeleteWorkloadResponse{}, nil
}

func (service WorkloadService) Deploy(ctx context.Context, request *paasv1.DeployRequest) (*paasv1.DeployResponse, error) {
	identity, ok := SessionFromContext(ctx)
	if err := requireSession(identity, ok); err != nil {
		return nil, err
	}
	if err := requireMutator(identity); err != nil {
		return nil, err
	}
	workloadID := request.GetWorkload().GetId()
	if workloadID == "" {
		return nil, invalidArgument("workload id is required")
	}
	if service.pool == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, "no runtime agent enrolled; create a join token and enroll a node first")
	}
	node, err := db.PickOnlineNode(ctx, service.pool, identity.OrganizationID)
	if err != nil {
		if strings.Contains(err.Error(), "no online agent") || strings.Contains(err.Error(), "no runtime agent enrolled") {
			return nil, connect.NewError(connect.CodeFailedPrecondition, err.Error())
		}
		return nil, internalError("pick node", err)
	}
	existing, err := db.GetWorkload(ctx, service.pool, identity.OrganizationID, workloadID)
	if err != nil {
		if strings.Contains(err.Error(), "get workload") {
			return nil, connect.NewError(connect.CodeNotFound, "workload not found")
		}
		return nil, internalError("get workload", err)
	}
	hostPort := existing.HostPort
	if hostPort <= 0 {
		hostPort, err = db.AllocateHostPort(ctx, service.pool, identity.OrganizationID)
		if err != nil {
			return nil, internalError("allocate port", err)
		}
	}
	containerPort := request.GetContainerPort()
	if containerPort <= 0 {
		containerPort = existing.ContainerPort
	}
	if containerPort <= 0 {
		containerPort = 80
	}
	deployment, _, err := db.DeployWorkload(ctx, service.pool, identity.OrganizationID, workloadID, node.ID, hostPort, containerPort, db.AuditActor{
		UserID: identity.UserID,
		Email:  identity.Email,
	})
	if err != nil {
		if strings.Contains(err.Error(), "workload not found") {
			return nil, connect.NewError(connect.CodeNotFound, "workload not found")
		}
		return nil, internalError("deploy workload", err)
	}
	return &paasv1.DeployResponse{
		DeploymentId:   deployment.ID,
		Status:         deployment.Status,
		HostPort:       deployment.HostPort,
		AssignedNodeId: deployment.NodeID,
	}, nil
}

func (service WorkloadService) GetWorkloadLogs(ctx context.Context, request *paasv1.GetWorkloadLogsRequest) (*paasv1.GetWorkloadLogsResponse, error) {
	identity, ok := SessionFromContext(ctx)
	if err := requireSession(identity, ok); err != nil {
		return nil, err
	}
	if request.GetWorkloadId() == "" {
		return nil, invalidArgument("workload id is required")
	}
	lines, err := db.ListWorkloadLogs(ctx, service.pool, identity.OrganizationID, request.GetWorkloadId(), request.GetTailLines())
	if err != nil {
		return nil, internalError("list workload logs", err)
	}
	return &paasv1.GetWorkloadLogsResponse{Lines: lines}, nil
}
