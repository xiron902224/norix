package api

import (
	"context"
	"errors"
	"testing"

	"connectrpc.com/connect/v2"
	paasv1 "github.com/xiron902224/norix/control-plane/internal/gen/v1"
)

func TestAuthServiceLogin(t *testing.T) {
	response, err := (AuthService{}).Login(context.Background(), &paasv1.LoginRequest{
		Email:    "developer@norix.local",
		Password: "development-only",
	})
	if err == nil {
		t.Fatal("Login() returned nil error while persistent authentication is disabled")
	}
	if response != nil {
		t.Fatal("Login() returned a response while persistent authentication is disabled")
	}
}

func TestAuthServiceLoginRequiresCredentials(t *testing.T) {
	_, err := (AuthService{}).Login(context.Background(), &paasv1.LoginRequest{})
	if err == nil {
		t.Fatal("Login() returned nil error, want invalid argument")
	}

	var connectError *connect.Error
	if !errors.As(err, &connectError) {
		t.Fatalf("Login() error = %T, want *connect.Error", err)
	}
	if connectError.Code() != connect.CodeInvalidArgument {
		t.Errorf("Login() code = %v, want %v", connectError.Code(), connect.CodeInvalidArgument)
	}
}
