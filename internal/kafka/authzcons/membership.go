// internal/kafka/authzcons/membership.go
package authzcons

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/hotkhwan/gateway-api/internal/services/authzsvc"
	"github.com/hotkhwan/gateway-api/utils/traceutil"

	"github.com/rs/zerolog/log"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
)

// MembershipEvent is the payload for membership changes (camelCase)
type MembershipEvent struct {
	UserId        string `json:"userId,omitempty"`
	RoleId        string `json:"roleId,omitempty"`
	GroupId       string `json:"groupId,omitempty"`
	ParentGroupId string `json:"parentGroupId,omitempty"`
}

var (
	membershipTopics = []string{
		"user.addedToRole",
		"user.removedFromRole",
		"user.addedToGroup",
		"user.removedFromGroup",
		"group.parentChanged",
	}
)

// SubscribeMembershipEvents subscribes to membership events and processes them with bounded concurrency
func SubscribeMembershipEvents(ctx context.Context, svc *authzsvc.MembershipService, concurrency int) {
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup

	for _, topic := range membershipTopics {
		go func(topic string) {
			// TODO: Replace with actual Kafka consumer setup
			for {
				msg := receiveKafkaMessage(topic) // placeholder
				sem <- struct{}{}
				wg.Add(1)
				go func(m []byte) {
					defer func() { <-sem; wg.Done() }()
					ctx := context.Background()
					tracer := otel.Tracer("github.com/hotkhwan/gateway-api/authzcons/membership")
					ctx, span := tracer.Start(ctx, "MembershipEvent")
					defer span.End()

					var event MembershipEvent
					if err := json.Unmarshal(m, &event); err != nil {
						log.Error().Err(err).Str("topic", topic).Msg("failed to unmarshal event")
						return
					}

					if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
						ctx = traceutil.WithTraceID(ctx, sc.TraceID().String())
					}
					logger := log.With().Str("topic", topic).Str("traceId", trace.SpanContextFromContext(ctx).TraceID().String()).Logger()

					// Call internal service to write/revoke tuples
					if err := svc.HandleMembershipEvent(ctx, topic, event); err != nil {
						logger.Error().Err(err).Msg("failed to handle membership event")
					} else {
						logger.Info().Msg("membership event handled")
					}
				}(msg)
			}
		}(topic)
	}
	wg.Wait()
}

// receiveKafkaMessage is a placeholder for actual Kafka consumer logic
func receiveKafkaMessage(_ string) []byte {
	// TODO: Implement Kafka consumer
	return []byte(`{"userId":"u1","roleId":"admin"}`)
}
