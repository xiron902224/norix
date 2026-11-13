package api

import (
	"context"
	"fmt"
	"strconv"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/xiron902224/norix/control-plane/internal/db"
	paasv1 "github.com/xiron902224/norix/control-plane/internal/gen/v1"
	"github.com/xiron902224/norix/control-plane/internal/gen/v1/paasv1connect"
)

type MetricsService struct {
	pool *pgxpool.Pool
}

var _ paasv1connect.MetricsServiceHandler = MetricsService{}

func NewMetricsService(pool *pgxpool.Pool) MetricsService {
	return MetricsService{pool: pool}
}

func (service MetricsService) StreamLogs(ctx context.Context, request *paasv1.StreamLogsRequest, stream paasv1connect.MetricsServiceStreamLogsServerStream) error {
	identity, ok := SessionFromContext(ctx)
	if err := requireSession(identity, ok); err != nil {
		return err
	}
	lines, err := db.ListWorkloadLogs(ctx, service.pool, identity.OrganizationID, request.GetWorkloadId(), 200)
	if err != nil {
		return internalError("stream logs", err)
	}
	for _, line := range lines {
		if err := stream.Send(&paasv1.StreamLogsResponse{
			WorkloadId: request.GetWorkloadId(),
			Content:    line,
			Timestamp:  0,
		}); err != nil {
			return err
		}
	}
	return nil
}

func (service MetricsService) GetClusterMetrics(ctx context.Context, request *paasv1.GetClusterMetricsRequest) (*paasv1.GetClusterMetricsResponse, error) {
	identity, ok := SessionFromContext(ctx)
	if err := requireSession(identity, ok); err != nil {
		return nil, err
	}
	samples, err := db.ListMetricSamples(ctx, service.pool, identity.OrganizationID, request.GetNodeId(), request.GetNamePrefix(), request.GetLimit())
	if err != nil {
		return nil, internalError("get cluster metrics", err)
	}
	out := make([]*paasv1.MetricSample, 0, len(samples))
	for _, sample := range samples {
		out = append(out, &paasv1.MetricSample{
			Id:        fmt.Sprintf("%d", sample.ID),
			NodeId:    sample.NodeID,
			Name:      sample.Name,
			Labels:    sample.Labels,
			Value:     sample.Value,
			Timestamp: sample.TS.Unix(),
		})
	}
	return &paasv1.GetClusterMetricsResponse{Samples: out}, nil
}

func (service MetricsService) QueryLogs(ctx context.Context, request *paasv1.QueryLogsRequest) (*paasv1.QueryLogsResponse, error) {
	identity, ok := SessionFromContext(ctx)
	if err := requireSession(identity, ok); err != nil {
		return nil, err
	}
	segments, err := db.QueryLogSegments(ctx, service.pool, identity.OrganizationID, request.GetProjectId(), request.GetWorkloadId(), request.GetLevel(), request.GetLimit())
	if err != nil {
		return nil, internalError("query logs", err)
	}
	out := make([]*paasv1.LogSegment, 0, len(segments))
	for _, segment := range segments {
		out = append(out, &paasv1.LogSegment{
			Id:         strconv.FormatInt(segment.ID, 10),
			ProjectId:  segment.ProjectID,
			WorkloadId: segment.WorkloadID,
			Body:       segment.Body,
			Level:      segment.Level,
			Timestamp:  segment.TS.Unix(),
		})
	}
	return &paasv1.QueryLogsResponse{Segments: out}, nil
}

func (service MetricsService) ListAlerts(ctx context.Context, request *paasv1.ListAlertsRequest) (*paasv1.ListAlertsResponse, error) {
	identity, ok := SessionFromContext(ctx)
	if err := requireSession(identity, ok); err != nil {
		return nil, err
	}
	rules, err := db.ListAlertRules(ctx, service.pool, identity.OrganizationID)
	if err != nil {
		return nil, internalError("list alert rules", err)
	}
	events, err := db.ListOpsAlertEventsAsObservability(ctx, service.pool, identity.OrganizationID, request.GetLimit())
	if err != nil {
		return nil, internalError("list alert events", err)
	}
	if len(events) == 0 {
		events, err = db.ListObservabilityAlertEvents(ctx, service.pool, identity.OrganizationID, request.GetLimit())
		if err != nil {
			return nil, internalError("list observability alert events", err)
		}
	}
	ruleOut := make([]*paasv1.AlertRule, 0, len(rules))
	for _, rule := range rules {
		ruleOut = append(ruleOut, &paasv1.AlertRule{
			Id:        rule.ID,
			Name:      rule.Name,
			Kind:      rule.Kind,
			Threshold: rule.Threshold,
			Channel:   rule.Channel,
			Enabled:   rule.Enabled,
		})
	}
	eventOut := make([]*paasv1.ObservabilityAlertEvent, 0, len(events))
	for _, event := range events {
		eventOut = append(eventOut, &paasv1.ObservabilityAlertEvent{
			Id:        event.ID,
			RuleId:    event.RuleID,
			Message:   event.Message,
			Status:    event.Status,
			CreatedAt: event.CreatedAt.Unix(),
		})
	}
	return &paasv1.ListAlertsResponse{Rules: ruleOut, Events: eventOut}, nil
}

func (service MetricsService) UpsertAlertRule(ctx context.Context, request *paasv1.UpsertAlertRuleRequest) (*paasv1.UpsertAlertRuleResponse, error) {
	identity, ok := SessionFromContext(ctx)
	if err := requireSession(identity, ok); err != nil {
		return nil, err
	}
	if err := requireMutator(identity); err != nil {
		return nil, err
	}
	rule := request.GetRule()
	if rule == nil {
		return nil, invalidArgument("rule is required")
	}
	stored, err := db.UpsertAlertRule(ctx, service.pool, identity.OrganizationID, db.ObservabilityAlertRule{
		ID:        rule.GetId(),
		Name:      rule.GetName(),
		Kind:      rule.GetKind(),
		Threshold: rule.GetThreshold(),
		Channel:   rule.GetChannel(),
		Enabled:   rule.GetEnabled(),
	})
	if err != nil {
		return nil, internalError("upsert alert rule", err)
	}
	return &paasv1.UpsertAlertRuleResponse{Rule: &paasv1.AlertRule{
		Id:        stored.ID,
		Name:      stored.Name,
		Kind:      stored.Kind,
		Threshold: stored.Threshold,
		Channel:   stored.Channel,
		Enabled:   stored.Enabled,
	}}, nil
}
