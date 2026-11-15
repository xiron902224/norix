package api

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"connectrpc.com/connect/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/xiron902224/norix/control-plane/internal/db"
	paasv1 "github.com/xiron902224/norix/control-plane/internal/gen/v1"
)

type SandboxService struct {
	pool *pgxpool.Pool
}

func NewSandboxService(pool *pgxpool.Pool) SandboxService {
	return SandboxService{pool: pool}
}

func sandboxToProto(item db.Sandbox) *paasv1.Sandbox {
	return &paasv1.Sandbox{
		Id:         item.ID,
		ProjectId:  item.ProjectID,
		Name:       item.Name,
		Template:   item.Template,
		Status:     item.Status,
		TtlSeconds: item.TTLSeconds,
		Cpu:        item.CPU,
		MemoryMb:   item.MemoryMB,
		EgressMode: item.EgressMode,
		ExpiresAt:  item.ExpiresAt.Unix(),
		PoolReady:  item.PoolReady,
	}
}

func (service SandboxService) ListSandboxes(ctx context.Context, request *paasv1.ListSandboxesRequest) (*paasv1.ListSandboxesResponse, error) {
	identity, ok := SessionFromContext(ctx)
	if err := requireSession(identity, ok); err != nil {
		return nil, err
	}
	items, err := db.ListSandboxes(ctx, service.pool, identity.OrganizationID, request.GetProjectId())
	if err != nil {
		return nil, internalError("list sandboxes", err)
	}
	out := make([]*paasv1.Sandbox, 0, len(items))
	for _, item := range items {
		out = append(out, sandboxToProto(item))
	}
	return &paasv1.ListSandboxesResponse{Sandboxes: out}, nil
}

func (service SandboxService) CreateSandbox(ctx context.Context, request *paasv1.CreateSandboxRequest) (*paasv1.CreateSandboxResponse, error) {
	identity, ok := SessionFromContext(ctx)
	if err := requireSession(identity, ok); err != nil {
		return nil, err
	}
	if err := requireMutator(identity); err != nil {
		return nil, err
	}
	if err := enforceProjectQuota(ctx, service.pool, identity.OrganizationID, request.GetProjectId(), db.QuotaDelta{
		AdditionalCPU:       request.GetCpu(),
		AdditionalMemoryMB:  int64(request.GetMemoryMb()),
		AdditionalSandboxes: 1,
	}); err != nil {
		return nil, err
	}
	item, err := db.CreateSandbox(ctx, service.pool, identity.OrganizationID, db.Sandbox{
		ProjectID:  request.GetProjectId(),
		Name:       request.GetName(),
		Template:   request.GetTemplate(),
		TTLSeconds: request.GetTtlSeconds(),
		CPU:        request.GetCpu(),
		MemoryMB:   request.GetMemoryMb(),
		EgressMode: request.GetEgressMode(),
	})
	if err != nil {
		if strings.Contains(err.Error(), "required") {
			return nil, invalidArgument(err.Error())
		}
		return nil, internalError("create sandbox", err)
	}
	if teamID, teamErr := db.TeamIDForProject(ctx, service.pool, identity.OrganizationID, request.GetProjectId()); teamErr == nil {
		_ = db.RecordUsage(ctx, service.pool, identity.OrganizationID, teamID, "sandbox", 1, "count", map[string]string{"sandbox_id": item.ID})
	}
	return &paasv1.CreateSandboxResponse{Sandbox: sandboxToProto(item)}, nil
}

func (service SandboxService) DestroySandbox(ctx context.Context, request *paasv1.DestroySandboxRequest) (*paasv1.DestroySandboxResponse, error) {
	identity, ok := SessionFromContext(ctx)
	if err := requireSession(identity, ok); err != nil {
		return nil, err
	}
	if err := requireMutator(identity); err != nil {
		return nil, err
	}
	if err := db.DestroySandbox(ctx, service.pool, identity.OrganizationID, request.GetId()); err != nil {
		return nil, internalError("destroy sandbox", err)
	}
	return &paasv1.DestroySandboxResponse{}, nil
}

func (service SandboxService) ExecSandbox(ctx context.Context, request *paasv1.ExecSandboxRequest) (*paasv1.ExecSandboxResponse, error) {
	identity, ok := SessionFromContext(ctx)
	if err := requireSession(identity, ok); err != nil {
		return nil, err
	}
	sandboxID := strings.TrimSpace(request.GetId())
	if sandboxID == "" {
		return nil, invalidArgument("id is required")
	}
	item, err := db.GetSandbox(ctx, service.pool, identity.OrganizationID, sandboxID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) || strings.Contains(err.Error(), "no rows") {
			return nil, connect.NewError(connect.CodeNotFound, "sandbox not found")
		}
		return nil, internalError("get sandbox", err)
	}
	cmd := strings.Join(request.GetCommand(), " ")
	if cmd == "" {
		cmd = "true"
	}
	switch item.Status {
	case "destroyed", "expired":
		return &paasv1.ExecSandboxResponse{
			Output:   fmt.Sprintf("sandbox %s is %s\n", item.Name, item.Status),
			ExitCode: 1,
		}, nil
	case "pending":
		return &paasv1.ExecSandboxResponse{
			Output:   fmt.Sprintf("sandbox %s (%s) is not ready yet\n", item.Name, item.Status),
			ExitCode: 1,
		}, nil
	}
	argv := request.GetCommand()
	if len(argv) == 0 {
		argv = []string{"true"}
	}
	output, exitCode, execErr := execSandboxCommand(ctx, item, argv)
	if execErr != nil {
		return &paasv1.ExecSandboxResponse{
			Output:   fmt.Sprintf("[%s] exec error: %v\n%s", item.Name, execErr, output),
			ExitCode: 1,
		}, nil
	}
	header := fmt.Sprintf("[%s] template=%s status=%s pool_ready=%v\n$ %s\n",
		item.Name, item.Template, item.Status, item.PoolReady, strings.Join(argv, " "))
	return &paasv1.ExecSandboxResponse{Output: header + output, ExitCode: exitCode}, nil
}

// execSandboxCommand runs the command inside a short-lived container labeled for the sandbox.
// Prefers an existing norix-sandbox-<id> container; otherwise starts template image ephemeral.
func execSandboxCommand(ctx context.Context, item db.Sandbox, argv []string) (string, int32, error) {
	engine := "docker"
	if _, err := exec.LookPath(engine); err != nil {
		engine = "nerdctl"
		if _, err := exec.LookPath(engine); err != nil {
			return "", 1, fmt.Errorf("docker/nerdctl not available")
		}
	}
	name := fmt.Sprintf("norix-sandbox-%s", strings.ReplaceAll(item.ID, "-", "")[:12])
	runCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	// Try docker exec into running warm-pool container.
	inspect := exec.CommandContext(runCtx, engine, "inspect", "-f", "{{.State.Running}}", name)
	if out, err := inspect.CombinedOutput(); err == nil && strings.TrimSpace(string(out)) == "true" {
		args := append([]string{"exec", name}, argv...)
		cmd := exec.CommandContext(runCtx, engine, args...)
		var buf bytes.Buffer
		cmd.Stdout = &buf
		cmd.Stderr = &buf
		err := cmd.Run()
		code := int32(0)
		if err != nil {
			if ee, ok := err.(*exec.ExitError); ok {
				code = int32(ee.ExitCode())
			} else {
				return buf.String(), 1, err
			}
		}
		return buf.String(), code, nil
	}

	image := sandboxTemplateImage(item.Template)
	mem := fmt.Sprintf("%dm", item.MemoryMB)
	if item.MemoryMB <= 0 {
		mem = "512m"
	}
	args := []string{
		"run", "--rm",
		"--name", name + "-exec",
		"--network", "none",
		"--memory", mem,
		"--label", "norix.sandbox_id=" + item.ID,
		"--label", "norix.sandbox=1",
		image,
	}
	if item.EgressMode != "allow" && item.EgressMode != "open" {
		// already --network none (deny-by-default)
	} else {
		// recreate with default bridge when egress allowed
		args = []string{
			"run", "--rm",
			"--name", name + "-exec",
			"--memory", mem,
			"--label", "norix.sandbox_id=" + item.ID,
			"--label", "norix.sandbox=1",
			image,
		}
	}
	args = append(args, argv...)
	cmd := exec.CommandContext(runCtx, engine, args...)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	code := int32(0)
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			code = int32(ee.ExitCode())
		} else {
			return buf.String(), 1, err
		}
	}
	return buf.String(), code, nil
}

func sandboxTemplateImage(template string) string {
	switch strings.ToLower(strings.TrimSpace(template)) {
	case "python", "python3":
		return "python:3.12-alpine"
	case "node", "nodejs":
		return "node:20-alpine"
	case "go", "golang":
		return "golang:1.22-alpine"
	default:
		return "alpine:3.20"
	}
}
