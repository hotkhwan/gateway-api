// internal/kafka/entitlementcons/consumer.go
package entitlementcons

import (
	"context"
	"os"

	kafkautil "github.com/hotkhwan/gateway-api/internal/kafka"
	"github.com/hotkhwan/gateway-api/internal/services/entitlementsvc"
	"github.com/hotkhwan/gateway-api/utils/traceutil"
)

// StartEntitlementConsumer consumes klynx.entitlement.snapshot.v1 messages and
// writes each RuntimeEntitlement snapshot into Redis via EntitlementService.
//
// This is the only path through which phibek learns about workspace entitlements.
// klynx-api produces snapshots whenever a workspace's commercial plan changes.
func StartEntitlementConsumer(svc *entitlementsvc.EntitlementService) {
	broker := os.Getenv("KAFKA_BROKER")
	topic := os.Getenv("KAFKA_TOPIC_ENTITLEMENT_SNAPSHOT")
	if topic == "" {
		topic = "klynx.entitlement.snapshot.v1"
	}
	groupID := "phibek-entitlement-grp"

	kafkautil.StartConsumerWithHeaders(
		broker,
		topic,
		groupID,
		func(snap entitlementsvc.RuntimeEntitlement, headers map[string]string) error {
			// Restore parent trace from Kafka message headers
			parentCtx := traceutil.ExtractHeaders(context.Background(), headers)
			ctx, end, log := traceutil.StartLite(
				parentCtx,
				"github.com/hotkhwan/gateway-api/entitlementcons",
				"entitlementcons.handle",
				"entitlementcons", "handle",
			)
			defer end()

			log.Info().
				Str("workspaceId", snap.WorkspaceID).
				Str("planCode", snap.PlanCode).
				Msg("received entitlement snapshot")

			return svc.StoreEntitlement(ctx, &snap)
		},
	)
}
