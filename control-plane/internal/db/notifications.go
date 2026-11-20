package db

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/mail"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/xiron902224/norix/control-plane/internal/gitssrf"
	"github.com/xiron902224/norix/control-plane/internal/notify"
)

const maxNotificationChannels = 20

// ErrNotificationInvalid is a caller error (bad name, kind, target, or events).
var ErrNotificationInvalid = errors.New("invalid notification channel")

// ErrNotificationNotFound is returned when the channel is missing for this tenant.
var ErrNotificationNotFound = errors.New("notification channel not found")

// NotificationChannel is a project destination. TargetHint never contains a secret path.
type NotificationChannel struct {
	ID         string
	ProjectID  string
	Name       string
	Kind       string
	TargetHint string
	Events     []string
	Enabled    bool
}

// NotificationChannelInput is a create or update. An empty Target on update keeps the stored one.
type NotificationChannelInput struct {
	ID        string
	ProjectID string
	Name      string
	Kind      string
	Target    string
	Events    []string
	Enabled   bool
}

// DueNotification is one outbox row claimed for delivery. Target is plaintext in memory only.
type DueNotification struct {
	ID        string
	Kind      string
	Target    string
	EventKind string
	Payload   map[string]string
	Attempts  int
}

func invalidNotification(msg string) error {
	return fmt.Errorf("%w: %s", ErrNotificationInvalid, msg)
}

func normalizeEvents(events []string) ([]string, error) {
	if len(events) == 0 {
		return []string{"deploy.succeeded", "deploy.failed", "node.down"}, nil
	}
	allowed := map[string]bool{
		"deploy.succeeded": true,
		"deploy.failed":    true,
		"node.down":        true,
	}
	out := make([]string, 0, len(events))
	seen := map[string]bool{}
	for _, event := range events {
		event = strings.TrimSpace(event)
		if event == "" {
			continue
		}
		if !allowed[event] {
			return nil, invalidNotification("unknown event " + event)
		}
		if seen[event] {
			continue
		}
		seen[event] = true
		out = append(out, event)
	}
	if len(out) == 0 {
		return nil, invalidNotification("select at least one event")
	}
	return out, nil
}

func validateNotificationTarget(kind, target string) error {
	target = strings.TrimSpace(target)
	if strings.ContainsAny(target, "\n\r\x00") {
		return invalidNotification("target must be a single line")
	}
	switch kind {
	case "email":
		addr, err := mail.ParseAddress(target)
		if err != nil || addr.Address == "" || strings.Contains(addr.Address, " ") {
			return invalidNotification("email address is invalid")
		}
		return nil
	case "webhook", "slack", "discord":
		if err := gitssrf.PolicyFromEnv().ValidateWebhookURL(target); err != nil {
			return invalidNotification(err.Error())
		}
		return nil
	default:
		return invalidNotification("kind must be webhook, slack, discord, or email")
	}
}

func targetHint(kind, target string) string {
	if kind == "email" {
		addr, err := mail.ParseAddress(target)
		if err != nil {
			return "configured"
		}
		return addr.Address
	}
	parsed, err := url.Parse(target)
	if err != nil || parsed.Host == "" {
		return "configured"
	}
	return parsed.Scheme + "://" + parsed.Host + "/…"
}

func scanChannel(row pgx.Row) (NotificationChannel, error) {
	var item NotificationChannel
	var eventsJSON string
	err := row.Scan(&item.ID, &item.ProjectID, &item.Name, &item.Kind, &item.TargetHint, &eventsJSON, &item.Enabled)
	if err != nil {
		return NotificationChannel{}, err
	}
	_ = json.Unmarshal([]byte(eventsJSON), &item.Events)
	return item, nil
}

// ListNotificationChannels returns channels for a project. Values stay encrypted.
func ListNotificationChannels(ctx context.Context, pool *pgxpool.Pool, tenantID, projectID string) ([]NotificationChannel, error) {
	var out []NotificationChannel
	err := withTenantTx(ctx, pool, tenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT id::text, project_id::text, name, kind, target_ciphertext, target_nonce, events::text, enabled
			FROM notification_channels
			WHERE project_id = $1::uuid
			ORDER BY created_at`, projectID)
		if err != nil {
			return err
		}
		defer rows.Close()
		out = []NotificationChannel{}
		for rows.Next() {
			var item NotificationChannel
			var ciphertext, nonce []byte
			var eventsJSON string
			if err := rows.Scan(&item.ID, &item.ProjectID, &item.Name, &item.Kind, &ciphertext, &nonce, &eventsJSON, &item.Enabled); err != nil {
				return err
			}
			plain, decErr := openStackSecret(ciphertext, nonce)
			if decErr != nil {
				item.TargetHint = "unavailable"
			} else {
				item.TargetHint = targetHint(item.Kind, plain)
			}
			_ = json.Unmarshal([]byte(eventsJSON), &item.Events)
			out = append(out, item)
		}
		return rows.Err()
	})
	return out, err
}

// UpsertNotificationChannel creates a channel when ID is empty, or updates it.
func UpsertNotificationChannel(ctx context.Context, pool *pgxpool.Pool, tenantID string, in NotificationChannelInput, actor AuditActor) (NotificationChannel, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" || len(name) > 80 {
		return NotificationChannel{}, invalidNotification("name is required (max 80 characters)")
	}
	kind := strings.ToLower(strings.TrimSpace(in.Kind))
	events, err := normalizeEvents(in.Events)
	if err != nil {
		return NotificationChannel{}, err
	}
	eventsJSON, err := json.Marshal(events)
	if err != nil {
		return NotificationChannel{}, err
	}
	target := strings.TrimSpace(in.Target)
	if in.ID == "" || target != "" {
		if err := validateNotificationTarget(kind, target); err != nil {
			return NotificationChannel{}, err
		}
	}
	var out NotificationChannel
	err = withTenantTx(ctx, pool, tenantID, func(tx pgx.Tx) error {
		if in.ID == "" {
			var count int
			if err := tx.QueryRow(ctx, `
				SELECT count(*) FROM notification_channels WHERE project_id = $1::uuid`, in.ProjectID).Scan(&count); err != nil {
				return err
			}
			if count >= maxNotificationChannels {
				return invalidNotification("a project can have at most 20 notification channels")
			}
			ciphertext, nonce, encErr := sealStackSecret(target)
			if encErr != nil {
				return encErr
			}
			item, scanErr := scanChannel(tx.QueryRow(ctx, `
				INSERT INTO notification_channels (
					organization_id, project_id, name, kind, target_ciphertext, target_nonce, events, enabled
				)
				SELECT $1::uuid, p.id, $3, $4, $5, $6, $7::jsonb, $8
				FROM projects p
				WHERE p.id = $2::uuid
				RETURNING id::text, project_id::text, name, kind, $9, events::text, enabled`,
				tenantID, in.ProjectID, name, kind, ciphertext, nonce, string(eventsJSON), in.Enabled, targetHint(kind, target),
			))
			if errors.Is(scanErr, pgx.ErrNoRows) {
				return ErrNotificationNotFound
			}
			if scanErr != nil {
				return scanErr
			}
			out = item
		} else {
			if target == "" {
				var currentKind string
				err := tx.QueryRow(ctx, `
					SELECT kind FROM notification_channels WHERE id = $1::uuid AND project_id = $2::uuid`,
					in.ID, in.ProjectID).Scan(&currentKind)
				if errors.Is(err, pgx.ErrNoRows) {
					return ErrNotificationNotFound
				}
				if err != nil {
					return err
				}
				if currentKind != kind {
					return invalidNotification("set the target when changing the channel kind")
				}
			}
			var ciphertext, nonce []byte
			if target != "" {
				var encErr error
				ciphertext, nonce, encErr = sealStackSecret(target)
				if encErr != nil {
					return encErr
				}
			}
			item, scanErr := scanChannel(tx.QueryRow(ctx, `
				UPDATE notification_channels SET
					name = $3,
					kind = $4,
					target_ciphertext = CASE WHEN $5::bytea IS NULL THEN target_ciphertext ELSE $5 END,
					target_nonce = CASE WHEN $6::bytea IS NULL THEN target_nonce ELSE $6 END,
					events = $7::jsonb,
					enabled = $8,
					updated_at = NOW()
				WHERE id = $1::uuid AND project_id = $2::uuid
				RETURNING id::text, project_id::text, name, kind,
					CASE WHEN $9 = '' THEN 'configured' ELSE $9 END,
					events::text, enabled`,
				in.ID, in.ProjectID, name, kind, ciphertext, nonce, string(eventsJSON), in.Enabled, targetHint(kind, target),
			))
			if errors.Is(scanErr, pgx.ErrNoRows) {
				return ErrNotificationNotFound
			}
			if scanErr != nil {
				return scanErr
			}
			if target == "" {
				var stored, storedNonce []byte
				if err := tx.QueryRow(ctx, `
					SELECT target_ciphertext, target_nonce FROM notification_channels WHERE id = $1::uuid`, item.ID,
				).Scan(&stored, &storedNonce); err != nil {
					return err
				}
				if plain, decErr := openStackSecret(stored, storedNonce); decErr == nil {
					item.TargetHint = targetHint(item.Kind, plain)
				}
			}
			out = item
		}
		return writeAudit(ctx, tx, tenantID, "notification_channel.upsert", "notification_channel", out.ID, actor, map[string]string{
			"kind": kind, "project_id": in.ProjectID,
		})
	})
	return out, err
}

// DeleteNotificationChannel removes a channel. Outbox rows cascade.
func DeleteNotificationChannel(ctx context.Context, pool *pgxpool.Pool, tenantID, channelID string, actor AuditActor) error {
	return withTenantTx(ctx, pool, tenantID, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `DELETE FROM notification_channels WHERE id = $1::uuid`, channelID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrNotificationNotFound
		}
		return writeAudit(ctx, tx, tenantID, "notification_channel.delete", "notification_channel", channelID, actor, nil)
	})
}

// EnqueueNotification writes one outbox row per matching channel. projectID empty
// means every project in the org (used for node.down). Duplicate keys are ignored.
func EnqueueNotification(ctx context.Context, tx pgx.Tx, tenantID, projectID, eventKind, dedupe string, payload map[string]string) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	var project any
	if strings.TrimSpace(projectID) != "" {
		project = projectID
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO notification_outbox (
			organization_id, project_id, channel_id, event_kind, dedupe_key, payload
		)
		SELECT c.organization_id, c.project_id, c.id, $3, $4, $5::jsonb
		FROM notification_channels c
		WHERE c.organization_id = $1::uuid
		  AND c.enabled
		  AND c.events ? $3
		  AND ($2::uuid IS NULL OR c.project_id = $2::uuid)
		ON CONFLICT (channel_id, dedupe_key) DO NOTHING`,
		tenantID, project, eventKind, dedupe, string(raw))
	return err
}

// EnqueueRecentDeployEvents records deploy.succeeded and deploy.failed for workloads
// that changed in the last two minutes. The dedupe key includes the generation, so
// repeated heartbeats do not send again.
func EnqueueRecentDeployEvents(ctx context.Context, pool *pgxpool.Pool, tenantID string) error {
	return withTenantTx(ctx, pool, tenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT w.id::text, w.project_id::text, w.name, COALESCE(p.name, ''),
			       w.status, w.observed_generation, COALESCE(w.last_error, '')
			FROM workloads w
			JOIN projects p ON p.id = w.project_id
			WHERE w.status IN ('running', 'failed')
			  AND w.observed_generation > 0
			  AND w.updated_at > NOW() - INTERVAL '2 minutes'`)
		if err != nil {
			return err
		}
		defer rows.Close()
		type row struct {
			id, projectID, name, projectName, status, lastError string
			generation                                          int64
		}
		var items []row
		for rows.Next() {
			var item row
			if err := rows.Scan(&item.id, &item.projectID, &item.name, &item.projectName, &item.status, &item.generation, &item.lastError); err != nil {
				return err
			}
			items = append(items, item)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		for _, item := range items {
			event := "deploy.succeeded"
			message := "deploy succeeded"
			if item.status == "failed" {
				event = "deploy.failed"
				message = "deploy failed"
				if strings.TrimSpace(item.lastError) != "" {
					message = truncate(item.lastError, 200)
				}
			}
			payload := map[string]string{
				"event":         event,
				"project_id":    item.projectID,
				"project_name":  item.projectName,
				"workload_id":   item.id,
				"workload_name": item.name,
				"message":       message,
			}
			dedupe := fmt.Sprintf("%s:%s:%d", event, item.id, item.generation)
			if err := EnqueueNotification(ctx, tx, tenantID, item.projectID, event, dedupe, payload); err != nil {
				return err
			}
		}
		return nil
	})
}

// ClaimDueNotifications marks due rows as sending and returns them with the decrypted target.
func ClaimDueNotifications(ctx context.Context, pool *pgxpool.Pool, tenantID string, limit int) ([]DueNotification, error) {
	if limit < 1 || limit > 50 {
		limit = 20
	}
	var out []DueNotification
	err := withTenantTx(ctx, pool, tenantID, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `
			UPDATE notification_outbox
			SET status = 'pending', updated_at = NOW()
			WHERE status = 'sending' AND updated_at < NOW() - INTERVAL '2 minutes'`); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `
			WITH due AS (
				SELECT id FROM notification_outbox
				WHERE status = 'pending' AND next_attempt_at <= NOW()
				ORDER BY next_attempt_at
				LIMIT $1
				FOR UPDATE SKIP LOCKED
			), claimed AS (
				UPDATE notification_outbox o
				SET status = 'sending', attempts = o.attempts + 1, updated_at = NOW()
				FROM due
				WHERE o.id = due.id
				RETURNING o.id, o.channel_id, o.event_kind, o.payload, o.attempts
			)
			SELECT c.id::text, ch.kind, ch.target_ciphertext, ch.target_nonce,
			       c.event_kind, c.payload::text, c.attempts
			FROM claimed c
			JOIN notification_channels ch ON ch.id = c.channel_id`, limit)
		if err != nil {
			return err
		}
		defer rows.Close()
		out = []DueNotification{}
		for rows.Next() {
			var item DueNotification
			var ciphertext, nonce []byte
			var payloadJSON string
			if err := rows.Scan(&item.ID, &item.Kind, &ciphertext, &nonce, &item.EventKind, &payloadJSON, &item.Attempts); err != nil {
				return err
			}
			plain, decErr := openStackSecret(ciphertext, nonce)
			if decErr != nil {
				item.Target = ""
				item.Payload = map[string]string{"event": item.EventKind, "message": "could not decrypt channel target"}
			} else {
				item.Target = plain
				item.Payload = map[string]string{}
				_ = json.Unmarshal([]byte(payloadJSON), &item.Payload)
			}
			out = append(out, item)
		}
		return rows.Err()
	})
	return out, err
}

// CompleteNotification records a delivery result. A nil sendErr marks it delivered.
func CompleteNotification(ctx context.Context, pool *pgxpool.Pool, tenantID, id string, attempts int, sendErr error) error {
	status := "delivered"
	last := ""
	next := time.Now()
	if sendErr != nil {
		last = truncate(sendErr.Error(), 300)
		if notify.GaveUp(attempts) {
			status = "failed"
		} else {
			status = "pending"
			next = notify.NextAttempt(time.Now(), attempts)
		}
	}
	return withTenantTx(ctx, pool, tenantID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			UPDATE notification_outbox
			SET status = $2, last_error = $3, next_attempt_at = $4, updated_at = NOW()
			WHERE id = $1::uuid AND status = 'sending'`,
			id, status, last, next)
		return err
	})
}
