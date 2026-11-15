package api

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/xiron902224/norix/control-plane/internal/db"
	paasv1 "github.com/xiron902224/norix/control-plane/internal/gen/v1"
)

type DatabaseService struct {
	pool *pgxpool.Pool
}

func NewDatabaseService(pool *pgxpool.Pool) DatabaseService {
	return DatabaseService{pool: pool}
}

func managedDBEngine(engine paasv1.ManagedDatabaseEngine) string {
	switch engine {
	case paasv1.ManagedDatabaseEngine_MANAGED_DATABASE_ENGINE_POSTGRES:
		return "postgres"
	case paasv1.ManagedDatabaseEngine_MANAGED_DATABASE_ENGINE_MYSQL:
		return "mysql"
	case paasv1.ManagedDatabaseEngine_MANAGED_DATABASE_ENGINE_REDIS:
		return "redis"
	case paasv1.ManagedDatabaseEngine_MANAGED_DATABASE_ENGINE_MONGODB:
		return "mongodb"
	default:
		return ""
	}
}

func managedDBToProto(item db.ManagedDatabase) *paasv1.ManagedDatabase {
	out := &paasv1.ManagedDatabase{
		Id:             item.ID,
		ProjectId:      item.ProjectID,
		Name:           item.Name,
		Status:         item.Status,
		NodeId:         item.NodeID,
		VolumePath:     item.VolumePath,
		BackupSchedule: item.BackupSchedule,
	}
	switch item.Engine {
	case "postgres":
		out.Engine = paasv1.ManagedDatabaseEngine_MANAGED_DATABASE_ENGINE_POSTGRES
	case "mysql":
		out.Engine = paasv1.ManagedDatabaseEngine_MANAGED_DATABASE_ENGINE_MYSQL
	case "redis":
		out.Engine = paasv1.ManagedDatabaseEngine_MANAGED_DATABASE_ENGINE_REDIS
	case "mongodb":
		out.Engine = paasv1.ManagedDatabaseEngine_MANAGED_DATABASE_ENGINE_MONGODB
	}
	if item.LastBackupAt != nil {
		out.LastBackupAt = item.LastBackupAt.Unix()
	}
	return out
}

func (service DatabaseService) ListManagedDatabases(ctx context.Context, request *paasv1.ListManagedDatabasesRequest) (*paasv1.ListManagedDatabasesResponse, error) {
	identity, ok := SessionFromContext(ctx)
	if err := requireSession(identity, ok); err != nil {
		return nil, err
	}
	items, err := db.ListManagedDatabases(ctx, service.pool, identity.OrganizationID, request.GetProjectId())
	if err != nil {
		return nil, internalError("list managed databases", err)
	}
	out := make([]*paasv1.ManagedDatabase, 0, len(items))
	for _, item := range items {
		out = append(out, managedDBToProto(item))
	}
	return &paasv1.ListManagedDatabasesResponse{Databases: out}, nil
}

func (service DatabaseService) CreateManagedDatabase(ctx context.Context, request *paasv1.CreateManagedDatabaseRequest) (*paasv1.CreateManagedDatabaseResponse, error) {
	identity, ok := SessionFromContext(ctx)
	if err := requireSession(identity, ok); err != nil {
		return nil, err
	}
	if err := requireMutator(identity); err != nil {
		return nil, err
	}
	engine := managedDBEngine(request.GetEngine())
	if engine == "" {
		return nil, invalidArgument("engine is required")
	}
	if err := enforceProjectQuota(ctx, service.pool, identity.OrganizationID, request.GetProjectId(), db.QuotaDelta{
		AdditionalDiskGB: 10,
	}); err != nil {
		return nil, err
	}
	item, err := db.CreateManagedDatabase(ctx, service.pool, identity.OrganizationID, request.GetProjectId(), engine, request.GetName(), request.GetBackupSchedule())
	if err != nil {
		if strings.Contains(err.Error(), "required") || strings.Contains(err.Error(), "unsupported") {
			return nil, invalidArgument(err.Error())
		}
		return nil, internalError("create managed database", err)
	}
	return &paasv1.CreateManagedDatabaseResponse{Database: managedDBToProto(item)}, nil
}

func (service DatabaseService) BackupManagedDatabase(ctx context.Context, request *paasv1.BackupManagedDatabaseRequest) (*paasv1.BackupManagedDatabaseResponse, error) {
	identity, ok := SessionFromContext(ctx)
	if err := requireSession(identity, ok); err != nil {
		return nil, err
	}
	if err := requireMutator(identity); err != nil {
		return nil, err
	}
	item, err := db.BackupManagedDatabase(ctx, service.pool, identity.OrganizationID, request.GetId())
	if err != nil {
		return nil, internalError("backup managed database", err)
	}
	return &paasv1.BackupManagedDatabaseResponse{Database: managedDBToProto(item)}, nil
}

func (service DatabaseService) RestoreManagedDatabase(ctx context.Context, request *paasv1.RestoreManagedDatabaseRequest) (*paasv1.RestoreManagedDatabaseResponse, error) {
	identity, ok := SessionFromContext(ctx)
	if err := requireSession(identity, ok); err != nil {
		return nil, err
	}
	if err := requireMutator(identity); err != nil {
		return nil, err
	}
	item, err := db.RestoreManagedDatabase(ctx, service.pool, identity.OrganizationID, request.GetId())
	if err != nil {
		return nil, internalError("restore managed database", err)
	}
	return &paasv1.RestoreManagedDatabaseResponse{Database: managedDBToProto(item)}, nil
}
