package api

import (
	"errors"
	"testing"

	"connectrpc.com/connect/v2"
	"github.com/xiron902224/norix/control-plane/internal/auth"
)

func TestRequireMutator(t *testing.T) {
	if err := requireMutator(auth.SessionIdentity{Role: "owner"}); err != nil {
		t.Fatalf("owner should mutate: %v", err)
	}
	if err := requireMutator(auth.SessionIdentity{Role: "admin"}); err != nil {
		t.Fatalf("admin should mutate: %v", err)
	}
	err := requireMutator(auth.SessionIdentity{Role: "member"})
	if err == nil {
		t.Fatal("member should be denied")
	}
	var connectError *connect.Error
	if !errors.As(err, &connectError) || connectError.Code() != connect.CodePermissionDenied {
		t.Fatalf("want PermissionDenied, got %v", err)
	}
}
