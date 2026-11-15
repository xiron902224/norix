package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"
	"sync"
	"time"

	"connectrpc.com/connect/v2"
)

const publicRateLimitWindow = time.Minute
const authenticatedMutateLimit = 120

type bucket struct {
	count    int
	refillAt time.Time
}

type RateLimiter struct {
	mu      sync.Mutex
	limit   int
	buckets map[string]*bucket
}

func NewRateLimiter(limit int) *RateLimiter {
	if limit < 1 {
		limit = 10
	}
	return &RateLimiter{limit: limit, buckets: make(map[string]*bucket)}
}

func (limiter *RateLimiter) Interceptor() connect.ServerInterceptor {
	return func(next connect.ServerFunc) connect.ServerFunc {
		return func(ctx context.Context, spec connect.Spec, stream connect.ServerStream) error {
			if isPublicProtectedProcedure(spec.Procedure) {
				callInfo, ok := connect.CallInfoForServerContext(ctx)
				if !ok {
					return next(ctx, spec, stream)
				}
				transport, ok := callInfo.TransportInfo.(interface{ RemoteAddr() string })
				key := "unknown"
				if ok && transport.RemoteAddr() != "" {
					key = transport.RemoteAddr()
				}
				if !limiter.allow(key, limiter.limit) {
					return connect.NewError(connect.CodeResourceExhausted, "too many login or bootstrap attempts")
				}
			}
			if isAuthenticatedMutateProcedure(spec.Procedure) {
				callInfo, ok := connect.CallInfoForServerContext(ctx)
				if ok {
					authHeader := callInfo.RequestHeader().Get("Authorization")
					if strings.HasPrefix(authHeader, "Bearer ") {
						token := strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer "))
						sum := sha256.Sum256([]byte(token))
						key := "session:" + hex.EncodeToString(sum[:8])
						if !limiter.allow(key, authenticatedMutateLimit) {
							return connect.NewError(connect.CodeResourceExhausted, "API token rate limit exceeded; retry shortly")
						}
					}
				}
			}
			return next(ctx, spec, stream)
		}
	}
}

func (limiter *RateLimiter) allow(key string, limit int) bool {
	if limit < 1 {
		limit = limiter.limit
	}
	limiter.mu.Lock()
	defer limiter.mu.Unlock()
	now := time.Now()
	current := limiter.buckets[key]
	if current == nil || now.After(current.refillAt) {
		limiter.buckets[key] = &bucket{count: 1, refillAt: now.Add(publicRateLimitWindow)}
		return true
	}
	if current.count >= limit {
		return false
	}
	current.count++
	return true
}

func isAuthenticatedMutateProcedure(procedure string) bool {
	switch procedure {
	case "/v1.ProjectService/CreateProject",
		"/v1.ProjectService/DeleteProject",
		"/v1.WorkloadService/CreateWorkload",
		"/v1.WorkloadService/UpdateWorkload",
		"/v1.WorkloadService/DeleteWorkload",
		"/v1.WorkloadService/DeployWorkload",
		"/v1.WorkloadService/CreateBuild",
		"/v1.WorkloadService/AssignDomain",
		"/v1.WorkloadService/UnassignDomain",
		"/v1.DatabaseService/CreateManagedDatabase",
		"/v1.DatabaseService/BackupManagedDatabase",
		"/v1.DatabaseService/RestoreManagedDatabase",
		"/v1.SandboxService/CreateSandbox",
		"/v1.SandboxService/DestroySandbox",
		"/v1.SandboxService/ExecSandbox",
		"/v1.PreviewService/UpsertPreview",
		"/v1.WasmService/CreateWasmFunction",
		"/v1.TeamService/CreateTeam",
		"/v1.TeamService/AddTeamMember",
		"/v1.TeamService/SetTeamQuota",
		"/v1.TeamService/UpsertSSOProvider":
		return true
	default:
		return false
	}
}

func isPublicProtectedProcedure(procedure string) bool {
	switch procedure {
	case "/v1.AuthService/Login",
		"/v1.AuthService/CreateInitialAdmin",
		"/v1.AgentService/EnrollNode":
		return true
	default:
		return false
	}
}

func ClientIPHeader(request *http.Request) string { return request.RemoteAddr }
