// internal/services/deliverysvc/delivery.go
package deliverysvc

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/hotkhwan/gateway-api/internal/gateways/discordgw"
	"github.com/hotkhwan/gateway-api/internal/gateways/linegw"
	"github.com/hotkhwan/gateway-api/internal/gateways/telegw"
	"github.com/hotkhwan/gateway-api/internal/gateways/webhookgw"
	"github.com/hotkhwan/gateway-api/internal/logger"
	"github.com/hotkhwan/gateway-api/internal/repo/targetrepo"
	"github.com/hotkhwan/gateway-api/models/authzmod"
	"github.com/rs/zerolog"
)

// NormalizedEvent represents an approved/normalized event
type NormalizedEvent struct {
	EventId        string                 `json:"eventId"`
	TenantId       string                 `json:"tenantId"`
	OrgId          string                 `json:"orgId"`
	Name           string                 `json:"name"`
	Lat            float64                `json:"lat"`
	Lng            float64                `json:"lng"`
	EventType      string                 `json:"eventType"`
	NormalizedData map[string]interface{} `json:"normalizedData"`
	SourceIp       string                 `json:"sourceIp"`
	IngestedAt     string                 `json:"ingestedAt"`
	ApprovedAt     string                 `json:"approvedAt"`
}

// DeliveryResult tracks the result of delivery to a target
type DeliveryResult struct {
	TargetId   string
	TargetName string
	Success    bool
	Error      string
	SentAt     time.Time
}

// Service handles delivery of events to configured targets
type Service struct {
	targetRepo *targetrepo.TargetRepo
	logger     zerolog.Logger
}

// NewService creates a new delivery service
func NewService(targetRepo *targetrepo.TargetRepo, logger zerolog.Logger) *Service {
	if targetRepo == nil {
		panic("DeliveryService: targetRepo is required")
	}
	return &Service{
		targetRepo: targetRepo,
		logger:     logger,
	}
}

// HandleNormalizedEvent processes a normalized event and delivers to all enabled targets
func HandleNormalizedEvent(ctx context.Context, event NormalizedEvent) error {
	log := logger.FromCtx(ctx, "deliverysvc", "HandleNormalizedEvent")

	// Get repo instance (singleton pattern for now)
	// TODO: Use dependency injection
	targetRepo := targetrepo.NewTargetRepo()
	service := NewService(targetRepo, log)

	return service.DeliverToAllTargets(ctx, event)
}

// DeliverToAllTargets finds all enabled targets for the org and delivers the event
func (s *Service) DeliverToAllTargets(ctx context.Context, event NormalizedEvent) error {
	log := logger.FromCtx(ctx, "deliverysvc", "DeliverToAllTargets")

	log.Info().
		Str("eventId", event.EventId).
		Str("tenantId", event.TenantId).
		Str("orgId", event.OrgId).
		Str("eventType", event.EventType).
		Msg("Starting delivery to targets")

	// Find all enabled targets for the org
	targets, err := s.targetRepo.FindEnabledByOrg(ctx, event.TenantId, event.OrgId)
	if err != nil {
		log.Error().
			Str("tenantId", event.TenantId).
			Str("orgId", event.OrgId).
			Err(err).
			Msg("failed to find targets for org")
		return fmt.Errorf("failed to find targets: %w", err)
	}

	if len(targets) == 0 {
		log.Debug().
			Str("tenantId", event.TenantId).
			Str("orgId", event.OrgId).
			Msg("no enabled targets found for org")
		return nil
	}

	log.Info().
		Str("orgId", event.OrgId).
		Int("targetCount", len(targets)).
		Msg("found enabled targets")

	// Prepare payload for delivery
	payload, err := json.Marshal(event)
	if err != nil {
		log.Error().
			Str("eventId", event.EventId).
			Err(err).
			Msg("failed to marshal event payload")
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	// Deliver to each target
	results := make([]DeliveryResult, 0, len(targets))
	for _, target := range targets {
		result := s.deliverToTarget(ctx, event, target, payload)
		results = append(results, result)

		if result.Success {
			log.Info().
				Str("targetId", target.TargetId).
				Str("targetName", target.Name).
				Str("type", target.Type).
				Msg("event delivered successfully")
		} else {
			log.Error().
				Str("targetId", target.TargetId).
				Str("targetName", target.Name).
				Str("type", target.Type).
				Str("error", result.Error).
				Msg("event delivery failed")
		}
	}

	// Log summary
	successCount := 0
	for _, r := range results {
		if r.Success {
			successCount++
		}
	}

	log.Info().
		Str("eventId", event.EventId).
		Int("totalTargets", len(targets)).
		Int("successCount", successCount).
		Msg("delivery completed")

	return nil
}

// deliverToTarget sends event to a specific target based on its type
func (s *Service) deliverToTarget(
	ctx context.Context,
	event NormalizedEvent,
	target authzmod.DeliveryTarget,
	payload []byte,
) DeliveryResult {
	result := DeliveryResult{
		TargetId:   target.TargetId,
		TargetName: target.Name,
		SentAt:     time.Now().UTC(),
		Success:    true,
	}

	var err error

	switch target.Type {
	case authzmod.TargetTypeWebhook:
		client := webhookgw.NewClient(target.Config)
		err = client.Send(ctx, event, payload)

	case authzmod.TargetTypeLine:
		client := linegw.NewClient(target.Config)
		err = client.Send(ctx, event, payload)

	case authzmod.TargetTypeTelegram:
		client := telegw.NewClient(target.Config)
		err = client.Send(ctx, event, payload)

	case authzmod.TargetTypeDiscord:
		client := discordgw.NewClient(target.Config)
		err = client.Send(ctx, event, payload)

	default:
		err = fmt.Errorf("unsupported target type: %s", target.Type)
	}

	if err != nil {
		result.Success = false
		result.Error = err.Error()
	}

	return result
}
