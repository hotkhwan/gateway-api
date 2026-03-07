# Ingest Pipeline — Implementation Plan

> Reference: `baseline/README.md`, `baseline/EVENTS.md`
> Last updated: 2026-03-07

---

## Context & Current State

```
Current (WRONG):
  Approved + template match → processTemplateMappedEvent → MongoDB event_details (direct) + Kafka normalized.events
  Approved + no template   → processAutoApprovedEvent   → MongoDB event_details (direct) + Kafka normalized.events
  Not approved             → event_management (pending) ✅

Target (CORRECT — per baseline):
  Approved + template match → Kafka raw.events → Normalizer → MongoDB event_details + S3
  Approved + no template   → 422 TEMPLATE_MISMATCH ✅ (done)
  Not approved             → event_management (pending) ✅ (done)
```

**Root issue:** `processTemplateMappedEvent` and `processAutoApprovedEvent` bypass Kafka and write directly to MongoDB.
**normalizedData** stored as `json.RawMessage` ([]byte) → BSON binary (base64) instead of JSON document.

---

## Full Pipeline (Target)

```
Device
  └─► POST /events/:orgId
        │
        ├─ NOT approved ──────────────► event_management (statusName: "pending")
        │                                    │
        │                               Operator review:
        │                               PATCH /:id (set eventType)
        │                               POST  /:id/approve
        │                                    │
        ├─ Approved + template match ───┤
        │                               │
        └─ Manual approve (operator) ───┘
                                        │
                                        ▼
                               Kafka: raw.events
                               (CanonicalEvent JSON)
                                        │
                                        ▼
                               Normalizer Consumer
                               (internal/kafka/normalizedcons/)
                                   │          │
                                   ▼          ▼
                            event_details    S3
                            (MongoDB)    (binary only)
                                   │
                                   ▼
                          Kafka: normalized.events
                                   │
                                   ▼
                          Deriver / Distributor
                              │         │
                              ▼         ▼
                           Webhook     DLQ
                           Retry    (dlq_events)
```

---

## Step 1 — Fix Ingest Hot Path: Kafka-first for Approved Events

> **Goal:** `processTemplateMappedEvent` must NOT write to MongoDB directly.
> Publish `CanonicalEvent` to `raw.events` only. Normalizer handles the rest.

### Files to change

| File | Change |
|------|--------|
| `internal/services/ingestsvc/ingest.go` | Replace `processTemplateMappedEvent` body |
| `internal/services/ingestsvc/ingest.go` | Remove `processAutoApprovedEvent` (no longer needed) |
| `internal/repo/ingestdetailsrepo/` | No changes — normalizer writes event_details |

### `processTemplateMappedEvent` — new behavior

```
Old: ApplyMappings → build normalizedData → INSERT event_details → publish normalized.events
New: ApplyMappings → build CanonicalEvent → publish raw.events
```

```go
// internal/services/ingestsvc/ingest.go
func (s *IngestService) processTemplateMappedEvent(
    ctx context.Context,
    tenantId, orgId, deviceKey, eventType, eventId string,
    receivedAt time.Time,
    sourceIp string,
    rawBody map[string]any,
    tmpl *ingestmod.MappingTemplate,
    deviceRef *ingestmod.DeviceIdentity,
) error {
    // 1) Apply field mappings
    mapped, missingRequired := s.tmplMatcher.ApplyMappings(rawBody, tmpl.Mappings)
    if len(missingRequired) > 0 {
        s.logger.Warn()...Strs("missingRequired", missingRequired).Msg("[TemplateMapped] missing required fields")
    }

    // 2) Resolve lat/lng from mapped fields
    lat, _ := getNestedValue(mapped, "location.lat")
    lng, _ := getNestedValue(mapped, "location.lng")
    latF, _ := lat.(float64)
    lngF, _ := lng.(float64)

    // 3) Resolve occurredAt from mapped fields (fallback: receivedAt)
    occurredAt := receivedAt
    if ts, ok := getNestedValue(mapped, "occurredAt"); ok {
        if t, ok := ts.(time.Time); ok {
            occurredAt = t
        }
    }

    // 4) Build CanonicalEvent for raw.events
    deviceId := ""
    deviceType := ""
    if deviceRef != nil {
        deviceId = deviceRef.ID
        deviceType = deviceRef.Type
    }
    canonical := &ingestmod.CanonicalEvent{
        EventId:    eventId,
        EventType:  eventType,
        OccurredAt: occurredAt,
        Source: ingestmod.SourceInfo{
            DeviceId:   deviceId,
            DeviceType: deviceType,
            OrgId:      orgId,
        },
        Location:   ingestmod.LocationInfo{Lat: latF, Lng: lngF},
        Payload:    rawBody,    // raw payload — normalizer applies template again
        CreatedAt:  time.Now().UTC(),
    }

    // 5) Publish to raw.events — normalizer will normalize + save to event_details
    topic := config.TopicEnv("KAFKA_TOPIC_RAW_EVENTS", "raw.events")
    payload, _ := json.Marshal(canonical)
    headers := map[string]string{
        "eventId":    eventId,
        "eventType":  eventType,
        "orgId":      orgId,
        "tenantId":   tenantId,
        "templateId": tmpl.TemplateId,  // hint for normalizer
    }
    if err := config.SendToKafkaWithCtx(ctx, topic, orgId, payload, headers); err != nil {
        return fmt.Errorf("kafka publish failed: %w", err)
    }

    // 6) Cache device:eventType approval for future events
    if deviceKey != "" {
        _ = cacheevt.SetDeviceEventTypeApproved(ctx, tenantId, deviceKey, eventType)
    }
    _ = cacheevt.SetEventStatusApproved(ctx, tenantId, eventId)

    s.logger.Debug()...Str("templateId", tmpl.TemplateId).Msg("[TemplateMapped] published to raw.events")
    return nil
}
```

### Remove `processAutoApprovedEvent`

- Function removed entirely
- Callers (in approved-but-no-template path) already return `ErrTemplateMismatch`
- Any remaining references must compile-error → fix at that point

### Kafka topic mapping

| Topic | Producer | Consumer |
|-------|----------|----------|
| `raw.events` | `IngestService` (approve path) | Normalizer consumer |
| `normalized.events` | Normalizer consumer | Deriver/distributor |

### CanonicalEvent Kafka message shape (raw.events)

```json
{
  "eventId":    "568f7c2c-...",
  "eventType":  "camera_Brand",
  "occurredAt": "2026-03-06T10:00:00Z",
  "source": {
    "deviceId":   "DEV-001",
    "deviceType": "device",
    "orgId":      "8c6226f3-..."
  },
  "location": { "lat": 13.7563, "lng": 100.5018 },
  "payload":  { "deviceId": "DEV-001", "temperature": 36.6 },
  "createdAt": "2026-03-06T17:53:56Z"
}
```

Kafka headers:
- `eventId`, `eventType`, `orgId`, `tenantId`
- `templateId` — hint for normalizer to apply correct template

### Acceptance criteria

- [ ] `processTemplateMappedEvent` contains zero MongoDB writes
- [ ] Kafka header `templateId` populated on template-matched events
- [ ] `processAutoApprovedEvent` removed, no compile errors
- [ ] `go build ./...` passes
- [ ] POST event (approved device + template) → message appears in `raw.events` Kafka topic
- [ ] `event_details` MongoDB collection NOT written by ingest path

---

## Step 2 — Fix `normalizedData` BSON Binary Bug

> **Goal:** `EventDetail.NormalizedData` must store as JSON document in MongoDB, not BSON binary.
> `json.RawMessage` ([]byte) serializes to BSON binary — must use `map[string]any`.

### Files to change

| File | Change |
|------|--------|
| `models/ingestmod/input.go` | Change `NormalizedData json.RawMessage` → `NormalizedData map[string]any` |
| `internal/repo/ingestdetailsrepo/repo.go` | No change needed — type drives serialization |
| `internal/services/ingestsvc/approval.go` | Update `buildCanonicalEvent` / `ApproveEvent` to use `map[string]any` |

### Model fix

```go
// models/ingestmod/input.go (or wherever EventDetail is defined)

// Before
type EventDetail struct {
    ...
    NormalizedData json.RawMessage `json:"normalizedData" bson:"normalizedData"` // ❌ stores as binary
    ...
}

// After
type EventDetail struct {
    ...
    NormalizedData map[string]any `json:"normalizedData" bson:"normalizedData"` // ✅ stores as JSON document
    ...
}
```

### Callers to update

```go
// Before (approval.go)
normalizedData, err := json.Marshal(canonical)
eventDetail := &ingestmod.EventDetail{
    NormalizedData: normalizedData,   // ❌ []byte
}

// After
eventDetail := &ingestmod.EventDetail{
    NormalizedData: map[string]any{   // ✅ map → BSON document
        "eventType":  pending.EventType,
        "occurredAt": pending.CreatedAt,
        "source":     ...,
        "location":   ...,
        "payload":    pending.RawBody,
    },
}
```

### Expected MongoDB shape after fix

```json
{
  "normalizedData": {
    "eventType": "camera_Brand",
    "occurredAt": "2026-03-06T10:00:00Z",
    "source": { "deviceId": "DEV-001", "orgId": "8c6226f3-..." },
    "location": { "lat": 13.7563, "lng": 100.5018 },
    "payload": { "deviceId": "DEV-001", "temperature": 36.6 }
  }
}
```

### Acceptance criteria

- [ ] `EventDetail.NormalizedData` type is `map[string]any`
- [ ] MongoDB Compass shows `normalizedData` as embedded JSON document (not `$binary`)
- [ ] `go build ./...` passes
- [ ] All existing callers updated (no compile errors on `json.RawMessage` assignment)

---

## Step 3 — Normalizer Consumer

> **Goal:** Kafka consumer that consumes `raw.events`, applies template (using `templateId` header),
> normalizes the payload, writes to `event_details` (MongoDB) and S3 (if binary fields present).
> Then publishes to `normalized.events` for downstream delivery.

### New files

| File | Purpose |
|------|---------|
| `internal/kafka/normalizedcons/consumer.go` | Consumer entry point + message handler |
| `internal/kafka/normalizedcons/normalize.go` | Normalization logic: apply template mappings |
| `internal/kafka/normalizedcons/s3writer.go` | Extract + upload binary fields to S3 |

### Consumer structure

```go
// internal/kafka/normalizedcons/consumer.go
package normalizedcons

// StartNormalizerConsumer starts consuming raw.events and writing to event_details.
func StartNormalizerConsumer(
    ctx context.Context,
    eventDetailsRepo *ingestdetailsrepo.EventDetailsRepo,
    templateRepo    *ingestrepo.MappingTemplateRepo,
    s3Client        *stos3minio.Client, // nil = skip S3
    logger          zerolog.Logger,
) {
    consumer := kafka.NewConsumer(
        config.KafkaConfig(),
        config.TopicEnv("KAFKA_TOPIC_RAW_EVENTS", "raw.events"),
        "gateway-normalizer-group",
    )
    consumer.Run(ctx, func(msg kafka.Message) error {
        return handleRawEvent(ctx, msg, eventDetailsRepo, templateRepo, s3Client, logger)
    })
}
```

### Message handler

```go
// internal/kafka/normalizedcons/consumer.go
func handleRawEvent(ctx context.Context, msg kafka.Message, ...) error {
    // 1) Decode CanonicalEvent from message body
    var canonical ingestmod.CanonicalEvent
    if err := json.Unmarshal(msg.Value, &canonical); err != nil {
        return fmt.Errorf("decode failed: %w", err)  // → DLQ
    }

    // 2) Extract templateId from Kafka headers (if present)
    templateId := msg.Headers["templateId"]
    orgId      := msg.Headers["orgId"]
    tenantId   := msg.Headers["tenantId"]

    // 3) Apply template mappings if templateId provided
    normalizedFields := map[string]any{}
    if templateId != "" {
        tmpl, err := templateRepo.FindById(ctx, orgId, templateId)
        if err == nil {
            normalizedFields, _ = applyMappings(canonical.Payload, tmpl.Mappings)
        }
    }

    // 4) Extract binary fields → S3
    s3Refs := map[string]string{}
    if s3Client != nil {
        s3Refs = extractBinaryFields(ctx, canonical, s3Client, tenantId, orgId)
    }

    // 5) Build EventDetail
    now := time.Now().UTC()
    eventDetail := &ingestmod.EventDetail{
        EventId:   canonical.EventId,
        TenantId:  tenantId,
        OrgId:     orgId,
        EventType: canonical.EventType,
        Lat:       canonical.Location.Lat,
        Lng:       canonical.Location.Lng,
        NormalizedData: map[string]any{    // ✅ JSON document, not binary
            "eventType":        canonical.EventType,
            "occurredAt":       canonical.OccurredAt,
            "source":           canonical.Source,
            "location":         canonical.Location,
            "fields":           normalizedFields,
            "rawData":          canonical.Payload,
            "s3Refs":           s3Refs,       // e.g. {"image": "s3://bucket/..."}
            "templateId":       templateId,
        },
        SourceIp:   "",                    // not carried in CanonicalEvent
        IngestedAt: canonical.OccurredAt,
        ApprovedAt: now,
        CreatedAt:  now,
        UpdatedAt:  now,
    }

    // 6) Write to event_details
    if err := eventDetailsRepo.Insert(ctx, eventDetail); err != nil {
        return fmt.Errorf("event_details insert failed: %w", err)  // → DLQ
    }

    // 7) Publish to normalized.events for downstream delivery
    topic := config.TopicEnv("KAFKA_TOPIC_NORMALIZED_EVENTS", "normalized.events")
    payload, _ := json.Marshal(eventDetail)
    headers := map[string]string{
        "eventId":   canonical.EventId,
        "eventType": canonical.EventType,
        "orgId":     orgId,
        "tenantId":  tenantId,
    }
    _ = config.SendToKafkaWithCtx(ctx, topic, orgId, payload, headers)

    return nil
}
```

### S3 binary extraction rules

```
Fields with binary content to extract:
  - Any field whose value is a base64-encoded string AND key matches: image, photo, frame, video, clip, snapshot, thumbnail
  - Extracted → uploaded to S3 at: s3://{bucket}/{tenantId}/{orgId}/events/{eventId}/{fieldName}
  - Original field replaced with S3 reference URL

Example:
  rawBody.image = "base64:..." → S3 upload → s3Refs["image"] = "https://..."
```

### Wiring in `container.go` / `main.go`

```go
// internal/app/container.go
type Container struct {
    ...
    NormalizerConsumer *normalizedcons.Consumer
}

// main.go — startup order
func main() {
    ...
    c := app.NewContainer()

    // Start normalizer consumer
    go normalizedcons.StartNormalizerConsumer(
        ctx,
        c.EventDetailsRepo,
        c.MappingTemplateRepo,
        c.S3Client,   // nil if S3 not configured
        logger.Boot("normalizedcons", "startup"),
    )

    router.Init(c)
    app.Listen(":8080")
}
```

### Consumer group & error handling

| Scenario | Behavior |
|----------|----------|
| JSON decode error | Log ERROR + return error → Kafka retries → DLQ after max retries |
| Template not found | Warn + continue without template mappings |
| MongoDB insert error | Return error → retry → DLQ |
| S3 upload error | Warn + continue without S3 ref (non-blocking) |
| Kafka normalized.events publish error | Warn + continue (event already saved to MongoDB) |

### Acceptance criteria

- [ ] Consumer starts on app boot
- [ ] `raw.events` message → `event_details` record created in MongoDB
- [ ] `normalizedData` field is JSON document (not binary)
- [ ] `normalized.events` message published after successful `event_details` write
- [ ] S3 upload attempted for binary fields (skipped gracefully if S3 unavailable)
- [ ] Consumer group `gateway-normalizer-group` appears in Kafka consumer groups
- [ ] Failed messages route to DLQ topic / logged at ERROR

---

## Step 4 — DLQ (Dead Letter Queue)

> **Goal:** Events that fail delivery (webhook timeout, consumer error, max retries exceeded)
> land in `dlq_events` MongoDB collection with full context for replay/debugging.
> DLQ behaviour (enable/disable, retry count, retry delay) is configured **per MappingTemplate**.
> API to list, inspect, retry, and replay DLQ messages.

### DLQ config — per MappingTemplate

Each `MappingTemplate` carries a `DLQ DLQConfig` field that controls DLQ behaviour for events
matched by that template. When no template is matched, global defaults apply.

```go
// models/ingestmod/mappingTemplate.go
type DLQConfig struct {
    Enabled             bool `json:"enabled"             bson:"enabled"`             // false = skip DLQ for this template
    MaxRetries          int  `json:"maxRetries"          bson:"maxRetries"`          // 0 = no retry; default 3
    RetryTimeoutSeconds int  `json:"retryTimeoutSeconds" bson:"retryTimeoutSeconds"` // delay between retries; default 60
}
```

**Example template body (create/update API):**

```json
{
  "name": "LPR Camera — HQ Gate",
  "match": { "deviceType": "camera", "eventType": "lpr.detected" },
  "mappings": [],
  "dlq": {
    "enabled": true,
    "maxRetries": 5,
    "retryTimeoutSeconds": 120
  }
}
```

**Defaults when `dlq` is omitted or template is unmatched:**

| Field | Default |
|-------|---------|
| `enabled` | `true` |
| `maxRetries` | `3` |
| `retryTimeoutSeconds` | `60` |

---

### DLQ Model

```go
// models/ingestmod/dlq.go
type DLQMessage struct {
    MessageId           string         `json:"messageId"            bson:"messageId"`
    EventId             string         `json:"eventId"              bson:"eventId"`
    TenantId            string         `json:"tenantId"             bson:"tenantId"`
    OrgId               string         `json:"orgId"                bson:"orgId"`
    TemplateId          string         `json:"templateId,omitempty" bson:"templateId,omitempty"` // source template
    Topic               string         `json:"topic"                bson:"topic"`                // Kafka topic that failed
    Stage               string         `json:"stage"                bson:"stage"`                // "normalize" | "deliver" | "webhook"
    Reason              string         `json:"reason"               bson:"reason"`
    Payload             map[string]any `json:"payload"              bson:"payload"`
    RetryCount          int            `json:"retryCount"           bson:"retryCount"`
    MaxRetries          int            `json:"maxRetries"           bson:"maxRetries"`            // from template.DLQ.MaxRetries
    RetryTimeoutSeconds int            `json:"retryTimeoutSeconds"  bson:"retryTimeoutSeconds"`  // from template.DLQ.RetryTimeoutSeconds
    Status              string         `json:"status"               bson:"status"`               // "pending" | "retrying" | "resolved" | "abandoned"
    LastErrorAt         time.Time      `json:"lastErrorAt"          bson:"lastErrorAt"`
    CreatedAt           time.Time      `json:"createdAt"            bson:"createdAt"`
    UpdatedAt           time.Time      `json:"updatedAt"            bson:"updatedAt"`
}
```

### DLQ stages

```
Stage: "normalize"   → Normalizer consumer failed (decode / MongoDB write)
Stage: "deliver"     → normalized.events consumer failed (delivery dispatch)
Stage: "webhook"     → Webhook HTTP delivery failed after max retries
```

### New files

| File | Purpose |
|------|---------|
| `internal/repo/dlqrepo/repo.go` | CRUD for dlq_events collection |
| `internal/repo/dlqrepo/mongoBootstrap.go` | EnsureIndexes |
| `internal/services/dlqsvc/service.go` | List, get, retry, replay, stats |
| `internal/services/dlqsvc/errors.go` | `ErrDLQMessageNotFound`, `ErrAlreadyResolved` |
| `internal/services/dlqsvc/types.go` | `RetryResult`, `ReplayResult`, `DLQStats` |
| `controllers/ingestapi/dlq.go` | HTTP handlers |

### DLQ Repo interface

```go
// internal/repo/dlqrepo/repo.go
type DLQRepo struct{}

func (r *DLQRepo) Insert(ctx, msg *ingestmod.DLQMessage) error
func (r *DLQRepo) FindById(ctx, tenantId, messageId string) (*ingestmod.DLQMessage, error)
func (r *DLQRepo) List(ctx, tenantId, orgId, stage, status string, page, perPage int) ([]*ingestmod.DLQMessage, *gmod.Pagination, error)
func (r *DLQRepo) UpdateStatus(ctx, messageId, status string, retryCount int) error
func (r *DLQRepo) Stats(ctx, tenantId, orgId string) (*DLQStats, error)
```

### DLQ Service

```go
// internal/services/dlqsvc/service.go
type DLQService struct {
    dlqRepo  *dlqrepo.DLQRepo
    producer kafka.Producer  // for replay: re-publish to original topic
}

// Retry: increment retryCount, re-publish message to original Kafka topic
func (s *DLQService) Retry(ctx, tenantId, orgId, messageId string) (*RetryResult, error)

// Replay: re-publish as fresh event to raw.events (full re-process from normalize stage)
func (s *DLQService) Replay(ctx, tenantId, orgId, messageId string) (*ReplayResult, error)

// Abandon: mark status = "abandoned" (won't retry)
func (s *DLQService) Abandon(ctx, tenantId, orgId, messageId string) error
```

### DLQ HTTP API — `/api/v1/ingest/dlq`

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/` | List DLQ messages (filter: stage, status, orgId) |
| `GET` | `/stats` | Count by stage + status |
| `GET` | `/:id` | Get single DLQ message |
| `POST` | `/:id/retry` | Re-publish to original Kafka topic |
| `POST` | `/:id/replay` | Re-process from normalize stage |
| `POST` | `/:id/abandon` | Mark as abandoned |

### Retry response

```json
{
  "code": "SUCCESS",
  "message": "message queued for retry",
  "status": true,
  "detail": {
    "messageId": "...",
    "retryCount": 2,
    "topic": "raw.events",
    "republishedAt": "2026-03-07T10:00:00Z"
  }
}
```

### DLQ Stats response

```json
{
  "code": "SUCCESS",
  "detail": {
    "total": 42,
    "byStage": {
      "normalize": 5,
      "deliver":   30,
      "webhook":   7
    },
    "byStatus": {
      "pending":   20,
      "retrying":  10,
      "resolved":  8,
      "abandoned": 4
    }
  }
}
```

### MongoDB indexes (`dlq_events`)

```go
// Lookup by tenant/org for list endpoint
{ tenantId: 1, orgId: 1, createdAt: -1 }

// Status filter (pending/retrying need attention)
{ tenantId: 1, status: 1 }

// Stage filter
{ tenantId: 1, stage: 1, status: 1 }

// TTL: auto-delete abandoned/resolved after 30 days
{ updatedAt: 1 }, TTL: 30 * 24 * time.Hour, filter: status in ["resolved", "abandoned"]
```

### How DLQ is triggered (from Normalizer consumer)

DLQ config is resolved from the matched template. If the template has `dlq.enabled = false`,
the message is **dropped silently** (no DLQ entry). If no template was matched, global defaults apply.

```go
// internal/kafka/normalizedcons/consumer.go

// resolveDLQConfig returns DLQConfig from matched template, or defaults if nil/unmatched.
func resolveDLQConfig(tmpl *ingestmod.MappingTemplate) ingestmod.DLQConfig {
    const defaultMaxRetries = 3
    const defaultRetryTimeout = 60
    if tmpl == nil {
        return ingestmod.DLQConfig{Enabled: true, MaxRetries: defaultMaxRetries, RetryTimeoutSeconds: defaultRetryTimeout}
    }
    cfg := tmpl.DLQ
    if cfg.MaxRetries == 0 { cfg.MaxRetries = defaultMaxRetries }
    if cfg.RetryTimeoutSeconds == 0 { cfg.RetryTimeoutSeconds = defaultRetryTimeout }
    return cfg
}

func handleRawEvent(...) error {
    ...
    if err := eventDetailsRepo.Insert(ctx, eventDetail); err != nil {
        dlqCfg := resolveDLQConfig(matchedTemplate)
        if !dlqCfg.Enabled {
            logger.Warn().Str("eventId", canonical.EventId).Msg("DLQ disabled for template, dropping failed event")
            return nil
        }
        _ = dlqRepo.Insert(ctx, &ingestmod.DLQMessage{
            MessageId:           uuid.NewString(),
            EventId:             canonical.EventId,
            TenantId:            tenantId,
            OrgId:               orgId,
            TemplateId:          canonical.TemplateId,
            Topic:               "raw.events",
            Stage:               "normalize",
            Reason:              err.Error(),
            Payload:             rawPayloadMap,
            RetryCount:          0,
            MaxRetries:          dlqCfg.MaxRetries,
            RetryTimeoutSeconds: dlqCfg.RetryTimeoutSeconds,
            Status:              "pending",
            CreatedAt:           time.Now().UTC(),
        })
        return nil  // ack message — DLQ handles retry
    }
}
```

### Audit prefix to add

```go
// internal/middleware/audit.go — auditPrefixes
{"/ingest/dlq", "ingestDlq"},
```

### Acceptance criteria

- [ ] `MappingTemplate.DLQ` fields (`enabled`, `maxRetries`, `retryTimeoutSeconds`) persisted in MongoDB
- [ ] Template create/update API accepts `dlq` object; omitted fields fall back to defaults
- [ ] Failed normalizer messages appear in `dlq_events` collection with `maxRetries` + `retryTimeoutSeconds` from template
- [ ] Template with `dlq.enabled: false` → failed events are dropped (no DLQ entry, WARN logged)
- [ ] `DLQMessage.retryTimeoutSeconds` respected by retry scheduler (next retry = `lastErrorAt + retryTimeoutSeconds`)
- [ ] `GET /dlq` lists DLQ messages with filter/pagination
- [ ] `GET /dlq/stats` returns counts by stage and status
- [ ] `POST /dlq/:id/retry` re-publishes to Kafka, increments `retryCount`; blocked if `retryTimeoutSeconds` not elapsed
- [ ] `POST /dlq/:id/replay` re-publishes to `raw.events` (full re-process)
- [ ] `POST /dlq/:id/abandon` sets `status: "abandoned"`
- [ ] TTL index auto-cleans resolved/abandoned after 30 days
- [ ] Swagger annotations on all 6 endpoints
- [ ] Audit prefix `/ingest/dlq` registered

---

## Implementation Order

```
Step 1 → Step 2 → Step 3 → Step 4
  │         │         │         │
  Fix       Fix       Build     Build
  Kafka     BSON      Consumer  DLQ
  flow      type

Step 1+2 can be done in same PR (both are ingest.go / model changes)
Step 3 depends on Step 1+2 (consumer reads CanonicalEvent, writes EventDetail)
Step 4 is independent but requires Step 3 to trigger DLQ entries
```

## PR Map

| PR | Step | Scope |
|----|------|-------|
| **PR-PIPE-1** | Step 1 + 2 | Fix ingest hot path (Kafka-only) + fix NormalizedData BSON type |
| **PR-PIPE-2** | Step 3 | Normalizer consumer + S3 writer |
| **PR-PIPE-3** | Step 4 | DLQ repo + service + controller + API |

**Each PR checklist:**
- [ ] `go build ./...` passes
- [ ] Swagger annotations on all new endpoints
- [ ] `traceutil.StartLite` at every service function entry
- [ ] `logger.Dev` used during implementation, replaced before merge
- [ ] No `logger.Dev` calls in final PR
- [ ] Audit prefix registered for new paths
- [ ] `errors.go` + `types.go` in service package

---

## Key Rules (from baseline)

1. **Kafka publish failure = ERROR + return error** — never swallow
2. **S3 failure = WARN + continue** — binary is best-effort, event must not be lost
3. **DLQ insert failure = ERROR log** — cannot fail silently
4. `normalizedData` is always `map[string]any` — never `json.RawMessage` / binary
5. Normalizer consumer must be **idempotent** — duplicate eventId → `upsert` not `insert`
6. `CanonicalEvent.Payload` = original `rawBody` — normalizer applies template again (source of truth = template, not pre-mapped payload)
7. S3 key format: `{tenantId}/{orgId}/events/{eventId}/{fieldName}`

---

## Addendum — Geo Enrichment in Normalizer (Step 3 Extension)

> Added: 2026-03-07
> Normalizer must compute geo fields from `location.lat` / `location.lng` before writing to `event_details`.

---

### NormalizedEvent — Final Stored Shape

Full shape stored in `event_details` (MongoDB) and published to `normalized.events`:

```json
{
  "eventId":    "uuid",
  "tenantId":   "aisom",
  "eventType":  "face.detected",
  "occurredAt": "2026-03-06T10:00:00Z",

  "source": {
    "deviceId":   "CAM-001",
    "deviceType": "camera",
    "subType":    "edgeai",
    "vendor":     "hikvision",
    "protocol":   "http",
    "orgId":      "8c6226f3-..."
  },

  "ownership": {
    "ownerService": "external-A"
  },

  "location": {
    "lat":  13.7563,
    "lng":  100.5018,
    "site": "Branch-HQ",
    "zone": "Gate-A"
  },

  "geo": {
    "countryCode": "TH",
    "adminLevel":  1,
    "adminName":   "Bangkok",
    "adminCode":   "TH-10",
    "idScheme":    "ISO_3166_2"
  },

  "geoCell": {
    "scheme":    "geohash",
    "precision": 5,
    "cell":      "w3gv2"
  },

  "byAdminArea": {
    "TH-10": {}
  },

  "payload": { "plateNumber": "1กข1234", "confidence": 0.98 },

  "binaryRefs": [
    {
      "objectId":    "aisom/8c6226f3/events/uuid/image",
      "contentType": "image/jpeg",
      "fieldName":   "image"
    }
  ],

  "meta": {
    "schemaVersion": "v1",
    "traceId":       "abc123",
    "templateId":    "fe42ba58-...",
    "normalizedAt":  "2026-03-06T17:53:56Z"
  }
}
```

---

### Geo Computation — Detail

#### 1. GeoCell (Geohash)

Compute spatial cell token from lat/lng. Used for heat maps and area-level aggregation.

```
Precision 5 = ~5km × 5km cell (suitable for city-level aggregation)
Precision 6 = ~1.2km × 0.6km (district-level)
Precision 4 = ~40km × 20km (province-level)
```

Library: `github.com/mmcloughlin/geohash`

```go
// internal/kafka/normalizedcons/geo.go
import "github.com/mmcloughlin/geohash"

func computeGeoCell(lat, lng float64, precision int) GeoCellInfo {
    if lat == 0 && lng == 0 {
        return GeoCellInfo{Scheme: "geohash", Precision: precision}
    }
    cell := geohash.EncodeWithPrecision(lat, lng, uint(precision))
    return GeoCellInfo{
        Scheme:    "geohash",
        Precision: precision,
        Cell:      cell,
    }
}
```

#### 2. Reverse Geocoding → GeoInfo

Resolve lat/lng to administrative area (province/state). Two options:

| Option | Library | Accuracy | Offline |
|--------|---------|----------|---------|
| **A — Embedded GeoJSON** | `github.com/paulmach/orb` + bundled boundaries | Medium | ✅ Yes |
| **B — External API** | Nominatim / Google Maps / HERE | High | ❌ No |
| **C — Pre-built DB** | `github.com/tidwall/rtree` + static TH boundary data | High | ✅ Yes |

**Recommended: Option A for MVP** — embed Thailand GeoJSON admin boundaries (province level).
Future: expand to Option C with full global dataset.

```go
// internal/kafka/normalizedcons/geo.go
func reverseGeocode(lat, lng float64, cfg GeoConfig) GeoInfo {
    if lat == 0 && lng == 0 {
        return GeoInfo{IdScheme: "ISO_3166_2"}
    }

    // Query embedded boundary index
    region := cfg.BoundaryIndex.Query(lat, lng)
    if region == nil {
        return GeoInfo{
            CountryCode: cfg.DefaultCountry, // "TH"
            AdminLevel:  1,
            IdScheme:    "ISO_3166_2",
        }
    }

    return GeoInfo{
        CountryCode: region.CountryCode, // "TH"
        AdminLevel:  region.AdminLevel,  // 1
        AdminName:   region.Name,        // "Bangkok"
        AdminCode:   region.Code,        // "TH-10"
        IdScheme:    "ISO_3166_2",
    }
}
```

#### 3. ByAdminArea Index

Pre-computed index key for fast aggregation dashboard queries.

```go
// Build ByAdminArea from GeoInfo
func buildByAdminArea(geo GeoInfo) ByAdminAreaInfo {
    if geo.AdminCode == "" {
        return ByAdminAreaInfo{}
    }
    return ByAdminAreaInfo{
        geo.AdminCode: map[string]any{},  // downstream aggregator fills counts
    }
}
// Result: { "TH-10": {} }
// MongoDB query: db.event_details.find({"byAdminArea.TH-10": {$exists: true}})
```

---

### GeoConfig — Normalizer Configuration

```go
// internal/kafka/normalizedcons/geo.go
type GeoConfig struct {
    DefaultCountry string          // "TH"
    AdminLevel     int             // 1 (province) or 2 (district)
    IdScheme       string          // "ISO_3166_2"
    GeoCellScheme  string          // "geohash"
    GeoCellPrec    int             // 5
    BoundaryIndex  BoundaryQuerier // interface for pluggable backend
}

type BoundaryQuerier interface {
    Query(lat, lng float64) *AdminRegion
}

type AdminRegion struct {
    CountryCode string
    AdminLevel  int
    Name        string
    Code        string // ISO 3166-2
}

// Default config (from ENV)
func DefaultGeoConfig() GeoConfig {
    return GeoConfig{
        DefaultCountry: envutil.Str("GEO_DEFAULT_COUNTRY", "TH"),
        AdminLevel:     envutil.Int("GEO_ADMIN_LEVEL", 1),
        IdScheme:       "ISO_3166_2",
        GeoCellScheme:  envutil.Str("GEO_CELL_SCHEME", "geohash"),
        GeoCellPrec:    envutil.Int("GEO_CELL_PRECISION", 5),
        BoundaryIndex:  geoboundary.LoadEmbedded(), // embedded static dataset
    }
}
```

---

### Normalizer Consumer — Updated Handler (with geo)

```go
// internal/kafka/normalizedcons/consumer.go
func handleRawEvent(ctx context.Context, msg kafka.Message, deps ConsumerDeps) error {
    var canonical ingestmod.CanonicalEvent
    if err := json.Unmarshal(msg.Value, &canonical); err != nil {
        return writeDLQ(ctx, deps.DLQRepo, msg, "normalize", "decode_failed", err)
    }

    orgId     := msg.Headers["orgId"]
    tenantId  := msg.Headers["tenantId"]
    templateId := msg.Headers["templateId"]

    // 1) Apply template mappings
    normalizedFields := applyTemplate(ctx, canonical, templateId, orgId, deps.TemplateRepo)

    // 2) Compute geo enrichment from lat/lng
    geo     := reverseGeocode(canonical.Location.Lat, canonical.Location.Lng, deps.GeoCfg)
    geoCell := computeGeoCell(canonical.Location.Lat, canonical.Location.Lng, deps.GeoCfg.GeoCellPrec)
    byArea  := buildByAdminArea(geo)

    // 3) Extract + upload binary fields to S3
    binaryRefs := extractBinaries(ctx, canonical, deps.S3Client, tenantId, orgId)

    // 4) Build NormalizedEvent
    now := time.Now().UTC()
    event := &ingestmod.NormalizedEvent{
        EventId:    canonical.EventId,
        TenantId:   tenantId,
        EventType:  canonical.EventType,
        OccurredAt: canonical.OccurredAt,
        Source:     canonical.Source,
        Location:   canonical.Location,
        Geo:        geo,
        GeoCell:    geoCell,
        ByAdminArea: byArea,
        Payload:    normalizedFields,    // mapped fields (not raw)
        BinaryRefs: binaryRefs,
        Meta: ingestmod.NormalizationMeta{
            SchemaVersion: "v1",
            TraceId:       msg.Headers["traceId"],
            TemplateId:    templateId,
            NormalizedAt:  now,
        },
    }

    // 5) Upsert to event_details (idempotent)
    if err := deps.EventDetailsRepo.Upsert(ctx, event); err != nil {
        return writeDLQ(ctx, deps.DLQRepo, msg, "normalize", "db_write_failed", err)
    }

    // 6) Publish to normalized.events
    topic := config.TopicEnv("KAFKA_TOPIC_NORMALIZED_EVENTS", "normalized.events")
    payload, _ := json.Marshal(event)
    _ = config.SendToKafkaWithCtx(ctx, topic, orgId, payload, msg.Headers)

    return nil
}
```

---

### MongoDB Indexes for Geo Queries

Add to `EventDetailsRepo.EnsureIndexes()`:

```go
// Geo cell lookup (heat maps)
{ "geoCell.cell": 1, "tenantId": 1 }

// Admin area filter (dashboard by province)
{ "byAdminArea.TH-10": 1 }  // NOT this — use wildcard index:
{ "byAdminArea.$**": 1 }    // wildcard covers all admin codes

// Country + admin level filter
{ "tenantId": 1, "geo.countryCode": 1, "geo.adminCode": 1, "occurredAt": -1 }

// Standard event lookup
{ "tenantId": 1, "orgId": 1, "occurredAt": -1 }
{ "eventId": 1 }, unique: true
```

---

### New ENV Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `GEO_DEFAULT_COUNTRY` | `TH` | Fallback country when reverse geocode fails |
| `GEO_ADMIN_LEVEL` | `1` | 1 = province, 2 = district |
| `GEO_CELL_SCHEME` | `geohash` | Spatial cell scheme |
| `GEO_CELL_PRECISION` | `5` | Geohash precision (5 = ~5km²) |

---

### New Dependencies

```bash
# geohash computation
go get github.com/mmcloughlin/geohash

# GeoJSON parsing (for embedded boundary dataset)
go get github.com/paulmach/orb
go get github.com/paulmach/orb/geojson
```

---

### New Files (Step 3 — updated)

| File | Purpose |
|------|---------|
| `internal/kafka/normalizedcons/consumer.go` | Consumer entry + message router |
| `internal/kafka/normalizedcons/normalize.go` | Apply template mappings |
| `internal/kafka/normalizedcons/geo.go` | `computeGeoCell`, `reverseGeocode`, `buildByAdminArea` |
| `internal/kafka/normalizedcons/s3writer.go` | Extract binary fields → S3 |
| `internal/kafka/normalizedcons/types.go` | `ConsumerDeps`, `GeoConfig`, `BoundaryQuerier` |
| `internal/geoboundary/` | Embedded boundary dataset + loader |
| `internal/geoboundary/th_provinces.go` | Static TH province boundary data (embedded) |

---

### Step 3 Acceptance Criteria (updated)

- [ ] `NormalizedEvent` contains `geo.countryCode`, `geo.adminCode`, `geo.adminName`
- [ ] `NormalizedEvent.geoCell.cell` is valid geohash string at configured precision
- [ ] `byAdminArea` contains `{adminCode: {}}` key for fast index query
- [ ] Events with `lat: 0, lng: 0` → geo fields empty (no false geocoding)
- [ ] Geo computation is offline — no external API calls on hot path
- [ ] `go build ./...` passes with new geo dependencies
- [ ] MongoDB wildcard index on `byAdminArea.$**` created at bootstrap
