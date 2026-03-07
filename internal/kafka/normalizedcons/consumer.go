// internal/kafka/normalizedcons/consumer.go
package normalizedcons

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/hotkhwan/gateway-api/config"
	"github.com/hotkhwan/gateway-api/models/ingestmod"
	"github.com/segmentio/kafka-go"
)

// StartNormalizerConsumer starts consuming raw.events, normalizes each event,
// writes to event_details (MongoDB) and publishes to normalized.events.
// Consumer group: gateway-normalizer-group
func StartNormalizerConsumer(ctx context.Context, deps ConsumerDeps) {
	broker := os.Getenv("KAFKA_BROKER")
	topic := config.TopicEnv("KAFKA_TOPIC_RAW_EVENTS", "raw.events")
	groupID := "gateway-normalizer-group"

	log := deps.Logger.With().
		Str("topic", topic).
		Str("group", groupID).
		Logger()

	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:        []string{broker},
		Topic:          topic,
		GroupID:        groupID,
		MinBytes:       1e3,
		MaxBytes:       10e6,
		MaxWait:        10 * time.Second,
		CommitInterval: 0,
	})
	defer func() { _ = reader.Close() }()

	log.Info().Msg("normalizer consumer started")

	for {
		m, err := reader.FetchMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				log.Info().Msg("normalizer consumer shutting down")
				return
			}
			log.Error().Err(err).Msg("[normalizer] fetch error")
			time.Sleep(time.Second)
			continue
		}

		if err := handleRawEvent(ctx, m, deps); err != nil {
			log.Error().
				Err(err).
				Int("partition", m.Partition).
				Int64("offset", m.Offset).
				Msg("[normalizer] handler failed — committing offset (DLQ path)")
		}

		_ = reader.CommitMessages(ctx, m)
	}
}

// handleRawEvent processes a single raw.events message:
// decode → apply template → geo enrichment → S3 binaries → upsert event_details → publish normalized.events
func handleRawEvent(ctx context.Context, m kafka.Message, deps ConsumerDeps) error {
	log := deps.Logger

	// 1) Decode CanonicalEvent
	var canonical ingestmod.CanonicalEvent
	if err := json.Unmarshal(m.Value, &canonical); err != nil {
		log.Error().Err(err).Msg("[normalizer] decode failed")
		return err
	}

	// 2) Extract headers
	hdrs := make(map[string]string, len(m.Headers))
	for _, h := range m.Headers {
		hdrs[h.Key] = string(h.Value)
	}
	orgId := hdrs["orgId"]
	tenantId := hdrs["tenantId"]
	templateId := hdrs["templateId"]
	traceId := hdrs["traceId"]

	// Prefer headers over payload fields for tenantId/orgId (more authoritative)
	if canonical.TenantId == "" {
		canonical.TenantId = tenantId
	}
	if canonical.Source.OrgId == "" {
		canonical.Source.OrgId = orgId
	}

	// 3) Apply template mappings → normalizedFields
	normalizedFields := applyTemplate(ctx, canonical, templateId, orgId, deps.TemplateRepo, log)

	// 4) Geo enrichment from lat/lng
	geo := reverseGeocode(canonical.Location.Lat, canonical.Location.Lng, deps.GeoCfg)
	geoCell := computeGeoCell(canonical.Location.Lat, canonical.Location.Lng, deps.GeoCfg)
	byArea := buildByAdminArea(geo)

	// 5) Extract binary fields → S3
	binaryRefs := extractBinaries(ctx, canonical, deps.S3BucketKey, tenantId, orgId, log)

	// 6) Build NormalizedEvent
	now := time.Now().UTC()
	event := &ingestmod.NormalizedEvent{
		EventId:     canonical.EventId,
		TenantId:    canonical.TenantId,
		EventType:   canonical.EventType,
		OccurredAt:  canonical.OccurredAt,
		Source:      canonical.Source,
		Location:    canonical.Location,
		Geo:         geo,
		GeoCell:     geoCell,
		ByAdminArea: byArea,
		Payload:     normalizedFields,
		BinaryRefs:  binaryRefs,
		Meta: ingestmod.NormalizationMeta{
			SchemaVersion: "v1",
			TraceId:       traceId,
			TemplateId:    templateId,
			NormalizedAt:  now,
		},
	}

	// 7) Upsert to event_details (idempotent)
	if err := deps.EventDetailsRepo.Upsert(ctx, event); err != nil {
		log.Error().
			Str("eventId", canonical.EventId).
			Err(err).
			Msg("[normalizer] event_details upsert failed")
		insertDLQ(ctx, deps, canonical, tenantId, orgId, templateId, m.Topic, err.Error())
		return err
	}

	// 7b) If this was a DLQ retry that succeeded, mark DLQ message resolved.
	if hdrs["dlqRetry"] == "true" && deps.DLQRepo != nil {
		msgId := fmt.Sprintf("%s:normalize", canonical.EventId)
		if uerr := deps.DLQRepo.UpdateStatus(ctx, tenantId, msgId, "resolved", 0); uerr != nil {
			log.Warn().Str("messageId", msgId).Err(uerr).Msg("[normalizer] dlq resolve update failed")
		}
	}

	// 8) Publish to normalized.events for downstream delivery
	topic := config.TopicEnv("KAFKA_TOPIC_NORMALIZED_EVENTS", "normalized.events")
	payload, _ := json.Marshal(event)
	pubHeaders := map[string]string{
		"eventId":   canonical.EventId,
		"eventType": canonical.EventType,
		"orgId":     orgId,
		"tenantId":  canonical.TenantId,
	}
	if err := config.SendToKafkaWithCtx(ctx, topic, orgId, payload, pubHeaders); err != nil {
		log.Warn().
			Str("eventId", canonical.EventId).
			Err(err).
			Msg("[normalizer] publish to normalized.events failed (non-blocking, event already saved)")
	}

	log.Debug().
		Str("eventId", canonical.EventId).
		Str("eventType", canonical.EventType).
		Str("orgId", orgId).
		Str("geoCell", geoCell.Cell).
		Str("adminCode", geo.AdminCode).
		Msg("[normalizer] event normalized and saved")

	return nil
}

// insertDLQ inserts a failed normalize event into the dead-letter queue when DLQ is configured.
// Looks up the template's DLQ config; skips silently if DLQ is disabled or repo is nil.
func insertDLQ(
	ctx context.Context,
	deps ConsumerDeps,
	canonical ingestmod.CanonicalEvent,
	tenantId, orgId, templateId, kafkaTopic, reason string,
) {
	if deps.DLQRepo == nil {
		return
	}

	// Resolve DLQ config from template (default: enabled with 3 retries / 60s timeout)
	dlqCfg := ingestmod.DLQConfig{Enabled: true, MaxRetries: 3, RetryTimeoutSeconds: 60}
	if templateId != "" && orgId != "" {
		if tmpl, err := deps.TemplateRepo.FindById(ctx, orgId, templateId); err == nil {
			dlqCfg = tmpl.DLQ
		}
	}
	if !dlqCfg.Enabled {
		return
	}

	now := time.Now().UTC()
	// Use deterministic messageId per event+stage for idempotency
	messageId := fmt.Sprintf("%s:normalize", canonical.EventId)

	// Convert payload to map for storage
	payload := map[string]any{}
	if b, err := json.Marshal(canonical); err == nil {
		_ = json.Unmarshal(b, &payload)
	}

	msg := &ingestmod.DLQMessage{
		MessageId:           messageId,
		EventId:             canonical.EventId,
		TenantId:            tenantId,
		OrgId:               orgId,
		TemplateId:          templateId,
		Topic:               kafkaTopic,
		Stage:               "normalize",
		Reason:              reason,
		Payload:             payload,
		RetryCount:          0,
		MaxRetries:          dlqCfg.MaxRetries,
		RetryTimeoutSeconds: dlqCfg.RetryTimeoutSeconds,
		Status:              "pending",
		LastErrorAt:         now,
		CreatedAt:           now,
		UpdatedAt:           now,
	}

	// Suppress duplicate key errors (same event already in DLQ from a prior attempt)
	if err := deps.DLQRepo.Insert(ctx, msg); err != nil {
		deps.Logger.Warn().
			Str("eventId", canonical.EventId).
			Str("messageId", messageId).
			Err(err).
			Msg("[normalizer] dlq insert failed (may be duplicate)")
	} else {
		deps.Logger.Info().
			Str("eventId", canonical.EventId).
			Str("messageId", messageId).
			Str("reason", reason).
			Msg("[normalizer] event sent to DLQ")
	}

}
