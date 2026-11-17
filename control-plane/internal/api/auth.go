package api

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"connectrpc.com/connect/v2"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/xiron902224/norix/control-plane/internal/auth"
	"github.com/xiron902224/norix/control-plane/internal/db"
	paasv1 "github.com/xiron902224/norix/control-plane/internal/gen/v1"
)

const totpRequiredMessage = "TOTP_REQUIRED: enter your authenticator code"

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
	if user.PasswordHash == db.SSOPasswordPlaceholder {
		return nil, connect.NewError(connect.CodeUnauthenticated, "this account uses SSO; sign in with your identity provider")
	}
	valid, err := auth.VerifyPassword(request.GetPassword(), user.PasswordHash)
	if err != nil || !valid {
		return nil, connect.NewError(connect.CodeUnauthenticated, "invalid email or password")
	}

	totpEnabled, err := db.UserTOTPEnabled(ctx, service.bootstrapPool, user.OrganizationID, user.ID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, "could not check totp status")
	}
	mandatory, err := db.OrganizationMandatory2FA(ctx, service.bootstrapPool, user.OrganizationID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, "could not check organization 2fa policy")
	}
	if totpEnabled || mandatory {
		code := strings.TrimSpace(request.GetTotpCode())
		if code == "" {
			return nil, connect.NewError(connect.CodeFailedPrecondition, totpRequiredMessage)
		}
		if !totpEnabled {
			return nil, connect.NewError(connect.CodeFailedPrecondition, "TOTP_ENROLLMENT_REQUIRED: enable authenticator before signing in")
		}
		ok, validateErr := db.ValidateUserTOTP(ctx, service.bootstrapPool, user.OrganizationID, user.ID, code)
		if validateErr != nil || !ok {
			return nil, connect.NewError(connect.CodeUnauthenticated, "invalid authenticator code")
		}
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

func (service AuthService) GetCurrentUser(ctx context.Context, _ *paasv1.GetCurrentUserRequest) (*paasv1.GetCurrentUserResponse, error) {
	identity, ok := SessionFromContext(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, "session is required")
	}
	role := paasv1.Role_ROLE_MEMBER
	switch identity.Role {
	case "owner":
		role = paasv1.Role_ROLE_OWNER
	case "admin":
		role = paasv1.Role_ROLE_ADMIN
	}
	return &paasv1.GetCurrentUserResponse{User: &paasv1.User{
		Id:    identity.UserID,
		Email: identity.Email,
		Name:  identity.Name,
		Role:  role,
	}}, nil
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
	if err := requireSecureBootstrap(ctx); err != nil {
		return nil, err
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

	// Docker Compose agent uses NORIX_BOOTSTRAP_JOIN_TOKEN / NORIX_JOIN_TOKEN (#10).
	if n, err := db.SeedBootstrapJoinToken(ctx, service.bootstrapPool); err == nil && n > 0 {
		slog.Info("seeded bootstrap join token after initial admin", "tokens", n)
	}

	return &paasv1.CreateInitialAdminResponse{Token: token, User: roleUser(created)}, nil
}

func (service AuthService) CreatePersonalAccessToken(ctx context.Context, request *paasv1.CreatePersonalAccessTokenRequest) (*paasv1.CreatePersonalAccessTokenResponse, error) {
	identity, ok := SessionFromContext(ctx)
	if err := requireSession(identity, ok); err != nil {
		return nil, err
	}
	name := strings.TrimSpace(request.GetName())
	if name == "" {
		return nil, invalidArgument("name is required")
	}
	var expires *time.Time
	if raw := strings.TrimSpace(request.GetExpiresAt()); raw != "" {
		parsed, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			return nil, invalidArgument("expires_at must be RFC3339")
		}
		if parsed.Before(time.Now().UTC()) {
			return nil, invalidArgument("expires_at must be in the future")
		}
		expires = &parsed
	}
	tok, plain, err := db.CreateUserAPIToken(ctx, service.appPool, identity.OrganizationID, identity.UserID, name, request.GetScopes(), expires, db.AuditActor{
		UserID: identity.UserID, Email: identity.Email,
	})
	if err != nil {
		return nil, invalidArgument(err.Error())
	}
	return &paasv1.CreatePersonalAccessTokenResponse{
		Token:     patToProto(tok),
		Plaintext: plain,
	}, nil
}

func (service AuthService) ListPersonalAccessTokens(ctx context.Context, _ *paasv1.ListPersonalAccessTokensRequest) (*paasv1.ListPersonalAccessTokensResponse, error) {
	identity, ok := SessionFromContext(ctx)
	if err := requireSession(identity, ok); err != nil {
		return nil, err
	}
	items, err := db.ListUserAPITokens(ctx, service.appPool, identity.OrganizationID, identity.UserID)
	if err != nil {
		return nil, internalError("list pats", err)
	}
	out := make([]*paasv1.PersonalAccessToken, 0, len(items))
	for _, item := range items {
		out = append(out, patToProto(item))
	}
	return &paasv1.ListPersonalAccessTokensResponse{Tokens: out}, nil
}

func (service AuthService) RevokePersonalAccessToken(ctx context.Context, request *paasv1.RevokePersonalAccessTokenRequest) (*paasv1.RevokePersonalAccessTokenResponse, error) {
	identity, ok := SessionFromContext(ctx)
	if err := requireSession(identity, ok); err != nil {
		return nil, err
	}
	id := strings.TrimSpace(request.GetId())
	if id == "" {
		return nil, invalidArgument("id is required")
	}
	if err := db.RevokeUserAPIToken(ctx, service.appPool, identity.OrganizationID, identity.UserID, id, db.AuditActor{
		UserID: identity.UserID, Email: identity.Email,
	}); err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, connect.NewError(connect.CodeNotFound, "token not found")
		}
		return nil, internalError("revoke pat", err)
	}
	return &paasv1.RevokePersonalAccessTokenResponse{}, nil
}

func patToProto(tok db.UserAPIToken) *paasv1.PersonalAccessToken {
	format := func(t *time.Time) string {
		if t == nil {
			return ""
		}
		return t.UTC().Format(time.RFC3339)
	}
	return &paasv1.PersonalAccessToken{
		Id: tok.ID, Name: tok.Name, TokenPrefix: tok.TokenPrefix, Scopes: tok.Scopes,
		ExpiresAt: format(tok.ExpiresAt), RevokedAt: format(tok.RevokedAt),
		LastUsedAt: format(tok.LastUsedAt), CreatedAt: tok.CreatedAt.UTC().Format(time.RFC3339),
	}
}
