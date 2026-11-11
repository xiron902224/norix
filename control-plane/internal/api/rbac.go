package api

import (
	"connectrpc.com/connect/v2"
	"github.com/xiron902224/norix/control-plane/internal/auth"
)

func requireSession(identity auth.SessionIdentity, ok bool) error {
	if !ok {
		return connect.NewError(connect.CodeUnauthenticated, "authenticated session required")
	}
	return nil
}

func requireMutator(identity auth.SessionIdentity) error {
	switch identity.Role {
	case "owner", "admin":
		return nil
	default:
		return connect.NewError(connect.CodePermissionDenied, "owner or admin role required")
	}
}
