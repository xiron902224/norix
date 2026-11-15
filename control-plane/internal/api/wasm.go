package api

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/xiron902224/norix/control-plane/internal/db"
	paasv1 "github.com/xiron902224/norix/control-plane/internal/gen/v1"
)

type WasmService struct {
	pool *pgxpool.Pool
}

func NewWasmService(pool *pgxpool.Pool) WasmService {
	return WasmService{pool: pool}
}

func wasmToProto(item db.WasmFunction) *paasv1.WasmFunction {
	return &paasv1.WasmFunction{
		Id:            item.ID,
		ProjectId:     item.ProjectID,
		Name:          item.Name,
		ImageRef:      item.ImageRef,
		Status:        item.Status,
		FuelLimit:     item.FuelLimit,
		MemoryLimitMb: item.MemoryLimitMB,
	}
}

func (service WasmService) ListWasmFunctions(ctx context.Context, request *paasv1.ListWasmFunctionsRequest) (*paasv1.ListWasmFunctionsResponse, error) {
	identity, ok := SessionFromContext(ctx)
	if err := requireSession(identity, ok); err != nil {
		return nil, err
	}
	items, err := db.ListWasmFunctions(ctx, service.pool, identity.OrganizationID, request.GetProjectId())
	if err != nil {
		return nil, internalError("list wasm functions", err)
	}
	out := make([]*paasv1.WasmFunction, 0, len(items))
	for _, item := range items {
		out = append(out, wasmToProto(item))
	}
	return &paasv1.ListWasmFunctionsResponse{Functions: out}, nil
}

func (service WasmService) CreateWasmFunction(ctx context.Context, request *paasv1.CreateWasmFunctionRequest) (*paasv1.CreateWasmFunctionResponse, error) {
	identity, ok := SessionFromContext(ctx)
	if err := requireSession(identity, ok); err != nil {
		return nil, err
	}
	if err := requireMutator(identity); err != nil {
		return nil, err
	}
	item, err := db.CreateWasmFunction(ctx, service.pool, identity.OrganizationID, db.WasmFunction{
		ProjectID:     request.GetProjectId(),
		Name:          request.GetName(),
		ImageRef:      request.GetImageRef(),
		FuelLimit:     request.GetFuelLimit(),
		MemoryLimitMB: request.GetMemoryLimitMb(),
	})
	if err != nil {
		if strings.Contains(err.Error(), "required") {
			return nil, invalidArgument(err.Error())
		}
		return nil, internalError("create wasm function", err)
	}
	return &paasv1.CreateWasmFunctionResponse{Function: wasmToProto(item)}, nil
}
