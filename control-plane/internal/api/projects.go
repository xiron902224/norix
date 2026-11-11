package api

import (
	"context"
	"strings"

	"connectrpc.com/connect/v2"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/xiron902224/norix/control-plane/internal/db"
	paasv1 "github.com/xiron902224/norix/control-plane/internal/gen/v1"
)

type ProjectService struct {
	pool *pgxpool.Pool
}

func NewProjectService(pool *pgxpool.Pool) ProjectService {
	return ProjectService{pool: pool}
}

func (service ProjectService) CreateProject(ctx context.Context, request *paasv1.CreateProjectRequest) (*paasv1.CreateProjectResponse, error) {
	identity, ok := SessionFromContext(ctx)
	if err := requireSession(identity, ok); err != nil {
		return nil, err
	}
	if err := requireMutator(identity); err != nil {
		return nil, err
	}
	if strings.TrimSpace(request.GetName()) == "" {
		return nil, invalidArgument("project name is required")
	}
	project, err := db.CreateProject(ctx, service.pool, identity.OrganizationID, request.GetName(), db.AuditActor{
		UserID: identity.UserID,
		Email:  identity.Email,
	})
	if err != nil {
		if strings.Contains(err.Error(), "project name is required") {
			return nil, invalidArgument(err.Error())
		}
		return nil, internalError("create project", err)
	}
	return &paasv1.CreateProjectResponse{Project: &paasv1.Project{Id: project.ID, OrganizationId: project.OrganizationID, Name: project.Name}}, nil
}

func (service ProjectService) ListProjects(ctx context.Context, _ *paasv1.ListProjectsRequest) (*paasv1.ListProjectsResponse, error) {
	identity, ok := SessionFromContext(ctx)
	if err := requireSession(identity, ok); err != nil {
		return nil, err
	}
	stored, err := db.ListProjects(ctx, service.pool, identity.OrganizationID)
	if err != nil {
		return nil, internalError("list projects", err)
	}
	projects := make([]*paasv1.Project, 0, len(stored))
	for _, project := range stored {
		projects = append(projects, &paasv1.Project{Id: project.ID, OrganizationId: project.OrganizationID, Name: project.Name})
	}
	return &paasv1.ListProjectsResponse{Projects: projects}, nil
}

func (service ProjectService) DeleteProject(ctx context.Context, request *paasv1.DeleteProjectRequest) (*paasv1.DeleteProjectResponse, error) {
	identity, ok := SessionFromContext(ctx)
	if err := requireSession(identity, ok); err != nil {
		return nil, err
	}
	if err := requireMutator(identity); err != nil {
		return nil, err
	}
	if request.GetId() == "" {
		return nil, invalidArgument("project id is required")
	}
	if err := db.DeleteProject(ctx, service.pool, identity.OrganizationID, request.GetId(), db.AuditActor{
		UserID: identity.UserID,
		Email:  identity.Email,
	}); err != nil {
		if strings.Contains(err.Error(), "project not found") {
			return nil, connect.NewError(connect.CodeNotFound, "project not found")
		}
		return nil, internalError("delete project", err)
	}
	return &paasv1.DeleteProjectResponse{}, nil
}
