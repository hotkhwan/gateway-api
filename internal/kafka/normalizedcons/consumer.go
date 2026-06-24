// internal/kafka/normalizedcons/consumer.go
package normalizedcons

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/hotkhwan/gateway-api/config"
	"github.com/hotkhwan/gateway-api/internal/eventschema"
	"github.com/hotkhwan/gateway-api/internal/gateways/webhookgw"
	"github.com/hotkhwan/gateway-api/internal/mqtt/alertmsg"
	"github.com/hotkhwan/gateway-api/internal/services/classifysvc"
	"github.com/hotkhwan/gateway-api/internal/sourcemapping/aibox"
	"github.com/hotkhwan/gateway-api/internal/sourcemapping/dahua"
	"github.com/hotkhwan/gateway-api/models/ingestmod"
	"github.com/hotkhwan/gateway-api/utils/traceutil"
	"github.com/segmentio/kafka-go"
)

// PublishCanonicalNotify implements CanonicalNotifier using alertmsg.
func (a *alertmsgNotifier) PublishCanonicalNotify(workspaceID, sourceFamily, eventID string) error {
	return alertmsg.PublishCanonicalNotify(workspaceID, sourceFamily, eventID)
}

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
	workspaceId := hdrs["workspaceId"]
	tenantId := hdrs["tenantId"]
	templateId := hdrs["templateId"]
	traceId := hdrs["traceId"]

	// Prefer headers over payload fields for tenantId/workspaceId (more authoritative)
	if canonical.TenantId == "" {
		canonical.TenantId = tenantId
	}
	if canonical.Source.WorkspaceId == "" {
		canonical.Source.WorkspaceId = workspaceId
	}

	// 3) Apply template mappings → normalizedFields
	normalizedFields := applyTemplate(ctx, canonical, templateId, workspaceId, deps.TemplateRepo, log)

	// 3b) Device management enrichment — resolve before geo so lat/lng backfill feeds reverseGeocode.
	var deviceMgmtId string
	if deps.DeviceMgmtResolver != nil && canonical.Source.DeviceId != "" {
		if dm := deps.DeviceMgmtResolver.Resolve(
			ctx, tenantId, workspaceId,
			canonical.SourceFamily, canonical.Source.DeviceType, canonical.Source.DeviceId,
		); dm != nil {
			deviceMgmtId = dm.DeviceMgmtId
			// Backfill canonical.Source.DeviceMgmtId so the persisted event_details
			// document includes the gateway-api device_management UUID. Without this
			// the field was only used downstream in buildBridgeEvent for the Kafka
			// republish path and never reached event_details.
			if canonical.Source.DeviceMgmtId == "" {
				canonical.Source.DeviceMgmtId = dm.DeviceMgmtId
			}
			// Backfill canonical.Location when the event carries no coordinates.
			// This lets reverseGeocode and geoCell use the registered device position.
			if canonical.Location.Lat == 0 && canonical.Location.Lng == 0 {
				canonical.Location.Lat = dm.Lat
				canonical.Location.Lng = dm.Lng
			}
			if canonical.Location.Site == "" && dm.Site != "" {
				canonical.Location.Site = dm.Site
			}
			if canonical.Location.Zone == "" && dm.Zone != "" {
				canonical.Location.Zone = dm.Zone
			}
		}
	}
	// Backfill canonical.Source.SN from raw payload (e.g. AIBOX "sn") when the
	// canonical event did not carry one. Same rationale as DeviceMgmtId above.
	if canonical.Source.SN == "" {
		if v, ok := canonical.Payload["sn"].(string); ok && v != "" {
			canonical.Source.SN = v
		}
	}
	// Backfill zone from payload field (e.g. AIBOX sends "zone": "PHK") when still unset.
	if canonical.Location.Zone == "" {
		if zoneVal, ok := canonical.Payload["zone"]; ok {
			if zoneStr, ok := zoneVal.(string); ok && zoneStr != "" {
				canonical.Location.Zone = zoneStr
			}
		}
	}

	// 4) Geo enrichment from lat/lng (uses backfilled coordinates when event had none)
	geo := reverseGeocode(canonical.Location.Lat, canonical.Location.Lng, deps.GeoCfg)
	geoCell := computeGeoCell(canonical.Location.Lat, canonical.Location.Lng, deps.GeoCfg)
	byArea := buildByAdminArea(geo)

	// 5) Extract binary fields → S3
	binaryRefs := extractBinaries(ctx, canonical, deps.S3BucketKey, workspaceId, log)

	// 5b) Resolve source-family-specific eventType before storing to MongoDB.
	// AIBOX uses a generic "AIBOX" eventType in the raw event; the actual alarm type
	// is encoded as "typeValue" (or "alarmType") in the raw payload as an integer.
	// We read from canonical.Payload (raw, pre-template) because template mappings with
	// valueCodes may have already converted the int to a string in normalizedFields.
	resolvedEventType := canonical.EventType
	if canonical.SourceFamily == "AIBOX" {
		code := aibox.AlarmTypeFromPayload(canonical.Payload)
		resolvedEventType = aibox.ResolveEventType(code, resolvedEventType)
	} else if canonical.SourceFamily == "dahua" {
		// Dahua shares the AIBOX taxonomy: derive {category}.{action} from the
		// raw event (Events[0].Data.Name / detected-object group) instead of the
		// static template eventType. See sourcemapping/dahua.
		resolvedEventType = dahua.EventTypeFromPayload(canonical.Payload, resolvedEventType)
	}

	// 5c) Carry people-counting region geometry into the normalized payload.
	// The mapping template flattens/renames most raw fields and DROPS the
	// region geometry (regionNames / regionRois) — but the people-counting
	// detail view needs them to draw the ROI polygons + crossing direction.
	// Pass them through verbatim from the raw payload under their generic keys
	// when present and not already produced by the template, so they survive
	// into event_details + the normalized wire event. Presence-gated, so
	// non-region source families are unaffected. See klynx-api
	// docs/contracts/edge-ai-events-pipeline-migration.md §2 (region note).
	if normalizedFields == nil {
		normalizedFields = map[string]any{}
	}
	for _, k := range []string{"regionNames", "regionRois"} {
		if _, exists := normalizedFields[k]; exists {
			continue
		}
		if v, ok := canonical.Payload[k]; ok && v != nil {
			normalizedFields[k] = v
		}
	}

	// 6) Build NormalizedEvent
	now := time.Now().UTC()
	evtCategory, evtAction := splitEventType(resolvedEventType)
	event := &ingestmod.NormalizedEvent{
		EventId:       canonical.EventId,
		TenantId:      canonical.TenantId,
		EventType:     resolvedEventType,
		EventCategory: evtCategory,
		EventAction:   evtAction,
		OccurredAt:    canonical.OccurredAt,
		Source:        canonical.Source,
		Location:      canonical.Location,
		Geo:           geo,
		GeoCell:       geoCell,
		ByAdminArea:   byArea,
		Payload:       normalizedFields,
		BinaryRefs:    binaryRefs,
		Meta: ingestmod.NormalizationMeta{
			SchemaVersion: "v1",
			TraceId:       traceId,
			TemplateId:    templateId,
			NormalizedAt:  now,
		},
	}

	// 6b) Entitlement gate (non-fatal — parallel mode)
	if deps.EntitlementSvc != nil {
		if err := deps.EntitlementSvc.CheckIngestAllowed(ctx, workspaceId, len(m.Value)); err != nil {
			log.Warn().Str("eventId", canonical.EventId).Str("workspaceId", workspaceId).Err(err).
				Msg("[normalizer] entitlement gate denied (continuing — parallel mode)")
		}
	}

	// 6c) Authz gate (non-fatal — parallel mode)
	if deps.IngestAuthzGw != nil {
		if ok, err := deps.IngestAuthzGw.CanIngest(ctx, workspaceId, canonical.Source.DeviceId); err != nil {
			log.Warn().Str("eventId", canonical.EventId).Str("workspaceId", workspaceId).Err(err).
				Msg("[normalizer] authz gate error (continuing — parallel mode)")
		} else if !ok {
			log.Warn().Str("eventId", canonical.EventId).Str("workspaceId", workspaceId).Str("deviceId", canonical.Source.DeviceId).
				Msg("[normalizer] authz gate denied (continuing — parallel mode)")
		}
	}

	// 7) Upsert to event_details (idempotent)
	if err := deps.EventDetailsRepo.Upsert(ctx, event); err != nil {
		log.Error().
			Str("eventId", canonical.EventId).
			Err(err).
			Msg("[normalizer] event_details upsert failed")
		insertDLQ(ctx, deps, canonical, tenantId, workspaceId, templateId, m.Topic, err.Error())
		return err
	}

	// 7b) Path A reconciliation — MQTT canonical notify (non-blocking, non-fatal)
	// Allows UI to reconcile the provisional fast alert (Path A) with the canonical event.
	if deps.CanonicalNotifier != nil {
		notifier := deps.CanonicalNotifier
		go func() {
			_ = notifier.PublishCanonicalNotify(workspaceId, canonical.SourceFamily, canonical.EventId)
		}()
	}

	// 7c) If this was a DLQ retry that succeeded, mark DLQ message resolved.
	if hdrs["dlqRetry"] == "true" && deps.DLQRepo != nil {
		msgId := fmt.Sprintf("%s:normalize", canonical.EventId)
		if uerr := deps.DLQRepo.UpdateStatus(ctx, tenantId, msgId, "resolved", 0); uerr != nil {
			log.Warn().Str("messageId", msgId).Err(uerr).Msg("[normalizer] dlq resolve update failed")
		}
	}

	// 7d) Normalize-stage binding dispatch — non-blocking, non-fatal.
	// Lookup bindings where dispatchStage=normalize, evaluate matchFields, and POST to webhook targets.
	if deps.BindingQuerier != nil && deps.TargetLookup != nil {
		go dispatchNormalizeBindings(ctx, workspaceId, tenantId, event, deps)
	}

	// 8+9) Profile-based routing — Wide slice unified topology (r2).
	//
	//   appliance (klynx-mapped OR standalone) → gw.events.normalized.v1 only.
	//                                            klynx-api reads for event_refs;
	//                                            gateway-api.deliverycons v2 reads for dispatch.
	//   saasPublic                              → events.delivery.v1 (unchanged).
	//
	// normalized.events is RETIRED — no longer published (contract §5.7 Phase-C).
	profile := os.Getenv("DEPLOYMENT_PROFILE")

	// Resolve klynxOrgId once (for the bridge event payload). A missing or empty
	// klynxOrgId is no longer a routing branch — all appliance workspaces publish
	// to gw.events.normalized.v1. The klynxOrgId still populates OrgID on the
	// bridge event so klynx-api's event_refs writer can scope correctly; for
	// standalone workspaces we fall back to workspaceId as the routing key.
	var klynxOrgID string
	if deps.KlynxOrgLookup != nil && profile == "appliance" {
		if kid, lookupErr := deps.KlynxOrgLookup.GetKlynxOrgID(ctx, workspaceId); lookupErr == nil {
			klynxOrgID = kid
		} else {
			log.Warn().Str("workspaceId", workspaceId).Err(lookupErr).
				Msg("[normalizer] klynxOrgId lookup failed — continuing with empty orgId")
		}
	}

	var route, reason string
	switch profile {
	case "saasPublic":
		route = "events.delivery.v1"
		reason = "saasPublic"
	case "appliance":
		route = "gw.events.normalized.v1"
		if klynxOrgID != "" {
			reason = "appliance_klynx_mapped"
		} else {
			reason = "appliance_standalone"
		}
	default:
		route = "gw.events.normalized.v1"
		reason = "default_canonical"
	}
	log.Info().
		Str("eventId", canonical.EventId).
		Str("workspaceId", workspaceId).
		Str("profile", profile).
		Str("resolvedKlynxOrgId", klynxOrgID).
		Str("route", route).
		Str("reason", reason).
		Msg("[normalizer] routing decision")

	if klynxOrgID == "" {
		klynxOrgID = workspaceId // fallback: use workspaceId as routing key
	}

	// Stamp severity/eventClass on the canonical (producer) path so klynx
	// consumers (/intDash, /biDash) receive them — Layer C of
	// klynx-api/docs/contracts/event-severity-forwarding.md. Until now
	// classification ran only on the delivery path (deliverycons), so
	// gw.events.normalized.v1 always shipped an empty severity. Template
	// ClassificationRules win; a watchlist default (blacklist→high, redlist→
	// medium) fills severity when no rule set it. Rule-match-only (no forced
	// unknown/none defaults) keeps the wire compact — klynx maps ""→none.
	// See docs/plan/severity-normalize-path-classification.md.
	if templateId != "" {
		if tmpl, err := deps.TemplateRepo.FindById(ctx, workspaceId, templateId); err == nil && tmpl != nil {
			classifysvc.ApplyClassificationRules(event, tmpl.ClassificationRules, false)
		}
	}
	if event.EventSeverity == "" {
		event.EventSeverity = classifysvc.WatchlistSeverityDefault(event.Payload)
	}

	// Build the canonical bridge event from the stored NormalizedEvent.
	bridgeEvt := buildBridgeEvent(event, workspaceId, klynxOrgID, traceId,
		deviceMgmtId, canonical.SourceFamily, canonical.Payload)

	if profile == "saasPublic" {
		// Publish to events.delivery.v1; klynxdeliverycons handles HTTP delivery (unchanged).
		deliveryTopic := config.TopicEnv("KAFKA_TOPIC_EVENTS_DELIVERY", "events.delivery.v1")
		bridgeEvt.OrgID = workspaceId // saasPublic: orgId = workspaceId
		deliveryHeaders := map[string]string{}
		traceutil.InjectHeaders(ctx, deliveryHeaders)
		deliveryPayload, _ := json.Marshal(bridgeEvt)
		if err := config.SendToKafkaWithCtx(ctx, deliveryTopic, workspaceId, deliveryPayload, deliveryHeaders); err != nil {
			log.Warn().Str("eventId", canonical.EventId).Err(err).
				Msg("[normalizer] saasPublic delivery publish failed (non-blocking)")
		}
	} else if deps.EventBridgePub != nil {
		// Appliance + default profiles → gw.events.normalized.v1 (EventBridge path).
		// Covers both klynx-mapped and standalone workspaces. Delivery consumer
		// (gateway-delivery-v2-group) receives directly from this topic.
		if err := deps.EventBridgePub.Publish(ctx, bridgeEvt); err != nil {
			log.Warn().Str("eventId", canonical.EventId).Err(err).
				Msg("[normalizer] eventbridge publish failed (non-blocking)")
		}
	}

	log.Debug().
		Str("eventId", canonical.EventId).
		Str("eventType", canonical.EventType).
		Str("workspaceId", workspaceId).
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
	tenantId, workspaceId, templateId, kafkaTopic, reason string,
) {
	if deps.DLQRepo == nil {
		return
	}

	// Resolve DLQ config from template (default: enabled with 3 retries / 60s timeout)
	dlqCfg := ingestmod.DLQConfig{Enabled: true, MaxRetries: 3, RetryTimeoutSeconds: 60}
	if templateId != "" && workspaceId != "" {
		if tmpl, err := deps.TemplateRepo.FindById(ctx, workspaceId, templateId); err == nil {
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
		WorkspaceId:         workspaceId,
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

// dispatchNormalizeBindings fetches all normalize-stage bindings for the workspace,
// evaluates matchFields against the event payload, and POSTs to each matching target.
// Non-fatal: all failures are logged only.
func dispatchNormalizeBindings(ctx context.Context, workspaceId, tenantId string, event *ingestmod.NormalizedEvent, deps ConsumerDeps) {
	log := deps.Logger.With().
		Str("eventId", event.EventId).
		Str("workspaceId", workspaceId).
		Logger()

	bindings, err := deps.BindingQuerier.GetNormalizeBindings(ctx, workspaceId)
	if err != nil {
		log.Warn().Err(err).Msg("[normalizer] normalize bindings lookup failed (non-fatal)")
		return
	}
	if len(bindings) == 0 {
		return
	}

	payload, _ := json.Marshal(event)

	for _, b := range bindings {
		if !bindingMatchesFields(event.Payload, b.MatchFields) {
			continue
		}

		target, err := deps.TargetLookup.FindByIDAndOrg(ctx, b.TargetID, tenantId, workspaceId)
		if err != nil {
			log.Warn().Err(err).Str("targetId", b.TargetID).
				Msg("[normalizer] target lookup failed — skipping binding")
			continue
		}
		if !target.Enabled {
			continue
		}

		client := webhookgw.NewClient(target.Config)
		if err := client.Send(ctx, event, payload); err != nil {
			log.Warn().Err(err).
				Str("targetId", b.TargetID).
				Str("bindingId", b.ID).
				Msg("[normalizer] normalize binding dispatch failed (non-fatal)")
		} else {
			log.Info().
				Str("targetId", b.TargetID).
				Str("bindingId", b.ID).
				Msg("[normalizer] normalize binding dispatch OK")
		}
	}
}

// bindingMatchesFields returns true when all matchFields key=value conditions are satisfied.
// An empty matchFields map passes all payloads (wildcard match).
func bindingMatchesFields(eventPayload map[string]any, matchFields map[string]any) bool {
	for k, v := range matchFields {
		actual, ok := eventPayload[k]
		if !ok {
			return false
		}
		if bindingFieldString(actual) != bindingFieldString(v) {
			return false
		}
	}
	return true
}

func bindingFieldString(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	b, _ := json.Marshal(v)
	return strings.Trim(string(b), `"`)
}

// splitEventType splits "category.action" into (category, action).
// For "pedestrian.detected" → ("pedestrian", "detected").
// For "vehicle.lpr.detected" → ("vehicle", "lpr.detected").
// Returns ("", "") when eventType is empty or has no dot.
func splitEventType(eventType string) (category, action string) {
	idx := strings.Index(eventType, ".")
	if idx < 0 || idx == len(eventType)-1 {
		return "", ""
	}
	return eventType[:idx], eventType[idx+1:]
}

// buildBridgeEvent constructs the canonical NormalizedEvent forwarded to klynx.
// Schema matches klynx-api's internal/eventbridge/types.go NormalizedEvent exactly
// (copy-by-convention — both repos must be updated together on schema changes).
//
// rawPayload is the original canonical.Payload (pre-template) used as fallback
// for fields that may not survive template mapping (e.g. sn).
func buildBridgeEvent(
	event *ingestmod.NormalizedEvent,
	workspaceId, orgId, traceId string,
	deviceMgmtId string,
	sourceFamily string,
	rawPayload map[string]any,
) eventschema.NormalizedEvent {
	srcCategory, srcAction := splitEventType(event.EventType)
	bridgeEvt := eventschema.NormalizedEvent{
		EventID:        event.EventId,
		SchemaVersion:  event.Meta.SchemaVersion,
		WorkspaceID:    workspaceId,
		OrgID:          orgId,
		SourceType:     event.EventType,
		SourceCategory: srcCategory,
		SourceAction:   srcAction,
		SourceFamily:   sourceFamily,
		OccurredAt:     event.OccurredAt,
		ReceivedAt:     event.Meta.NormalizedAt,
		// Severity + EventClass — admin-classified per ClassificationRule.
		// Layer C — klynx-api docs/contracts/event-severity-forwarding.md §6.
		// Empty when no rule matched OR rule emitted "none" — omitempty on
		// the JSON tag keeps the wire compact and lets pre-feature consumers
		// ignore the field. ingestmod.EventSeverity is populated by
		// normalizesvc.classification before this build site runs.
		Severity:   event.EventSeverity,
		EventClass: event.EventClass,
		// Forward matched templateId so klynx-api delivery can resolve deliveryTargets/messageTemplates.
		// Empty when no template matched (suggestion fallback / pending).
		TemplateID: event.Meta.TemplateId,
		TraceID:    traceId,
	}

	// Source — device identity + context group.
	src := &eventschema.NormalizedSource{
		WorkspaceID:  workspaceId,
		OrgID:        orgId,
		SourceType:   event.EventType,
		SourceFamily: sourceFamily,
		DeviceID:     event.Source.DeviceId,
		DeviceMgmtID: deviceMgmtId,
	}
	// sn: prefer normalized payload, fall back to raw canonical payload.
	sn, _ := event.Payload["sn"].(string)
	if sn == "" {
		sn, _ = rawPayload["sn"].(string)
	}
	src.SN = sn
	// edgeName: device name from normalized payload.
	if v, ok := event.Payload["deviceName"].(string); ok && v != "" {
		src.EdgeName = v
	}
	bridgeEvt.Source = src

	// Payload — eventAttribute contents (with translated codes) + supplementary fields.
	if ea, ok := event.Payload["eventAttribute"].(map[string]any); ok {
		if sourceFamily == "AIBOX" {
			bridgeEvt.Payload = aibox.TranslateEventAttribute(ea)
		} else {
			bridgeEvt.Payload = ea
		}
	} else {
		// No "eventAttribute" sub-key (e.g. a Dahua/ANPR template mapping flat
		// fields like plate/vehicleColor) — forward the full normalized payload
		// instead of dropping it (this is what event_details already stores).
		// Copy so the pictureCoordinates write below never aliases event.Payload.
		cp := make(map[string]any, len(event.Payload))
		for k, v := range event.Payload {
			cp[k] = v
		}
		bridgeEvt.Payload = cp
	}
	// pictureCoordinates: prefer normalized payload, fall back to raw canonical payload.
	if coords, ok := event.Payload["pictureCoordinates"]; ok {
		bridgeEvt.Payload["pictureCoordinates"] = coords
	} else if coords, ok := rawPayload["pictureCoordinates"]; ok {
		bridgeEvt.Payload["pictureCoordinates"] = coords
	}

	// Location — include only when coordinates or site/zone are present.
	if event.Location.Lat != 0 || event.Location.Lng != 0 || event.Location.Site != "" || event.Location.Zone != "" {
		bridgeEvt.Location = &eventschema.NormalizedLocation{
			Lat:  event.Location.Lat,
			Lng:  event.Location.Lng,
			Site: event.Location.Site,
			Zone: event.Location.Zone,
		}
	}

	// Geo — include only when reverse-geocode succeeded (adminCode non-empty).
	if event.Geo.AdminCode != "" {
		bridgeEvt.Geo = &eventschema.NormalizedGeo{
			CountryCode: event.Geo.CountryCode,
			AdminLevel:  event.Geo.AdminLevel,
			AdminName:   event.Geo.AdminName,
			AdminCode:   event.Geo.AdminCode,
			IdScheme:    event.Geo.IdScheme,
		}
	}

	// GeoCell — include only when a cell was computed.
	if event.GeoCell.Cell != "" {
		bridgeEvt.GeoCell = &eventschema.NormalizedGeoCell{
			Cell:      event.GeoCell.Cell,
			Scheme:    event.GeoCell.Scheme,
			Precision: event.GeoCell.Precision,
		}
	}

	// ByAdminArea — pass through as-is.
	if len(event.ByAdminArea) > 0 {
		bridgeEvt.ByAdminArea = event.ByAdminArea
	}

	// BinaryRefs — map ingestmod.BinaryRef → eventschema.NormalizedBinaryRef.
	if len(event.BinaryRefs) > 0 {
		refs := make([]eventschema.NormalizedBinaryRef, 0, len(event.BinaryRefs))
		for _, r := range event.BinaryRefs {
			refs = append(refs, eventschema.NormalizedBinaryRef{
				ObjectID:    r.ObjectId,
				Bucket:      r.Bucket,
				ContentType: r.ContentType,
				Kind:        r.Kind,
				Role:        r.Role,
				SourceIndex: r.SourceIndex,
			})
		}
		bridgeEvt.BinaryRefs = refs
	}

	return bridgeEvt
}
