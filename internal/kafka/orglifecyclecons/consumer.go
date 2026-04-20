// internal/kafka/orglifecyclecons/consumer.go
package orglifecyclecons

import (
	"context"
	"encoding/json"
	"os"
	"time"

	"github.com/hotkhwan/gateway-api/config"
	"github.com/hotkhwan/gateway-api/internal/eventschema"
	"github.com/hotkhwan/gateway-api/internal/logger"
	"github.com/hotkhwan/gateway-api/internal/services/workspacesvc"
	"github.com/hotkhwan/gateway-api/utils/traceutil"
	"github.com/segmentio/kafka-go"
)

// StartOrgLifecycleConsumers starts two consumers:
//   - klynx.org.created.v1  → workspacesvc.ProvisionFromOrg
//   - klynx.org.deleted.v1  → workspacesvc.SuspendFromOrg
//
// Both consumers must run concurrently; call this once from main.go.
func StartOrgLifecycleConsumers(svc *workspacesvc.WorkspaceService) {
	go consumeOrgCreated(svc)
	go consumeOrgDeleted(svc)
}

func consumeOrgCreated(svc *workspacesvc.WorkspaceService) {
	broker := os.Getenv("KAFKA_BROKER")
	topic := config.TopicEnv("KAFKA_TOPIC_ORG_CREATED", "klynx.org.created.v1")
	groupID := "phibek-org-created-grp"
	log := logger.Boot("orglifecyclecons", "consumeOrgCreated").With().Str("topic", topic).Logger()

	r := kafka.NewReader(kafka.ReaderConfig{
		Brokers: []string{broker}, Topic: topic, GroupID: groupID,
		MinBytes: 1e3, MaxBytes: 1e6, MaxWait: 10 * time.Second,
	})
	defer func() { _ = r.Close() }()
	log.Info().Msg("org created consumer started")

	for {
		m, err := r.FetchMessage(context.Background())
		if err != nil {
			log.Error().Err(err).Msg("[orgcreated] fetch error")
			time.Sleep(time.Second)
			continue
		}

		hdrs := extractHeaders(m)
		parentCtx := traceutil.ExtractHeaders(context.Background(), hdrs)
		ctx, end, spanLog := traceutil.StartLite(parentCtx,
			"github.com/hotkhwan/gateway-api/orglifecyclecons",
			"orglifecyclecons.created",
			"orglifecyclecons", "created",
		)

		var ev eventschema.OrgCreatedEvent
		if err := json.Unmarshal(m.Value, &ev); err != nil {
			spanLog.Error().Err(err).Msg("[orgcreated] decode failed — skipping")
			end()
			_ = r.CommitMessages(ctx, m)
			continue
		}

		if _, err := svc.ProvisionFromOrg(ctx, ev); err != nil {
			spanLog.Error().Err(err).Str("orgId", ev.OrgID).Msg("[orgcreated] provision failed")
		}
		end()
		_ = r.CommitMessages(ctx, m)
	}
}

func consumeOrgDeleted(svc *workspacesvc.WorkspaceService) {
	broker := os.Getenv("KAFKA_BROKER")
	topic := config.TopicEnv("KAFKA_TOPIC_ORG_DELETED", "klynx.org.deleted.v1")
	groupID := "phibek-org-deleted-grp"
	log := logger.Boot("orglifecyclecons", "consumeOrgDeleted").With().Str("topic", topic).Logger()

	r := kafka.NewReader(kafka.ReaderConfig{
		Brokers: []string{broker}, Topic: topic, GroupID: groupID,
		MinBytes: 1e3, MaxBytes: 1e6, MaxWait: 10 * time.Second,
	})
	defer func() { _ = r.Close() }()
	log.Info().Msg("org deleted consumer started")

	for {
		m, err := r.FetchMessage(context.Background())
		if err != nil {
			log.Error().Err(err).Msg("[orgdeleted] fetch error")
			time.Sleep(time.Second)
			continue
		}

		hdrs := extractHeaders(m)
		parentCtx := traceutil.ExtractHeaders(context.Background(), hdrs)
		ctx, end, spanLog := traceutil.StartLite(parentCtx,
			"github.com/hotkhwan/gateway-api/orglifecyclecons",
			"orglifecyclecons.deleted",
			"orglifecyclecons", "deleted",
		)

		var ev eventschema.OrgDeletedEvent
		if err := json.Unmarshal(m.Value, &ev); err != nil {
			spanLog.Error().Err(err).Msg("[orgdeleted] decode failed — skipping")
			end()
			_ = r.CommitMessages(ctx, m)
			continue
		}

		if err := svc.SuspendFromOrg(ctx, ev); err != nil {
			spanLog.Error().Err(err).Str("orgId", ev.OrgID).Msg("[orgdeleted] suspend failed")
		}
		end()
		_ = r.CommitMessages(ctx, m)
	}
}

func extractHeaders(m kafka.Message) map[string]string {
	h := make(map[string]string, len(m.Headers))
	for _, hdr := range m.Headers {
		h[hdr.Key] = string(hdr.Value)
	}
	return h
}
