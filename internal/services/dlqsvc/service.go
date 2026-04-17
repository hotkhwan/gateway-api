// internal/services/dlqsvc/service.go
package dlqsvc

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/hotkhwan/gateway-api/config"
	"github.com/hotkhwan/gateway-api/internal/repo/dlqrepo"
	"github.com/hotkhwan/gateway-api/models/gmod"
	"github.com/hotkhwan/gateway-api/models/ingestmod"
	"github.com/rs/zerolog"
)

// DLQService encapsulates all DLQ operations.
type DLQService struct {
	repo *dlqrepo.DLQRepo
	log  zerolog.Logger
}

func NewDLQService(repo *dlqrepo.DLQRepo, log zerolog.Logger) *DLQService {
	return &DLQService{repo: repo, log: log}
}

// List returns paginated DLQ messages for a tenant with optional filters.
func (s *DLQService) List(
	ctx context.Context,
	tenantId, orgId, stage, status string,
	page, perPage int,
) ([]*ingestmod.DLQMessage, *gmod.Pagination, error) {
	return s.repo.List(ctx, tenantId, orgId, stage, status, page, perPage)
}

// Get returns a single DLQ message.
func (s *DLQService) Get(ctx context.Context, tenantId, messageId string) (*ingestmod.DLQMessage, error) {
	msg, err := s.repo.FindById(ctx, tenantId, messageId)
	if err != nil {
		if errors.Is(err, dlqrepo.ErrNotFound) {
			return nil, ErrDLQMessageNotFound
		}
		return nil, err
	}
	return msg, nil
}

// Retry schedules the next retry for a DLQ message.
// Returns ErrAlreadyResolved, ErrMaxRetriesExceeded, ErrRetryTooSoon when applicable.
func (s *DLQService) Retry(ctx context.Context, tenantId, messageId string) (*RetryResult, error) {
	msg, err := s.Get(ctx, tenantId, messageId)
	if err != nil {
		return nil, err
	}

	if msg.Status == "resolved" || msg.Status == "abandoned" {
		return nil, ErrAlreadyResolved
	}
	if msg.RetryCount >= msg.MaxRetries {
		return nil, ErrMaxRetriesExceeded
	}

	// Enforce retry timeout between attempts.
	if msg.RetryCount > 0 && msg.RetryTimeoutSeconds > 0 {
		nextAllowed := msg.LastErrorAt.Add(time.Duration(msg.RetryTimeoutSeconds) * time.Second)
		if time.Now().UTC().Before(nextAllowed) {
			return nil, ErrRetryTooSoon
		}
	}

	newCount := msg.RetryCount + 1
	if err := s.republish(ctx, msg); err != nil {
		s.log.Error().
			Str("messageId", messageId).
			Err(err).
			Msg("[dlqsvc] retry publish failed")
		return nil, err
	}

	if err := s.repo.UpdateStatus(ctx, tenantId, messageId, "retrying", newCount); err != nil {
		s.log.Error().Str("messageId", messageId).Err(err).Msg("[dlqsvc] retry status update failed")
		return nil, err
	}

	s.log.Info().
		Str("messageId", messageId).
		Int("retryCount", newCount).
		Msg("[dlqsvc] retry scheduled")

	return &RetryResult{MessageId: messageId, RetryCount: newCount, Status: "retrying"}, nil
}

// Replay force-republishes a DLQ message regardless of retryCount or timeout.
func (s *DLQService) Replay(ctx context.Context, tenantId, messageId string) (*ReplayResult, error) {
	msg, err := s.Get(ctx, tenantId, messageId)
	if err != nil {
		return nil, err
	}

	if err := s.republish(ctx, msg); err != nil {
		s.log.Error().Str("messageId", messageId).Err(err).Msg("[dlqsvc] replay publish failed")
		return nil, err
	}

	if err := s.repo.UpdateStatus(ctx, tenantId, messageId, "retrying", msg.RetryCount); err != nil {
		s.log.Error().Str("messageId", messageId).Err(err).Msg("[dlqsvc] replay status update failed")
		return nil, err
	}

	s.log.Info().Str("messageId", messageId).Msg("[dlqsvc] replayed")
	return &ReplayResult{MessageId: messageId, Status: "retrying"}, nil
}

// Abandon marks a DLQ message as abandoned (no further retries).
func (s *DLQService) Abandon(ctx context.Context, tenantId, messageId string) error {
	msg, err := s.Get(ctx, tenantId, messageId)
	if err != nil {
		return err
	}
	if msg.Status == "abandoned" {
		return nil // idempotent
	}
	if err := s.repo.UpdateStatus(ctx, tenantId, messageId, "abandoned", msg.RetryCount); err != nil {
		return err
	}
	s.log.Info().Str("messageId", messageId).Msg("[dlqsvc] message abandoned")
	return nil
}

// Stats returns aggregate counts by stage and status.
func (s *DLQService) Stats(ctx context.Context, tenantId, orgId string) (map[string]any, error) {
	return s.repo.Stats(ctx, tenantId, orgId)
}

// republish re-publishes the DLQ message payload back to its original Kafka topic.
func (s *DLQService) republish(ctx context.Context, msg *ingestmod.DLQMessage) error {
	payload, err := json.Marshal(msg.Payload)
	if err != nil {
		return err
	}
	headers := map[string]string{
		"messageId":  msg.MessageId,
		"eventId":    msg.EventId,
		"orgId":      msg.WorkspaceId,
		"tenantId":   msg.TenantId,
		"templateId": msg.TemplateId,
		"dlqRetry":   "true",
	}
	return config.SendToKafkaWithCtx(ctx, msg.Topic, msg.WorkspaceId, payload, headers)
}
