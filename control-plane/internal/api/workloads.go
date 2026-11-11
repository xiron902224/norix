package api

import (
	"context"
	"fmt"
	"strings"
	"time"

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
	sourceKind := strings.TrimSpace(workload.GetSourceKind())
	gitURL := strings.TrimSpace(workload.GetGitUrl())
	if sourceKind == "" {
		if gitURL != "" {
			sourceKind = "git"
		} else {
			sourceKind = "image"
		}
	}
	return db.Workload{
		ID:             workload.GetId(),
		ProjectID:      workload.GetProjectId(),
		Name:           strings.TrimSpace(workload.GetName()),
		Runtime:        runtime,
		ImageOrSource:  workload.GetImageOrSource(),
		Environment:    workload.GetEnvVars(),
		CPUShares:      workload.GetCpuShares(),
		MemoryLimitMB:  workload.GetMemoryLimitMb(),
		Domain:         strings.TrimSpace(workload.GetDomain()),
		ContainerPort:  workload.GetContainerPort(),
		SourceKind:     sourceKind,
		GitURL:         gitURL,
		GitRef:         strings.TrimSpace(workload.GetGitRef()),
		DockerfilePath: strings.TrimSpace(workload.GetDockerfilePath()),
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
		SourceKind: workload.SourceKind, GitUrl: workload.GitURL, GitRef: workload.GitRef,
		DockerfilePath: workload.DockerfilePath, LatestBuildId: workload.LatestBuildID,
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

func (service WorkloadService) DeployFromGit(ctx context.Context, request *paasv1.DeployFromGitRequest) (*paasv1.DeployFromGitResponse, error) {
	identity, ok := SessionFromContext(ctx)
	if err := requireSession(identity, ok); err != nil {
		return nil, err
	}
	if err := requireMutator(identity); err != nil {
		return nil, err
	}
	workloadID := strings.TrimSpace(request.GetWorkloadId())
	if workloadID == "" {
		return nil, invalidArgument("workload id is required")
	}
	gitURL := strings.TrimSpace(request.GetGitUrl())
	if gitURL == "" {
		existing, err := db.GetWorkload(ctx, service.pool, identity.OrganizationID, workloadID)
		if err != nil {
			return nil, connect.NewError(connect.CodeNotFound, "workload not found")
		}
		gitURL = existing.GitURL
	}
	if gitURL == "" {
		return nil, invalidArgument("git url is required")
	}
	build, err := db.CreateBuild(
		ctx,
		service.pool,
		identity.OrganizationID,
		workloadID,
		gitURL,
		request.GetGitRef(),
		request.GetDockerfilePath(),
		request.GetContainerPort(),
		request.GetBuildSecrets(),
		db.AuditActor{UserID: identity.UserID, Email: identity.Email},
	)
	if err != nil {
		if strings.Contains(err.Error(), "no online agent") || strings.Contains(err.Error(), "no runtime") {
			return nil, connect.NewError(connect.CodeFailedPrecondition, err.Error())
		}
		if strings.Contains(err.Error(), "workload not found") {
			return nil, connect.NewError(connect.CodeNotFound, "workload not found")
		}
		if strings.Contains(err.Error(), "required") || strings.Contains(err.Error(), "NORIX_BUILD_SECRETS_KEY") {
			return nil, invalidArgument(err.Error())
		}
		return nil, internalError("create build", err)
	}
	return &paasv1.DeployFromGitResponse{
		BuildId:    build.ID,
		WorkloadId: build.WorkloadID,
		Status:     build.Status,
	}, nil
}

func (service WorkloadService) ListBuilds(ctx context.Context, request *paasv1.ListBuildsRequest) (*paasv1.ListBuildsResponse, error) {
	identity, ok := SessionFromContext(ctx)
	if err := requireSession(identity, ok); err != nil {
		return nil, err
	}
	if request.GetWorkloadId() == "" {
		return nil, invalidArgument("workload id is required")
	}
	builds, err := db.ListBuilds(ctx, service.pool, identity.OrganizationID, request.GetWorkloadId())
	if err != nil {
		return nil, internalError("list builds", err)
	}
	result := make([]*paasv1.Build, 0, len(builds))
	for _, build := range builds {
		result = append(result, buildToProto(build))
	}
	return &paasv1.ListBuildsResponse{Builds: result}, nil
}

func (service WorkloadService) GetBuildLogs(ctx context.Context, request *paasv1.GetBuildLogsRequest) (*paasv1.GetBuildLogsResponse, error) {
	identity, ok := SessionFromContext(ctx)
	if err := requireSession(identity, ok); err != nil {
		return nil, err
	}
	if request.GetBuildId() == "" {
		return nil, invalidArgument("build id is required")
	}
	build, err := db.GetBuild(ctx, service.pool, identity.OrganizationID, request.GetBuildId())
	if err != nil {
		if strings.Contains(err.Error(), "build not found") {
			return nil, connect.NewError(connect.CodeNotFound, "build not found")
		}
		return nil, internalError("get build", err)
	}
	lines, err := db.ListBuildLogs(ctx, service.pool, identity.OrganizationID, request.GetBuildId(), request.GetTailLines())
	if err != nil {
		return nil, internalError("list build logs", err)
	}
	return &paasv1.GetBuildLogsResponse{Build: buildToProto(build), Lines: lines}, nil
}

func buildToProto(build db.Build) *paasv1.Build {
	return &paasv1.Build{
		Id: build.ID, WorkloadId: build.WorkloadID, GitUrl: build.GitURL, GitRef: build.GitRef,
		DockerfilePath: build.DockerfilePath, Status: build.Status, Image: build.Image,
		AssignedNodeId: build.AssignedNodeID, LastError: build.LastError,
		CreatedAt: build.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt: build.UpdatedAt.UTC().Format(time.RFC3339),
	}
}
