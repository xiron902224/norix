package api

import (
	"context"
	"strings"

	"connectrpc.com/connect/v2"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/xiron902224/norix/control-plane/internal/db"
)

func failedPrecondition(message string) error {
	return connect.NewError(connect.CodeFailedPrecondition, message)
}

func enforceProjectQuota(ctx context.Context, pool *pgxpool.Pool, organizationID, projectID string, delta db.QuotaDelta) error {
	if projectID == "" {
		return invalidArgument("project id is required")
	}
	if err := db.CheckProjectQuota(ctx, pool, organizationID, projectID, delta); err != nil {
		if strings.Contains(err.Error(), "exceeded") {
			return failedPrecondition(err.Error())
		}
		return internalError("quota check", err)
	}
	return nil
}

// WithQuotaInterceptor reserves quota-sensitive RPCs for future request decoding; handlers call enforceProjectQuota.
func WithQuotaInterceptor(pool *pgxpool.Pool) connect.ServerInterceptor {
	_ = pool
	return func(next connect.ServerFunc) connect.ServerFunc {
		return func(ctx context.Context, spec connect.Spec, stream connect.ServerStream) error {
			return next(ctx, spec, stream)
		}
	}
}
