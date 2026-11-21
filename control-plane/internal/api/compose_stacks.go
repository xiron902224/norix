package api

import (
	"context"
	"errors"
	"fmt"
	"path"
	"strings"
	"time"
	"unicode"

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

// validateStackName allows a display name. The docker compose project is
// derived from the stack id, so spaces and letters outside ASCII are safe.
func validateStackName(raw string) (string, error) {
	name := strings.TrimSpace(raw)
	if name == "" || len([]rune(name)) > 63 {
		return "", errors.New("stack name must be 1-63 characters")
	}
	for _, ch := range name {
		if unicode.IsLetter(ch) || unicode.IsDigit(ch) || ch == ' ' || ch == '-' || ch == '_' || ch == '.' {
			continue
		}
		return "", errors.New("stack name may only contain letters, digits, spaces, '-', '_' and '.'")
	}
	return name, nil
}

func validateServiceName(raw string) (string, error) {
	name := strings.TrimSpace(raw)
	if name == "" || len(name) > 63 {
		return "", errors.New("service name must be 1-63 characters")
	}
	for i, ch := range name {
		ok := (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '_' || ch == '-' || ch == '.'
		if ch == '.' || ch == '-' {
			ok = i > 0
		}
		if !ok {
			return "", errors.New("service name may only contain letters, digits, '_', '-' and '.'")
		}
	}
	return name, nil
}

func validateStackVarName(raw string) (string, error) {
	name := strings.TrimSpace(raw)
	if name == "" || len(name) > 128 {
		return "", errors.New("variable name must be 1-128 characters")
	}
	for i, ch := range name {
		ok := (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || ch == '_' || (i > 0 && ch >= '0' && ch <= '9')
		if !ok {
			return "", fmt.Errorf("invalid variable name %q", name)
		}
	}
	return name, nil
}

func validateStackHostname(raw string) (string, error) {
	host := db.NormalizeHostname(raw)
	if host == "" || len(host) > 253 || strings.ContainsAny(host, " /\\") {
		return "", errors.New("hostname is required")
	}
	return host, nil
}

func normalizeStackSpec(public []string, domains, variables map[string]string) ([]string, map[string]string, map[string]string, error) {
	outPublic := make([]string, 0, len(public))
	seen := map[string]bool{}
	for _, name := range public {
		clean, err := validateServiceName(name)
		if err != nil {
			return nil, nil, nil, err
		}
		if seen[clean] {
			continue
		}
		seen[clean] = true
		outPublic = append(outPublic, clean)
	}
	outDomains := map[string]string{}
	for service, host := range domains {
		clean, err := validateServiceName(service)
		if err != nil {
			return nil, nil, nil, err
		}
		hostname, err := validateStackHostname(host)
		if err != nil {
			return nil, nil, nil, err
		}
		outDomains[clean] = hostname
	}
	outVars := map[string]string{}
	for name, value := range variables {
		clean, err := validateStackVarName(name)
		if err != nil {
			return nil, nil, nil, err
		}
		if strings.ContainsAny(value, "\n\r\x00") {
			return nil, nil, nil, fmt.Errorf("variable %s must be a single line", clean)
		}
		outVars[clean] = value
	}
	return outPublic, outDomains, outVars, nil
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
	public, domains, variables, err := normalizeStackSpec(request.GetPublicServices(), request.GetDomains(), request.GetVariables())
	if err != nil {
		return nil, invalidArgument(err.Error())
	}
	stack, err := db.CreateComposeStack(ctx, service.pool, identity.OrganizationID, db.CreateComposeStackInput{
		ProjectID: projectID, Name: name, GitURL: gitURL, GitRef: gitRef, ComposePath: composePath,
		PublicServices: public, Domains: domains, Variables: variables,
	}, db.AuditActor{UserID: identity.UserID, Email: identity.Email})
	if err != nil {
		msg := err.Error()
		if strings.Contains(msg, "already exists") || strings.Contains(msg, "project not found") {
			return nil, invalidArgument(msg)
		}
		return nil, internalError("create compose stack", err)
	}
	NotifyDesiredChanged(stack.AssignedNodeID)
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
	stack, err := db.SetComposeStackAction(ctx, service.pool, identity.OrganizationID, stackID, request.GetAction(), request.GetPurgeVolumes(),
		db.AuditActor{UserID: identity.UserID, Email: identity.Email})
	if errors.Is(err, db.ErrComposeStackNotFound) {
		return nil, connect.NewError(connect.CodeNotFound, "compose stack not found")
	}
	if err != nil {
		return nil, internalError("compose stack action", err)
	}
	NotifyDesiredChanged(stack.AssignedNodeID)
	return &paasv1.ComposeStackActionResponse{Stack: composeStackToProto(stack)}, nil
}

func (service WorkloadService) UpdateComposeStack(ctx context.Context, request *paasv1.UpdateComposeStackRequest) (*paasv1.UpdateComposeStackResponse, error) {
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
	public, domains, variables, err := normalizeStackSpec(request.GetPublicServices(), request.GetDomains(), request.GetVariables())
	if err != nil {
		return nil, invalidArgument(err.Error())
	}
	stack, err := db.UpdateComposeStackSpec(ctx, service.pool, identity.OrganizationID, stackID, public, domains, variables,
		db.AuditActor{UserID: identity.UserID, Email: identity.Email})
	if errors.Is(err, db.ErrComposeStackNotFound) {
		return nil, connect.NewError(connect.CodeNotFound, "compose stack not found")
	}
	if err != nil {
		return nil, internalError("update compose stack", err)
	}
	NotifyDesiredChanged(stack.AssignedNodeID)
	return &paasv1.UpdateComposeStackResponse{Stack: composeStackToProto(stack)}, nil
}

func composeStackToProto(stack db.ComposeStack) *paasv1.ComposeStack {
	services := make([]*paasv1.ComposeStackService, 0, len(stack.Services))
	for _, svc := range stack.Services {
		services = append(services, &paasv1.ComposeStackService{
			Name: svc.Name, State: svc.State, Health: svc.Health, ExitCode: svc.ExitCode, Image: svc.Image, Ports: svc.Ports,
			ContainerPort: svc.ContainerPort,
		})
	}
	volumes := make([]*paasv1.ComposeVolume, 0, len(stack.Volumes))
	for _, volume := range stack.Volumes {
		volumes = append(volumes, &paasv1.ComposeVolume{Name: volume.Name, Kind: volume.Kind, Source: volume.Source})
	}
	out := &paasv1.ComposeStack{
		Id: stack.ID, ProjectId: stack.ProjectID, Name: stack.Name, GitUrl: stack.GitURL, GitRef: stack.GitRef,
		ComposePath: stack.ComposePath, AssignedNodeId: stack.AssignedNodeID, DesiredAction: stack.DesiredAction,
		DesiredGeneration: stack.DesiredGeneration, ObservedGeneration: stack.ObservedGeneration,
		Status: stack.Status, Message: stack.Message, Services: services,
		PublicServices: stack.PublicServices, Domains: stack.Domains, VariableNames: stack.VariableNames,
		Volumes: volumes, PurgeVolumes: stack.PurgeVolumes,
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
