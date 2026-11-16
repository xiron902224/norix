package api

import (
	"context"
	"errors"
	"fmt"
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

func TestPlacementFailedPreconditionMapsUnschedulable(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "unschedulable no feasible",
			err:  fmt.Errorf("unschedulable: no feasible node for workload constraints"),
			want: true,
		},
		{
			name: "no online agent",
			err:  fmt.Errorf("no online agent enrolled"),
			want: true,
		},
		{
			name: "no runtime",
			err:  fmt.Errorf("no runtime agent enrolled; create a join token and enroll a node first"),
			want: true,
		},
		{
			name: "no nodes enrolled",
			err:  fmt.Errorf("no nodes enrolled: ensure local failed"),
			want: true,
		},
		{
			name: "unrelated",
			err:  fmt.Errorf("workload not found"),
			want: false,
		},
		{
			name: "nil",
			err:  nil,
			want: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mapped := placementFailedPrecondition(tc.err)
			if tc.want {
				if mapped == nil {
					t.Fatal("expected FailedPrecondition mapping")
				}
				var connectError *connect.Error
				if !errors.As(mapped, &connectError) {
					t.Fatalf("error type = %T", mapped)
				}
				if connectError.Code() != connect.CodeFailedPrecondition {
					t.Fatalf("code = %v, want FailedPrecondition", connectError.Code())
				}
				if connectError.Message() != tc.err.Error() {
					t.Fatalf("message = %q, want %q", connectError.Message(), tc.err.Error())
				}
				return
			}
			if mapped != nil {
				t.Fatalf("expected nil mapping, got %v", mapped)
			}
		})
	}
}
