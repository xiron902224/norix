package api

import (
	"context"
	"fmt"
	"path"
	"strings"

	"connectrpc.com/connect/v2"
	"github.com/xiron902224/norix/control-plane/internal/composeparse"
	"github.com/xiron902224/norix/control-plane/internal/db"
	paasv1 "github.com/xiron902224/norix/control-plane/internal/gen/v1"
	"github.com/xiron902224/norix/control-plane/internal/gitssrf"
)

func (service WorkloadService) AnalyzeComposeFromGit(ctx context.Context, request *paasv1.AnalyzeComposeFromGitRequest) (*paasv1.AnalyzeComposeFromGitResponse, error) {
	identity, ok := SessionFromContext(ctx)
	if err := requireSession(identity, ok); err != nil {
		return nil, err
	}
	gitURL := strings.TrimSpace(request.GetGitUrl())
	if gitURL == "" {
		return nil, invalidArgument("git url is required")
	}
	stack, err := db.AnalyzeComposeFromGit(gitURL, request.GetGitRef(), request.GetComposePath())
	if err != nil {
		return nil, invalidArgument(err.Error())
	}
	return &paasv1.AnalyzeComposeFromGitResponse{
		ComposeFile: stack.ComposeFile,
		Services:    composeServicesToProto(stack.Services),
	}, nil
}

func (service WorkloadService) DeployComposeStack(ctx context.Context, request *paasv1.DeployComposeStackRequest) (*paasv1.DeployComposeStackResponse, error) {
	identity, ok := SessionFromContext(ctx)
	if err := requireSession(identity, ok); err != nil {
		return nil, err
	}
	if err := requireMutator(identity); err != nil {
		return nil, err
	}
	projectID := strings.TrimSpace(request.GetProjectId())
	gitURL := strings.TrimSpace(request.GetGitUrl())
	stackName := strings.TrimSpace(request.GetStackName())
	if projectID == "" || gitURL == "" || stackName == "" {
		return nil, invalidArgument("project_id, stack_name and git_url are required")
	}
	if err := gitssrf.ValidateGitURL(gitURL); err != nil {
		return nil, invalidArgument(err.Error())
	}
	gitRef := strings.TrimSpace(request.GetGitRef())
	if gitRef == "" {
		gitRef = "main"
	}
	composePath := strings.TrimSpace(request.GetComposePath())
	if composePath == "" {
		composePath = "docker-compose.yml"
	}
	services := composeServicesFromProto(request.GetServices())
	enabled := make([]composeparse.Service, 0, len(services))
	for _, svc := range services {
		if svc.Enabled {
			enabled = append(enabled, svc)
		}
	}
	if len(enabled) == 0 {
		return nil, invalidArgument("enable at least one compose service")
	}

	stackID := db.NewStackID()
	created := make([]*paasv1.Workload, 0, len(enabled))
	actor := db.AuditActor{UserID: identity.UserID, Email: identity.Email}

	// Dependencies first so sibling discovery / API URLs resolve in a stable order.
	enabled = orderComposeServices(enabled)

	// Pre-allocate host ports so browser-facing env (REACT_APP_*) can point at
	// the published backend URL, not docker-network aliases (useless in the browser).
	hostPorts := map[string]int32{}
	for _, svc := range enabled {
		if !svc.ExposePublic {
			hostPorts[svc.Name] = 0
			continue
		}
		hostPort := svc.HostPort
		if hostPort < 1024 || hostPort >= 60000 {
			allocated, err := db.AllocateHostPort(ctx, service.pool, identity.OrganizationID)
			if err != nil {
				return nil, internalError("allocate host port", err)
			}
			hostPort = allocated
		}
		hostPorts[svc.Name] = hostPort
	}

	discovery := map[string]string{}
	for _, svc := range enabled {
		port := svc.ContainerPort
		if port <= 0 {
			port = 80
		}
		discovery[fmt.Sprintf("NORIX_SERVICE_%s_HOST", sanitizeEnv(svc.Name))] = svc.Name
		discovery[fmt.Sprintf("NORIX_SERVICE_%s_PORT", sanitizeEnv(svc.Name))] = fmt.Sprintf("%d", port)
		discovery[fmt.Sprintf("NORIX_SERVICE_%s_URL", sanitizeEnv(svc.Name))] = fmt.Sprintf("http://%s:%d", svc.Name, port)
		if hp := hostPorts[svc.Name]; hp > 0 {
			discovery[fmt.Sprintf("NORIX_SERVICE_%s_PUBLIC_URL", sanitizeEnv(svc.Name))] = fmt.Sprintf("http://127.0.0.1:%d", hp)
		}
	}

	for _, svc := range enabled {
		hostPort := hostPorts[svc.Name]
		containerPort := svc.ContainerPort
		if containerPort <= 0 {
			containerPort = 80
		}
		env := map[string]string{}
		for k, v := range svc.Environment {
			env[k] = v
		}
		for k, v := range discovery {
			if _, exists := env[k]; !exists {
				env[k] = v
			}
		}
		env["NORIX_STACK_ID"] = stackID
		env["NORIX_COMPOSE_SERVICE"] = svc.Name
		// CRA / Vite / Next bind localhost by default — unreachable via docker -p.
		if _, ok := env["HOST"]; !ok {
			env["HOST"] = "0.0.0.0"
		}
		if _, ok := env["DANGEROUSLY_DISABLE_HOST_CHECK"]; !ok {
			env["DANGEROUSLY_DISABLE_HOST_CHECK"] = "true"
		}
		rewriteBrowserAPIURLs(env, hostPorts, enabled)

		dockerfilePath := "Dockerfile"
		if svc.HasBuild {
			df := svc.Dockerfile
			if df == "" {
				df = "Dockerfile"
			}
			ctxDir := strings.TrimPrefix(strings.TrimSpace(svc.BuildContext), "./")
			if ctxDir == "" || ctxDir == "." {
				dockerfilePath = df
			} else {
				dockerfilePath = path.Join(ctxDir, df)
			}
		}

		name := fmt.Sprintf("%s-%s", stackName, svc.Name)
		wl := db.Workload{
			ProjectID:      projectID,
			Name:           name,
			Runtime:        "docker-compose",
			Environment:    env,
			ContainerPort:  containerPort,
			SourceKind:     "git",
			GitURL:         gitURL,
			GitRef:         gitRef,
			DockerfilePath: dockerfilePath,
			ImageOrSource:  "pending-build",
			ReplicaCount:   1,
		}
		if svc.HasImage && !svc.HasBuild {
			wl.SourceKind = "image"
			wl.ImageOrSource = svc.Image
			wl.GitURL = ""
			wl.DockerfilePath = ""
		}
		createdWL, err := db.CreateWorkload(ctx, service.pool, identity.OrganizationID, wl, actor)
		if err != nil {
			return nil, invalidArgument(fmt.Sprintf("create service %s: %v", svc.Name, err))
		}
		if err := db.AttachStackMetadata(ctx, service.pool, identity.OrganizationID, createdWL.ID, stackID, svc.Name, svc); err != nil {
			return nil, internalError("attach stack metadata", err)
		}
		createdWL.StackID = stackID
		createdWL.ComposeService = svc.Name

		if wl.SourceKind == "git" {
			_, err := db.CreateBuild(
				ctx, service.pool, identity.OrganizationID, createdWL.ID,
				gitURL, gitRef, dockerfilePath, containerPort, nil, actor,
			)
			if err != nil {
				return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Sprintf("build %s: %v", svc.Name, err))
			}
		} else {
			_, _, err := db.DeployWorkloadReplicas(ctx, service.pool, identity.OrganizationID, createdWL.ID, containerPort, actor)
			if err != nil {
				return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Sprintf("deploy %s: %v", svc.Name, err))
			}
		}
		if hostPort > 0 {
			_ = db.PreferHostPort(ctx, service.pool, identity.OrganizationID, createdWL.ID, hostPort)
		}

		fresh, _ := db.GetWorkload(ctx, service.pool, identity.OrganizationID, createdWL.ID)
		if fresh.ID != "" {
			createdWL = fresh
		}
		created = append(created, workloadToProto(createdWL))
	}

	return &paasv1.DeployComposeStackResponse{
		StackId:    stackID,
		Workloads:  created,
	}, nil
}

func composeServicesToProto(services []composeparse.Service) []*paasv1.ComposeServiceSpec {
	out := make([]*paasv1.ComposeServiceSpec, 0, len(services))
	for _, svc := range services {
		ports := make([]*paasv1.ComposePortMap, 0, len(svc.PublishedPorts))
		for _, p := range svc.PublishedPorts {
			ports = append(ports, &paasv1.ComposePortMap{Host: p.Host, Container: p.Container, Protocol: p.Protocol})
		}
		out = append(out, &paasv1.ComposeServiceSpec{
			Name: svc.Name, Image: svc.Image, BuildContext: svc.BuildContext, Dockerfile: svc.Dockerfile,
			ContainerPorts: svc.ContainerPorts, PublishedPorts: ports, DependsOn: svc.DependsOn,
			Environment: svc.Environment, Enabled: svc.Enabled, ExposePublic: svc.ExposePublic,
			HostPort: svc.HostPort, ContainerPort: svc.ContainerPort, HasBuild: svc.HasBuild, HasImage: svc.HasImage,
		})
	}
	return out
}

func composeServicesFromProto(services []*paasv1.ComposeServiceSpec) []composeparse.Service {
	out := make([]composeparse.Service, 0, len(services))
	for _, svc := range services {
		if svc == nil {
			continue
		}
		ports := make([]composeparse.PortMap, 0, len(svc.PublishedPorts))
		for _, p := range svc.PublishedPorts {
			ports = append(ports, composeparse.PortMap{Host: p.Host, Container: p.Container, Protocol: p.Protocol})
		}
		out = append(out, composeparse.Service{
			Name: svc.Name, Image: svc.Image, BuildContext: svc.BuildContext, Dockerfile: svc.Dockerfile,
			ContainerPorts: svc.ContainerPorts, PublishedPorts: ports, DependsOn: svc.DependsOn,
			Environment: svc.Environment, Enabled: svc.Enabled, ExposePublic: svc.ExposePublic,
			HostPort: svc.HostPort, ContainerPort: svc.ContainerPort, HasBuild: svc.HasBuild, HasImage: svc.HasImage,
		})
	}
	return out
}

func sanitizeEnv(name string) string {
	var b strings.Builder
	for _, ch := range strings.ToUpper(name) {
		if (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') {
			b.WriteRune(ch)
		} else {
			b.WriteByte('_')
		}
	}
	return b.String()
}

func orderComposeServices(services []composeparse.Service) []composeparse.Service {
	if len(services) < 2 {
		return services
	}
	byName := map[string]composeparse.Service{}
	for _, svc := range services {
		byName[svc.Name] = svc
	}
	visited := map[string]bool{}
	var out []composeparse.Service
	var visit func(string)
	visit = func(name string) {
		if visited[name] {
			return
		}
		svc, ok := byName[name]
		if !ok {
			return
		}
		visited[name] = true
		for _, dep := range svc.DependsOn {
			visit(dep)
		}
		out = append(out, svc)
	}
	for _, svc := range services {
		visit(svc.Name)
	}
	return out
}

// rewriteBrowserAPIURLs replaces localhost/compose-published API URLs with the
// Norix-allocated host port so CRA/Vite apps in the browser reach the backend.
func rewriteBrowserAPIURLs(env map[string]string, hostPorts map[string]int32, services []composeparse.Service) {
	backendPort := int32(0)
	for _, svc := range services {
		lower := strings.ToLower(svc.Name)
		if lower == "backend" || lower == "api" || lower == "server" {
			if hp := hostPorts[svc.Name]; hp > 0 {
				backendPort = hp
				break
			}
		}
	}
	if backendPort == 0 {
		for _, svc := range services {
			if hp := hostPorts[svc.Name]; hp > 0 && strings.ToLower(svc.Name) != "frontend" && strings.ToLower(svc.Name) != "web" {
				backendPort = hp
				break
			}
		}
	}
	if backendPort == 0 {
		return
	}
	publicAPI := fmt.Sprintf("http://127.0.0.1:%d/api", backendPort)
	for _, key := range []string{"REACT_APP_API_URL", "VITE_API_URL", "NEXT_PUBLIC_API_URL", "API_URL"} {
		if val, ok := env[key]; ok {
			if strings.Contains(val, "localhost") || strings.Contains(val, "127.0.0.1") || strings.Contains(val, ":3001") {
				env[key] = publicAPI
			}
		}
	}
	if _, ok := env["REACT_APP_API_URL"]; !ok {
		// Common for CRA samples that default to localhost:3001 in source.
		env["REACT_APP_API_URL"] = publicAPI
	}
}
