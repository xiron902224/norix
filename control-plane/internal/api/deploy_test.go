package api

import (
	"context"
	"errors"
	"testing"

	"connectrpc.com/connect/v2"
	"github.com/xiron902224/norix/control-plane/internal/auth"
	paasv1 "github.com/xiron902224/norix/control-plane/internal/gen/v1"
)

func TestDeployRequiresEnrolledAgent(t *testing.T) {
	ctx := contextWithSession(context.Background(), auth.SessionIdentity{
		UserID:         "user-1",
		OrganizationID: "org-1",
		Email:          "owner@example.com",
		Role:           "owner",
	})
	_, err := (WorkloadService{}).Deploy(ctx, &paasv1.DeployRequest{
		Workload: &paasv1.Workload{Id: "workload-1"},
	})
	if err == nil {
		t.Fatal("Deploy() returned nil error")
	}
	var connectError *connect.Error
	if !errors.As(err, &connectError) {
		t.Fatalf("error type = %T", err)
	}
	if connectError.Code() != connect.CodeFailedPrecondition {
		t.Fatalf("code = %v, want FailedPrecondition", connectError.Code())
	}
}
