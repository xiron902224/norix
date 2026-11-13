package api

import (
	"context"
	"errors"
	"fmt"
	"strings"

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
	output := fmt.Sprintf(
		"[%s] template=%s status=%s pool_ready=%v\n$ %s\n(local exec stub — command accepted)\n",
		item.Name, item.Template, item.Status, item.PoolReady, cmd,
	)
	return &paasv1.ExecSandboxResponse{Output: output, ExitCode: 0}, nil
}
