package api

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	paasv1 "github.com/xiron902224/norix/control-plane/internal/gen/v1"
	"github.com/xiron902224/norix/control-plane/internal/db"
)

type TeamService struct {
	pool *pgxpool.Pool
}

func NewTeamService(pool *pgxpool.Pool) TeamService {
	return TeamService{pool: pool}
}

func teamToProto(item db.Team) *paasv1.Team {
	return &paasv1.Team{
		Id:        item.ID,
		Name:      item.Name,
		Slug:      item.Slug,
		IsDefault: item.IsDefault,
	}
}

func quotaToProto(item db.TeamQuota) *paasv1.TeamQuota {
	return &paasv1.TeamQuota{
		TeamId:       item.TeamID,
		MaxCpu:       item.MaxCPU,
		MaxMemoryMb:  item.MaxMemoryMB,
		MaxDiskGb:    item.MaxDiskGB,
		MaxSandboxes: item.MaxSandboxes,
		MaxPreviews:  item.MaxPreviews,
	}
}

func usageToProto(item db.TeamUsageSnapshot) *paasv1.TeamUsage {
	return &paasv1.TeamUsage{
		TeamId:        item.TeamID,
		CpuUsed:       item.CPUUsed,
		MemoryMbUsed:  item.MemoryMBUsed,
		DiskGbUsed:    item.DiskGBUsed,
		SandboxesUsed: item.SandboxesUsed,
		PreviewsUsed:  item.PreviewsUsed,
	}
}

func (service TeamService) ListTeams(ctx context.Context, _ *paasv1.ListTeamsRequest) (*paasv1.ListTeamsResponse, error) {
	identity, ok := SessionFromContext(ctx)
	if err := requireSession(identity, ok); err != nil {
		return nil, err
	}
	items, err := db.ListTeams(ctx, service.pool, identity.OrganizationID)
	if err != nil {
		return nil, internalError("list teams", err)
	}
	out := make([]*paasv1.Team, 0, len(items))
	for _, item := range items {
		out = append(out, teamToProto(item))
	}
	return &paasv1.ListTeamsResponse{Teams: out}, nil
}

func (service TeamService) CreateTeam(ctx context.Context, request *paasv1.CreateTeamRequest) (*paasv1.CreateTeamResponse, error) {
	identity, ok := SessionFromContext(ctx)
	if err := requireSession(identity, ok); err != nil {
		return nil, err
	}
	if err := requireMutator(identity); err != nil {
		return nil, err
	}
	item, err := db.CreateTeam(ctx, service.pool, identity.OrganizationID, request.GetName(), request.GetSlug())
	if err != nil {
		if strings.Contains(err.Error(), "required") {
			return nil, invalidArgument(err.Error())
		}
		return nil, internalError("create team", err)
	}
	return &paasv1.CreateTeamResponse{Team: teamToProto(item)}, nil
}

func (service TeamService) AddTeamMember(ctx context.Context, request *paasv1.AddTeamMemberRequest) (*paasv1.AddTeamMemberResponse, error) {
	identity, ok := SessionFromContext(ctx)
	if err := requireSession(identity, ok); err != nil {
		return nil, err
	}
	if err := requireMutator(identity); err != nil {
		return nil, err
	}
	member, err := db.AddTeamMember(ctx, service.pool, identity.OrganizationID, request.GetTeamId(), request.GetUserEmail(), request.GetRole())
	if err != nil {
		if strings.Contains(err.Error(), "required") {
			return nil, invalidArgument(err.Error())
		}
		return nil, internalError("add team member", err)
	}
	return &paasv1.AddTeamMemberResponse{Member: &paasv1.TeamMember{
		UserId: member.UserID,
		Email:  member.Email,
		Name:   member.Name,
		Role:   member.Role,
	}}, nil
}

func (service TeamService) SetTeamQuota(ctx context.Context, request *paasv1.SetTeamQuotaRequest) (*paasv1.SetTeamQuotaResponse, error) {
	identity, ok := SessionFromContext(ctx)
	if err := requireSession(identity, ok); err != nil {
		return nil, err
	}
	if err := requireMutator(identity); err != nil {
		return nil, err
	}
	q := request.GetQuota()
	if q == nil || q.GetTeamId() == "" {
		return nil, invalidArgument("team quota is required")
	}
	stored, err := db.SetTeamQuota(ctx, service.pool, identity.OrganizationID, db.TeamQuota{
		TeamID:       q.GetTeamId(),
		MaxCPU:       q.GetMaxCpu(),
		MaxMemoryMB:  q.GetMaxMemoryMb(),
		MaxDiskGB:    q.GetMaxDiskGb(),
		MaxSandboxes: q.GetMaxSandboxes(),
		MaxPreviews:  q.GetMaxPreviews(),
	})
	if err != nil {
		return nil, internalError("set team quota", err)
	}
	return &paasv1.SetTeamQuotaResponse{Quota: quotaToProto(stored)}, nil
}

func (service TeamService) GetTeamUsage(ctx context.Context, request *paasv1.GetTeamUsageRequest) (*paasv1.GetTeamUsageResponse, error) {
	identity, ok := SessionFromContext(ctx)
	if err := requireSession(identity, ok); err != nil {
		return nil, err
	}
	teamID := strings.TrimSpace(request.GetTeamId())
	if teamID == "" {
		var err error
		teamID, err = db.DefaultTeamID(ctx, service.pool, identity.OrganizationID)
		if err != nil {
			return nil, internalError("default team", err)
		}
	}
	usage, err := db.ComputeTeamUsage(ctx, service.pool, identity.OrganizationID, teamID)
	if err != nil {
		return nil, internalError("team usage", err)
	}
	quota, err := db.GetTeamQuota(ctx, service.pool, identity.OrganizationID, teamID)
	if err != nil {
		return nil, internalError("team quota", err)
	}
	return &paasv1.GetTeamUsageResponse{Usage: usageToProto(usage), Quota: quotaToProto(quota)}, nil
}

func (service TeamService) ListSSOProviders(ctx context.Context, _ *paasv1.ListSSOProvidersRequest) (*paasv1.ListSSOProvidersResponse, error) {
	identity, ok := SessionFromContext(ctx)
	if err := requireSession(identity, ok); err != nil {
		return nil, err
	}
	items, err := db.ListSSOProviders(ctx, service.pool, identity.OrganizationID)
	if err != nil {
		return nil, internalError("list sso providers", err)
	}
	out := make([]*paasv1.SSOProvider, 0, len(items))
	for _, item := range items {
		out = append(out, &paasv1.SSOProvider{
			Id:        item.ID,
			Name:      item.Name,
			Kind:      item.Kind,
			IssuerUrl: item.IssuerURL,
			ClientId:  item.ClientID,
			Enabled:   item.Enabled,
		})
	}
	return &paasv1.ListSSOProvidersResponse{Providers: out}, nil
}

func (service TeamService) UpsertSSOProvider(ctx context.Context, request *paasv1.UpsertSSOProviderRequest) (*paasv1.UpsertSSOProviderResponse, error) {
	identity, ok := SessionFromContext(ctx)
	if err := requireSession(identity, ok); err != nil {
		return nil, err
	}
	if err := requireMutator(identity); err != nil {
		return nil, err
	}
	p := request.GetProvider()
	if p == nil {
		return nil, invalidArgument("provider is required")
	}
	stored, err := db.UpsertSSOProvider(ctx, service.pool, identity.OrganizationID, db.SSOProvider{
		ID:        p.GetId(),
		Name:      p.GetName(),
		Kind:      p.GetKind(),
		IssuerURL: p.GetIssuerUrl(),
		ClientID:  p.GetClientId(),
		Enabled:   p.GetEnabled(),
	}, request.GetClientSecret())
	if err != nil {
		if strings.Contains(err.Error(), "required") || strings.Contains(err.Error(), "must be") {
			return nil, invalidArgument(err.Error())
		}
		return nil, internalError("upsert sso provider", err)
	}
	return &paasv1.UpsertSSOProviderResponse{Provider: &paasv1.SSOProvider{
		Id:        stored.ID,
		Name:      stored.Name,
		Kind:      stored.Kind,
		IssuerUrl: stored.IssuerURL,
		ClientId:  stored.ClientID,
		Enabled:   stored.Enabled,
	}}, nil
}

func (service TeamService) EnableTOTP(ctx context.Context, _ *paasv1.EnableTOTPRequest) (*paasv1.EnableTOTPResponse, error) {
	identity, ok := SessionFromContext(ctx)
	if err := requireSession(identity, ok); err != nil {
		return nil, err
	}
	enroll, err := db.EnableTOTP(ctx, service.pool, identity.OrganizationID, identity.UserID, identity.Email)
	if err != nil {
		return nil, internalError("enable totp", err)
	}
	return &paasv1.EnableTOTPResponse{Secret: enroll.Secret, OtpauthUrl: enroll.OTPAUTHURL}, nil
}

func (service TeamService) VerifyTOTP(ctx context.Context, request *paasv1.VerifyTOTPRequest) (*paasv1.VerifyTOTPResponse, error) {
	identity, ok := SessionFromContext(ctx)
	if err := requireSession(identity, ok); err != nil {
		return nil, err
	}
	okVerified, err := db.VerifyTOTP(ctx, service.pool, identity.OrganizationID, identity.UserID, request.GetCode())
	if err != nil {
		if strings.Contains(err.Error(), "required") || strings.Contains(err.Error(), "invalid") || strings.Contains(err.Error(), "not enabled") {
			return nil, invalidArgument(err.Error())
		}
		return nil, internalError("verify totp", err)
	}
	return &paasv1.VerifyTOTPResponse{Verified: okVerified}, nil
}
