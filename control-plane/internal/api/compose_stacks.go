package api

import (
	"context"
	"errors"
	"path"
	"strings"
	"time"

	"connectrpc.com/connect/v2"
	"github.com/xiron902224/norix/control-plane/internal/db"
	paasv1 "github.com/xiron902224/norix/control-plane/internal/gen/v1"
	"github.com/xiron902224/norix/control-plane/internal/gitssrf"
)

// validateComposePath keeps the compose file inside the cloned repository.
func validateComposePath(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "docker-compose.yml", nil
	}
	if strings.HasPrefix(value, "/") || strings.Contains(value, "\\") {
		return "", errors.New("compose path must be relative to the repository root")
	}
	clean := path.Clean(value)
	if clean == ".." || strings.HasPrefix(clean, "../") {
		return "", errors.New("compose path must stay inside the repository")
	}
	return clean, nil
}

// validateStackName allows names that are readable in the UI and in audit logs.
func validateStackName(raw string) (string, error) {
	name := strings.TrimSpace(raw)
	if name == "" || len(name) > 63 {
		return "", errors.New("stack name must be 1-63 characters")
	}
	for _, ch := range name {
		ok := (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '-' || ch == '_'
		if !ok {
			return "", errors.New("stack name may only contain letters, digits, '-' and '_'")
		}
	}
	return name, nil
}

func (service WorkloadService) CreateComposeStack(ctx context.Context, request *paasv1.CreateComposeStackRequest) (*paasv1.CreateComposeStackResponse, error) {
	identity, ok := SessionFromContext(ctx)
	if err := requireSession(identity, ok); err != nil {
		return nil, err
	}
	if err := requireMutator(identity); err != nil {
		return nil, err
	}
	projectID := strings.TrimSpace(request.GetProjectId())
	if projectID == "" {
		return nil, invalidArgument("project_id is required")
	}
	name, err := validateStackName(request.GetName())
	if err != nil {
		return nil, invalidArgument(err.Error())
	}
	gitURL := strings.TrimSpace(request.GetGitUrl())
	if err := gitssrf.ValidateGitURL(gitURL); err != nil {
		return nil, invalidArgument(err.Error())
	}
	composePath, err := validateComposePath(request.GetComposePath())
	if err != nil {
		return nil, invalidArgument(err.Error())
	}
	gitRef := strings.TrimSpace(request.GetGitRef())
	if gitRef == "" {
		gitRef = "main"
	}
	stack, err := db.CreateComposeStack(ctx, service.pool, identity.OrganizationID, db.CreateComposeStackInput{
		ProjectID: projectID, Name: name, GitURL: gitURL, GitRef: gitRef, ComposePath: composePath,
	}, db.AuditActor{UserID: identity.UserID, Email: identity.Email})
	if err != nil {
		msg := err.Error()
		if strings.Contains(msg, "already exists") || strings.Contains(msg, "project not found") {
			return nil, invalidArgument(msg)
		}
		return nil, internalError("create compose stack", err)
	}
	return &paasv1.CreateComposeStackResponse{Stack: composeStackToProto(stack)}, nil
}

func (service WorkloadService) ListComposeStacks(ctx context.Context, request *paasv1.ListComposeStacksRequest) (*paasv1.ListComposeStacksResponse, error) {
	identity, ok := SessionFromContext(ctx)
	if err := requireSession(identity, ok); err != nil {
		return nil, err
	}
	stacks, err := db.ListComposeStacks(ctx, service.pool, identity.OrganizationID, strings.TrimSpace(request.GetProjectId()))
	if err != nil {
		return nil, internalError("list compose stacks", err)
	}
	out := make([]*paasv1.ComposeStack, 0, len(stacks))
	for _, stack := range stacks {
		out = append(out, composeStackToProto(stack))
	}
	return &paasv1.ListComposeStacksResponse{Stacks: out}, nil
}

func (service WorkloadService) GetComposeStack(ctx context.Context, request *paasv1.GetComposeStackRequest) (*paasv1.GetComposeStackResponse, error) {
	identity, ok := SessionFromContext(ctx)
	if err := requireSession(identity, ok); err != nil {
		return nil, err
	}
	stackID := strings.TrimSpace(request.GetId())
	if stackID == "" {
		return nil, invalidArgument("id is required")
	}
	stack, err := db.GetComposeStack(ctx, service.pool, identity.OrganizationID, stackID)
	if errors.Is(err, db.ErrComposeStackNotFound) {
		return nil, connect.NewError(connect.CodeNotFound, "compose stack not found")
	}
	if err != nil {
		return nil, internalError("get compose stack", err)
	}
	logs, err := db.ListComposeStackLogs(ctx, service.pool, identity.OrganizationID, stackID, strings.TrimSpace(request.GetService()), int(request.GetTailLines()))
	if err != nil {
		return nil, internalError("list compose stack logs", err)
	}
	lines := make([]*paasv1.ComposeStackLogLine, 0, len(logs))
	for _, line := range logs {
		lines = append(lines, &paasv1.ComposeStackLogLine{
			Service: line.Service, Line: line.Line, CreatedAt: line.CreatedAt.UTC().Format(time.RFC3339),
		})
	}
	return &paasv1.GetComposeStackResponse{Stack: composeStackToProto(stack), Logs: lines}, nil
}

func (service WorkloadService) ComposeStackAction(ctx context.Context, request *paasv1.ComposeStackActionRequest) (*paasv1.ComposeStackActionResponse, error) {
	identity, ok := SessionFromContext(ctx)
	if err := requireSession(identity, ok); err != nil {
		return nil, err
	}
	if err := requireMutator(identity); err != nil {
		return nil, err
	}
	stackID := strings.TrimSpace(request.GetId())
	if stackID == "" {
		return nil, invalidArgument("id is required")
	}
	if _, err := db.ComposeStackActionDesired(request.GetAction()); err != nil {
		return nil, invalidArgument(err.Error())
	}
	stack, err := db.SetComposeStackAction(ctx, service.pool, identity.OrganizationID, stackID, request.GetAction(),
		db.AuditActor{UserID: identity.UserID, Email: identity.Email})
	if errors.Is(err, db.ErrComposeStackNotFound) {
		return nil, connect.NewError(connect.CodeNotFound, "compose stack not found")
	}
	if err != nil {
		return nil, internalError("compose stack action", err)
	}
	return &paasv1.ComposeStackActionResponse{Stack: composeStackToProto(stack)}, nil
}

func composeStackToProto(stack db.ComposeStack) *paasv1.ComposeStack {
	services := make([]*paasv1.ComposeStackService, 0, len(stack.Services))
	for _, svc := range stack.Services {
		services = append(services, &paasv1.ComposeStackService{
			Name: svc.Name, State: svc.State, Health: svc.Health, ExitCode: svc.ExitCode, Image: svc.Image, Ports: svc.Ports,
		})
	}
	out := &paasv1.ComposeStack{
		Id: stack.ID, ProjectId: stack.ProjectID, Name: stack.Name, GitUrl: stack.GitURL, GitRef: stack.GitRef,
		ComposePath: stack.ComposePath, AssignedNodeId: stack.AssignedNodeID, DesiredAction: stack.DesiredAction,
		DesiredGeneration: stack.DesiredGeneration, ObservedGeneration: stack.ObservedGeneration,
		Status: stack.Status, Message: stack.Message, Services: services,
	}
	if stack.ID != "" {
		out.ComposeProject = stack.ComposeProject()
	}
	if !stack.CreatedAt.IsZero() {
		out.CreatedAt = stack.CreatedAt.UTC().Format(time.RFC3339)
	}
	if !stack.UpdatedAt.IsZero() {
		out.UpdatedAt = stack.UpdatedAt.UTC().Format(time.RFC3339)
	}
	return out
}
