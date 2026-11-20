package api

import (
	"context"
	"errors"
	"strings"

	"connectrpc.com/connect/v2"
	"github.com/xiron902224/norix/control-plane/internal/db"
	paasv1 "github.com/xiron902224/norix/control-plane/internal/gen/v1"
)

func notificationToProto(item db.NotificationChannel) *paasv1.NotificationChannel {
	return &paasv1.NotificationChannel{
		Id:         item.ID,
		ProjectId:  item.ProjectID,
		Name:       item.Name,
		Kind:       item.Kind,
		TargetHint: item.TargetHint,
		Events:     item.Events,
		Enabled:    item.Enabled,
	}
}

func notificationError(operation string, err error) error {
	switch {
	case errors.Is(err, db.ErrNotificationInvalid):
		msg := strings.TrimPrefix(err.Error(), db.ErrNotificationInvalid.Error()+": ")
		return invalidArgument(msg)
	case errors.Is(err, db.ErrNotificationNotFound):
		return connect.NewError(connect.CodeNotFound, "notification channel not found")
	default:
		return internalError(operation, err)
	}
}

func (service ProjectService) ListNotificationChannels(ctx context.Context, request *paasv1.ListNotificationChannelsRequest) (*paasv1.ListNotificationChannelsResponse, error) {
	identity, ok := SessionFromContext(ctx)
	if err := requireSession(identity, ok); err != nil {
		return nil, err
	}
	projectID := strings.TrimSpace(request.GetProjectId())
	if projectID == "" {
		return nil, invalidArgument("project_id is required")
	}
	items, err := db.ListNotificationChannels(ctx, service.pool, identity.OrganizationID, projectID)
	if err != nil {
		return nil, notificationError("list notification channels", err)
	}
	out := make([]*paasv1.NotificationChannel, 0, len(items))
	for _, item := range items {
		out = append(out, notificationToProto(item))
	}
	return &paasv1.ListNotificationChannelsResponse{Channels: out}, nil
}

func (service ProjectService) UpsertNotificationChannel(ctx context.Context, request *paasv1.UpsertNotificationChannelRequest) (*paasv1.UpsertNotificationChannelResponse, error) {
	identity, ok := SessionFromContext(ctx)
	if err := requireSession(identity, ok); err != nil {
		return nil, err
	}
	if err := requireMutator(identity); err != nil {
		return nil, err
	}
	item, err := db.UpsertNotificationChannel(ctx, service.pool, identity.OrganizationID, db.NotificationChannelInput{
		ID:        strings.TrimSpace(request.GetId()),
		ProjectID: strings.TrimSpace(request.GetProjectId()),
		Name:      request.GetName(),
		Kind:      request.GetKind(),
		Target:    request.GetTarget(),
		Events:    request.GetEvents(),
		Enabled:   request.GetEnabled(),
	}, db.AuditActor{UserID: identity.UserID, Email: identity.Email})
	if err != nil {
		return nil, notificationError("upsert notification channel", err)
	}
	return &paasv1.UpsertNotificationChannelResponse{Channel: notificationToProto(item)}, nil
}

func (service ProjectService) DeleteNotificationChannel(ctx context.Context, request *paasv1.DeleteNotificationChannelRequest) (*paasv1.DeleteNotificationChannelResponse, error) {
	identity, ok := SessionFromContext(ctx)
	if err := requireSession(identity, ok); err != nil {
		return nil, err
	}
	if err := requireMutator(identity); err != nil {
		return nil, err
	}
	if strings.TrimSpace(request.GetId()) == "" {
		return nil, invalidArgument("id is required")
	}
	if err := db.DeleteNotificationChannel(ctx, service.pool, identity.OrganizationID, request.GetId(), db.AuditActor{
		UserID: identity.UserID, Email: identity.Email,
	}); err != nil {
		return nil, notificationError("delete notification channel", err)
	}
	return &paasv1.DeleteNotificationChannelResponse{}, nil
}
