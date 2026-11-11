package api

import (
	"context"
	"strings"

	"connectrpc.com/connect/v2"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/xiron902224/norix/control-plane/internal/auth"
	"github.com/xiron902224/norix/control-plane/internal/db"
)

func WithAuthentication(pool *pgxpool.Pool) connect.ServerInterceptor {
	return func(next connect.ServerFunc) connect.ServerFunc {
		return func(ctx context.Context, spec connect.Spec, stream connect.ServerStream) error {
			if isPublicProcedure(spec.Procedure) {
				return next(ctx, spec, stream)
			}
			callInfo, ok := connect.CallInfoForServerContext(ctx)
			if !ok {
				return connect.NewError(connect.CodeUnauthenticated, "authentication context unavailable")
			}
			authorization := callInfo.RequestHeader().Get("Authorization")
			if !strings.HasPrefix(authorization, "Bearer ") {
				return connect.NewError(connect.CodeUnauthenticated, "Bearer token required")
			}
			token := strings.TrimSpace(strings.TrimPrefix(authorization, "Bearer "))
			hash := auth.HashSessionToken(token)

			if isAgentProcedure(spec.Procedure) {
				node, err := db.FindNodeByTokenHash(ctx, pool, hash)
				if err != nil {
					return connect.NewError(connect.CodeUnauthenticated, "node credential is invalid")
				}
				return next(contextWithNode(ctx, node), spec, stream)
			}

			identity, err := findActiveSession(ctx, pool, hash)
			if err != nil {
				return connect.NewError(connect.CodeUnauthenticated, "session is invalid or expired")
			}
			return next(contextWithSession(ctx, identity), spec, stream)
		}
	}
}

func isPublicProcedure(procedure string) bool {
	switch procedure {
	case "/v1.AuthService/Login",
		"/v1.AuthService/GetBootstrapStatus",
		"/v1.AuthService/CreateInitialAdmin",
		"/v1.AgentService/EnrollNode":
		return true
	default:
		return false
	}
}

func isAgentProcedure(procedure string) bool {
	switch procedure {
	case "/v1.AgentService/Heartbeat",
		"/v1.AgentService/ReportObserved",
		"/v1.AgentService/ReportBuild",
		"/v1.AgentService/ConnectStream":
		return true
	default:
		return false
	}
}
