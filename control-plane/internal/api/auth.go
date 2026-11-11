package api

import (
	"context"
	"strings"
	"time"

	"connectrpc.com/connect/v2"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/xiron902224/norix/control-plane/internal/auth"
	"github.com/xiron902224/norix/control-plane/internal/db"
	paasv1 "github.com/xiron902224/norix/control-plane/internal/gen/v1"
)

// AuthService provides authentication and first-admin bootstrap behavior.
type AuthService struct {
	appPool         *pgxpool.Pool
	bootstrapPool   *pgxpool.Pool
	sessionDuration int
}

func NewAuthService(appPool, bootstrapPool *pgxpool.Pool) AuthService {
	return AuthService{appPool: appPool, bootstrapPool: bootstrapPool, sessionDuration: 12}
}

func (service AuthService) BootstrapPool() *pgxpool.Pool {
	return service.bootstrapPool
}

func (service AuthService) Login(ctx context.Context, request *paasv1.LoginRequest) (*paasv1.LoginResponse, error) {
	if request.GetEmail() == "" || request.GetPassword() == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, "email and password are required")
	}
	if service.bootstrapPool == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, "database is not configured")
	}

	user, err := db.FindUserByEmail(ctx, service.bootstrapPool, request.GetEmail())
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, "invalid email or password")
	}
	valid, err := auth.VerifyPassword(request.GetPassword(), user.PasswordHash)
	if err != nil || !valid {
		return nil, connect.NewError(connect.CodeUnauthenticated, "invalid email or password")
	}

	token, tokenHash, err := auth.NewSessionToken()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, "could not create session")
	}
	expiresAt := time.Now().UTC().Add(12 * time.Hour)
	if err := db.CreateSession(ctx, service.bootstrapPool, user.OrganizationID, user.ID, tokenHash, expiresAt); err != nil {
		return nil, connect.NewError(connect.CodeInternal, "could not create session")
	}

	return &paasv1.LoginResponse{Token: token, User: roleUser(user)}, nil
}

func roleUser(user db.AuthUser) *paasv1.User {
	role := paasv1.Role_ROLE_MEMBER
	switch user.Role {
	case "owner":
		role = paasv1.Role_ROLE_OWNER
	case "admin":
		role = paasv1.Role_ROLE_ADMIN
	}
	return &paasv1.User{Id: user.ID, Email: user.Email, Name: user.Name, Role: role}
}

func (service AuthService) Logout(ctx context.Context, request *paasv1.LogoutRequest) (*paasv1.LogoutResponse, error) {
	if request.GetToken() == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, "session token is required")
	}
	if service.bootstrapPool == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, "database is not configured")
	}
	if err := db.RevokeSession(ctx, service.bootstrapPool, auth.HashSessionToken(request.GetToken())); err != nil {
		return nil, connect.NewError(connect.CodeInternal, "could not revoke session")
	}
	return &paasv1.LogoutResponse{}, nil
}

func (service AuthService) RenewSession(ctx context.Context, request *paasv1.RenewSessionRequest) (*paasv1.RenewSessionResponse, error) {
	if request.GetToken() == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, "session token is required")
	}
	if service.bootstrapPool == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, "database is not configured")
	}
	expiresAt, err := db.RenewSession(ctx, service.bootstrapPool, auth.HashSessionToken(request.GetToken()), time.Now().UTC().Add(12*time.Hour))
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, "session is invalid or expired")
	}
	return &paasv1.RenewSessionResponse{Token: request.GetToken(), ExpiresAtUnix: expiresAt.Unix()}, nil
}

func (service AuthService) GetBootstrapStatus(ctx context.Context, _ *paasv1.GetBootstrapStatusRequest) (*paasv1.GetBootstrapStatusResponse, error) {
	if service.bootstrapPool == nil {
		return &paasv1.GetBootstrapStatusResponse{Configured: false}, nil
	}
	exists, err := db.OwnerExists(ctx, service.bootstrapPool)
	if err != nil {
		return nil, internalError("bootstrap status", err)
	}
	return &paasv1.GetBootstrapStatusResponse{Configured: exists}, nil
}

func (service AuthService) CreateInitialAdmin(ctx context.Context, request *paasv1.CreateInitialAdminRequest) (*paasv1.CreateInitialAdminResponse, error) {
	if service.bootstrapPool == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, "database is not configured")
	}

	exists, err := db.OwnerExists(ctx, service.bootstrapPool)
	if err != nil {
		return nil, internalError("bootstrap check", err)
	}
	if exists {
		return nil, connect.NewError(connect.CodeAlreadyExists, "the initial administrator has already been created")
	}

	if len(request.GetAdminPassword()) < 12 {
		return nil, connect.NewError(connect.CodeInvalidArgument, "administrator password must contain at least 12 characters")
	}
	passwordHash, err := auth.HashPassword(request.GetAdminPassword())
	if err != nil {
		return nil, invalidArgument("administrator password is invalid")
	}
	owner, err := db.CreateInitialOwner(ctx, service.bootstrapPool, request.GetOrganizationName(), request.GetAdminName(), request.GetAdminEmail(), passwordHash)
	if err != nil {
		if strings.Contains(err.Error(), "already been created") {
			return nil, connect.NewError(connect.CodeAlreadyExists, "the initial administrator has already been created")
		}
		return nil, internalError("create initial admin", err)
	}
	created, err := db.FindUserByEmail(ctx, service.bootstrapPool, owner.Email)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, "could not load initial administrator")
	}
	token, tokenHash, err := auth.NewSessionToken()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, "could not create session")
	}
	if err := db.CreateSession(ctx, service.bootstrapPool, created.OrganizationID, created.ID, tokenHash, time.Now().UTC().Add(12*time.Hour)); err != nil {
		return nil, connect.NewError(connect.CodeInternal, "could not create session")
	}

	return &paasv1.CreateInitialAdminResponse{Token: token, User: roleUser(created)}, nil
}
