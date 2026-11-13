package api

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/xiron902224/norix/control-plane/internal/db"
	paasv1 "github.com/xiron902224/norix/control-plane/internal/gen/v1"
)

type PreviewService struct {
	pool *pgxpool.Pool
}

func NewPreviewService(pool *pgxpool.Pool) PreviewService {
	return PreviewService{pool: pool}
}

func previewToProto(item db.PRPreview) *paasv1.PRPreview {
	return &paasv1.PRPreview{
		Id:              item.ID,
		ProjectId:       item.ProjectID,
		Repo:            item.Repo,
		PrNumber:        item.PRNumber,
		Status:          item.Status,
		PreviewUrl:      item.PreviewURL,
		GithubCommentId: item.GithubCommentID,
		CreatedAt:       item.CreatedAt.Unix(),
	}
}

func (service PreviewService) ListPreviews(ctx context.Context, request *paasv1.ListPreviewsRequest) (*paasv1.ListPreviewsResponse, error) {
	identity, ok := SessionFromContext(ctx)
	if err := requireSession(identity, ok); err != nil {
		return nil, err
	}
	items, err := db.ListPRPreviews(ctx, service.pool, identity.OrganizationID, request.GetProjectId())
	if err != nil {
		return nil, internalError("list previews", err)
	}
	out := make([]*paasv1.PRPreview, 0, len(items))
	for _, item := range items {
		out = append(out, previewToProto(item))
	}
	return &paasv1.ListPreviewsResponse{Previews: out}, nil
}

func (service PreviewService) UpsertPreview(ctx context.Context, request *paasv1.UpsertPreviewRequest) (*paasv1.UpsertPreviewResponse, error) {
	identity, ok := SessionFromContext(ctx)
	if err := requireSession(identity, ok); err != nil {
		return nil, err
	}
	if err := requireMutator(identity); err != nil {
		return nil, err
	}
	preview := request.GetPreview()
	if preview == nil {
		return nil, invalidArgument("preview is required")
	}
	if err := enforceProjectQuota(ctx, service.pool, identity.OrganizationID, preview.GetProjectId(), db.QuotaDelta{
		AdditionalPreviews: 1,
	}); err != nil {
		return nil, err
	}
	stored, err := db.UpsertPRPreview(ctx, service.pool, identity.OrganizationID, db.PRPreview{
		ID:              preview.GetId(),
		ProjectID:       preview.GetProjectId(),
		Repo:            preview.GetRepo(),
		PRNumber:        preview.GetPrNumber(),
		Status:          preview.GetStatus(),
		PreviewURL:      preview.GetPreviewUrl(),
		GithubCommentID: preview.GetGithubCommentId(),
	})
	if err != nil {
		if strings.Contains(err.Error(), "required") {
			return nil, invalidArgument(err.Error())
		}
		return nil, internalError("upsert preview", err)
	}
	return &paasv1.UpsertPreviewResponse{Preview: previewToProto(stored)}, nil
}
