package api

import (
	"context"
	"net/http"
	"sync"
	"time"

	"connectrpc.com/connect/v2"
)

const publicRateLimitWindow = time.Minute

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
			if !isPublicProtectedProcedure(spec.Procedure) {
				return next(ctx, spec, stream)
			}
			callInfo, ok := connect.CallInfoForServerContext(ctx)
			if !ok {
				return next(ctx, spec, stream)
			}
			transport, ok := callInfo.TransportInfo.(interface{ RemoteAddr() string })
			key := "unknown"
			if ok && transport.RemoteAddr() != "" {
				key = transport.RemoteAddr()
			}
			if !limiter.allow(key) {
				return connect.NewError(connect.CodeResourceExhausted, "too many login or bootstrap attempts")
			}
			return next(ctx, spec, stream)
		}
	}
}

func (limiter *RateLimiter) allow(key string) bool {
	limiter.mu.Lock()
	defer limiter.mu.Unlock()
	now := time.Now()
	current := limiter.buckets[key]
	if current == nil || now.After(current.refillAt) {
		limiter.buckets[key] = &bucket{count: 1, refillAt: now.Add(publicRateLimitWindow)}
		return true
	}
	if current.count >= limiter.limit {
		return false
	}
	current.count++
	return true
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
