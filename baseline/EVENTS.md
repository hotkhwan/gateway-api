# Ingest Event Management — Implementation Spec

> Last updated: 2026-03-06  
> Reference: `baseline/README.md`

---

## 0. Field Clarification: `createdAt` vs `occurredAt`

| Field | Meaning |
|---|---|
| `createdAt` | When **our system** stored the record |
| `occurredAt` | When the event **actually happened** on the device |

```
Device captured image  → 10:00:00   (occurredAt)
Gateway received event → 10:00:02   (createdAt)
```

- `createdAt` **always** exists — set by gateway on ingest
- `occurredAt` **derived** from `rawBody` via `fieldMappings` — set by operator mapping
- All timestamps: **RFC3339 UTC**

---

## 1. Architecture Rules

Flow: `repo → service → controller → router`

Router must remain **pure routing** — no repo creation, no index calls, no logic.

---

## 2. Implementation Standards (Every PR)

Every new endpoint and service function **must** include all four:

### Swagger
- Every handler has `@Summary`, `@Tags`, `@Security BearerAuth`, `@Param X-Active-Org`
- Response types use concrete structs — never `interface{}`
- Tags: `ingest-management`, `ingest-mapping-templates`, `ingest-dlq`, `ingest-bulk`

### Logging

During implementation use `logger.Dev` — focused trace logs for **only the component you are building**, without noise from the rest of the system (other devs testing concurrently won't affect your output).

```bash
# .env.local while building this feature
LOG_LEVEL=info                    # global stays quiet
LOG_DEV_COMPONENT=ingestsvc       # only ingestsvc emits trace logs
```

```go
// While building — logger.Dev traces only this component
log := logger.Dev(ctx, "ingestsvc", "approve")
log.Trace().Str("eventId", id).Msg("step_x")

// After done — replace with FromCtx + right level
log := logger.FromCtx(ctx, "ingestsvc", "approve")
log.Info().Str("eventId", id).Msg("event_approved")
```

**⚠️ `logger.Dev` must not exist in any file merged to `main`.**

**Final level guide for ingest domain:**

| Event | Final level |
|---|---|
| Every branch / variable during dev | `logger.Dev` → remove after done |
| Function inputs, template match result | `DEBUG` |
| Event received, approve started | `INFO` |
| Event approved, published to Kafka | `INFO` |
| Mapping validation failed | `WARN` |
| DB write failed, Kafka publish failed | `ERROR` |

### Tracing
- Every service function takes `ctx context.Context` as first argument
- Use `traceutil.StartLite(ctx, tracer, opName, component, source)` at service entry
- `defer end()` immediately after
- Use the returned `log` (pre-enriched with traceId/spanId) for all logs inside that function

```go
// internal/services/ingestsvc/approve.go
func (s *svc) Approve(ctx context.Context, orgId, eventId string) (*ApproveResult, error) {
    ctx, end, log := traceutil.StartLite(ctx,
        "github.com/hotkhwan/gateway-api/services/ingestsvc",
        "Approve", "ingestsvc", "Approve",
    )
    defer end()

    // during implementation — use logger.Dev instead of the span log for trace-level detail
    devlog := logger.Dev(ctx, "ingestsvc", "Approve")
    devlog.Trace().Str("eventId", eventId).Msg("approve_start")
    // ...
    log.Info().Str("eventId", eventId).Msg("event_approved")
}
```

### Audit
Add the path to `auditPrefixes` in `internal/middleware/audit.go`:

```go
{"/ingest/management",         "ingestManagement"},
{"/ingest/mappingTemplates",   "ingestMappingTemplates"},
{"/ingest/dlq",                "ingestDlq"},
```

Audit is automatic for all `POST`, `PATCH`, `DELETE` on these paths once prefix is registered.

---

## 3. Model Package: `models/ingestmod`

Remove `models/eventmod`. All ingest domain models live in `models/ingestmod`.

```
models/
  ingestmod/
    pending.go          ← PendingEvent (event_management collection)
    fieldmapping.go     ← FieldMapping (snapshot on event)
    mappingTemplate.go  ← MappingTemplate + MatchRule
    normalization.go    ← CanonicalEvent, SourceInfo, LocationInfo
    dlq.go              ← DLQMessage
    dashboard.go        ← stats/summary models
```

### `pending.go` — collection: `event_management`

```go
// models/ingestmod/pending.go
package ingestmod

import "time"

type PendingEvent struct {
    EventId       string         `json:"eventId"               bson:"eventId"`
    OrgId         string         `json:"orgId"                 bson:"orgId"`
    EventType     string         `json:"eventType"             bson:"eventType"`
    RawBody       map[string]any `json:"rawBody"               bson:"rawBody"`
    FieldMappings []FieldMapping `json:"fieldMappings"         bson:"fieldMappings"`
    TemplateId    *string        `json:"templateId,omitempty"  bson:"templateId,omitempty"`
    Fingerprint   string         `json:"fingerprint,omitempty" bson:"fingerprint,omitempty"`
    StatusName    string         `json:"statusName"            bson:"statusName"`
    SourceIp      string         `json:"sourceIp,omitempty"    bson:"sourceIp,omitempty"`
    ContentType   string         `json:"contentType,omitempty" bson:"contentType,omitempty"`
    CreatedAt     time.Time      `json:"createdAt"             bson:"createdAt"`
    UpdatedAt     time.Time      `json:"updatedAt"             bson:"updatedAt"`
}
```

**Valid `statusName` values:**

| Value | Meaning |
|---|---|
| `pending` | Received, awaiting operator mapping |
| `mapped` | FieldMappings set, not yet approved |
| `approved` | Passed validation, published to Kafka |
| `rejected` | Rejected by operator |

> **Breaking change from current DB shape:**
> - `rawBody` must be `map[string]any` — never binary
> - Remove flat fields: `lat`, `lng`, `status` (bool), `name`, `tenantId`, `rawAliases`
> - `statusName` (string) replaces `status` (bool)
> - Location derives from `fieldMappings` targeting `location.lat` / `location.lng`

### `fieldmapping.go`

```go
// models/ingestmod/fieldmapping.go
package ingestmod

import "time"

type FieldMapping struct {
    TargetPath string    `json:"targetPath" bson:"targetPath"`
    SourcePath string    `json:"sourcePath" bson:"sourcePath"`
    Transform  string    `json:"transform"  bson:"transform"`
    Required   bool      `json:"required"   bson:"required"`
    Confidence float64   `json:"confidence" bson:"confidence"`
    UpdatedAt  time.Time `json:"updatedAt"  bson:"updatedAt"`
}
```

### `mappingTemplate.go` — collection: `mapping_templates`

```go
// models/ingestmod/mappingTemplate.go
package ingestmod

import "time"

type MappingTemplate struct {
    TemplateId string         `json:"templateId" bson:"templateId"`
    OrgId      string         `json:"orgId"      bson:"orgId"`
    Name       string         `json:"name"       bson:"name"`
    Match      MatchRule      `json:"match"      bson:"match"`
    Mappings   []FieldMapping `json:"mappings"   bson:"mappings"`
    CreatedAt  time.Time      `json:"createdAt"  bson:"createdAt"`
    UpdatedAt  time.Time      `json:"updatedAt"  bson:"updatedAt"`
}

type MatchRule struct {
    Vendor           string `json:"vendor,omitempty"           bson:"vendor,omitempty"`
    Protocol         string `json:"protocol,omitempty"         bson:"protocol,omitempty"`
    DeviceType       string `json:"deviceType,omitempty"       bson:"deviceType,omitempty"`
    SubType          string `json:"subType,omitempty"          bson:"subType,omitempty"`
    EventType        string `json:"eventType,omitempty"        bson:"eventType,omitempty"`
    RawSchemaVersion string `json:"rawSchemaVersion,omitempty" bson:"rawSchemaVersion,omitempty"`
    RawBodyKeyHash   string `json:"rawBodyKeyHash,omitempty"   bson:"rawBodyKeyHash,omitempty"`
}
```

### `normalization.go`

```go
// models/ingestmod/normalization.go
package ingestmod

import "time"

type CanonicalEvent struct {
    EventId    string         `json:"eventId"    bson:"eventId"`
    EventType  string         `json:"eventType"  bson:"eventType"`
    OccurredAt time.Time      `json:"occurredAt" bson:"occurredAt"`
    Source     SourceInfo     `json:"source"     bson:"source"`
    Location   LocationInfo   `json:"location"   bson:"location"`
    Payload    map[string]any `json:"payload"    bson:"payload"`
    CreatedAt  time.Time      `json:"createdAt"  bson:"createdAt"`
}

type SourceInfo struct {
    DeviceId   string `json:"deviceId"   bson:"deviceId"`
    DeviceType string `json:"deviceType" bson:"deviceType"`
    Vendor     string `json:"vendor"     bson:"vendor"`
    OrgId      string `json:"orgId"      bson:"orgId"`
}

type LocationInfo struct {
    Lat float64 `json:"lat" bson:"lat"`
    Lng float64 `json:"lng" bson:"lng"`
}
```

### `dlq.go`

```go
// models/ingestmod/dlq.go
package ingestmod

import "time"

type DLQMessage struct {
    MessageId  string    `json:"messageId"  bson:"messageId"`
    EventId    string    `json:"eventId"    bson:"eventId"`
    Reason     string    `json:"reason"     bson:"reason"`
    RetryCount int       `json:"retryCount" bson:"retryCount"`
    CreatedAt  time.Time `json:"createdAt"  bson:"createdAt"`
}
```

---

## 4. Service Errors & Types

### `internal/services/ingestsvc/errors.go`

```go
// internal/services/ingestsvc/errors.go
package ingestsvc

import "errors"

var (
    ErrEventNotFound    = errors.New("event not found")
    ErrAlreadyApproved  = errors.New("event already approved")
    ErrMappingRequired  = errors.New("mapping required before approval")
    ErrTemplateNotFound = errors.New("template not found")
    ErrInvalidStatus    = errors.New("invalid event status for this operation")
)
```

### `internal/services/ingestsvc/types.go`

```go
// internal/services/ingestsvc/types.go
package ingestsvc

type ApproveResult struct {
    EventId     string
    KafkaTopic  string
    PublishedAt string
}

type MappingValidationError struct {
    MissingTargets []string
    InvalidTargets []string
}

func (e *MappingValidationError) Error() string { return "mapping validation failed" }

type BulkResult struct {
    Succeeded []string
    Failed    []BulkFailItem
}

type BulkFailItem struct {
    EventId string
    Reason  string
}
```

---

## 5. Ingest Handler — Critical Fix

**Current bug:** `rawBody` is stored as `primitive.Binary`.  
**Fix:** decode to `map[string]any` and sanitize BSON-unsafe keys before storing.

```go
// internal/services/ingestsvc/receive.go
package ingestsvc

import (
    "encoding/json"
    "regexp"
    "strings"
)

func decodeRawBody(data []byte) (map[string]any, error) {
    var body map[string]any
    if err := json.Unmarshal(data, &body); err != nil {
        return map[string]any{"_raw": string(data)}, nil
    }
    return sanitizeBSONKeys(body), nil
}

// sanitizeBSONKeys replaces "." and "$" in keys — BSON does not allow them
func sanitizeBSONKeys(m map[string]any) map[string]any {
    out := make(map[string]any, len(m))
    for k, v := range m {
        safe := strings.ReplaceAll(k, ".", "_")
        safe = strings.ReplaceAll(safe, "$", "_")
        if nested, ok := v.(map[string]any); ok {
            v = sanitizeBSONKeys(nested)
        }
        out[safe] = v
    }
    return out
}
```

Resulting document shape in MongoDB:

```json
{
  "eventId":     "8fae4f3b-...",
  "orgId":       "8c6226f3-...",
  "eventType":   "camera_Brand",
  "rawBody": {
    "address":   "Floor3",
    "channelId": 30,
    "deviceId":  1,
    "eventAttribute": { "age": 0, "gender": 2 }
  },
  "fieldMappings": [],
  "templateId":  null,
  "fingerprint": "||camera_Brand|||<hash>",
  "statusName":  "pending",
  "sourceIp":    "49.228.68.186",
  "createdAt":   "2026-03-06T05:19:42Z",
  "updatedAt":   "2026-03-06T05:19:42Z"
}
```

---

## 6. Fingerprint Generation

Format:

```
vendor|protocol|deviceType|subType|eventType|rawSchemaVersion|rawBodyKeyHash
```

`rawBodyKeyHash` = SHA256 of sorted top-level keys of rawBody, truncated to 8 chars.

```go
// internal/services/ingestsvc/fingerprint.go
package ingestsvc

import (
    "crypto/sha256"
    "fmt"
    "sort"
    "strings"

    "github.com/hotkhwan/gateway-api/models/ingestmod"
)

func BuildFingerprint(eventType string, rawBody map[string]any) string {
    keys := make([]string, 0, len(rawBody))
    for k := range rawBody {
        keys = append(keys, k)
    }
    sort.Strings(keys)
    hash := sha256.Sum256([]byte(strings.Join(keys, ",")))
    keyHash := fmt.Sprintf("%x", hash)[:8]

    // vendor|protocol|deviceType|subType|eventType|schemaVersion|keyHash
    // (vendor/protocol/deviceType not yet known at ingest — filled by template match)
    return fmt.Sprintf("||%s|||%s", eventType, keyHash)
}
```

Matching logic:
- **1 match** → auto-bind `templateId`, copy `Mappings` as snapshot → `fieldMappings`, `statusName = "mapped"`
- **0 matches** → `fieldMappings: []`, `statusName = "pending"`
- **>1 matches** → `fieldMappings: []`, `statusName = "pending"` (operator selects)

---

## 7. Route Structure

Base: `/api/v1/ingest`

All routes wired in `router/ingest.go` only — remove `router/fieldmapping.go` and `router/dlq.go`.

### 7.1 Management — `/api/v1/ingest/management`

| Method | Path | Description |
|---|---|---|
| `GET` | `/` | List pending events |
| `GET` | `/:eventId` | Get event detail |
| `PATCH` | `/:eventId` | Update event metadata |
| `PATCH` | `/:eventId/fieldMappings` | Update field mappings |
| `POST` | `/:eventId/approve` | Approve → publish Kafka |
| `POST` | `/:eventId/reject` | Reject event |
| `DELETE` | `/:eventId` | Delete event |

### 7.2 Bulk — `/api/v1/ingest/management/bulk`

| Method | Path | Body |
|---|---|---|
| `POST` | `/applyTemplate` | `{ "eventIds": [], "templateId": "" }` |
| `POST` | `/approve` | `{ "eventIds": [] }` |
| `POST` | `/reject` | `{ "eventIds": [], "reason": "" }` |
| `POST` | `/delete` | `{ "eventIds": [] }` |

Partial success response:

```json
{
  "code": "PARTIAL_SUCCESS",
  "status": true,
  "details": {
    "succeeded": ["id1", "id2"],
    "failed": [
      { "eventId": "id3", "reason": "MAPPING_REQUIRED" }
    ]
  }
}
```

### 7.3 Mapping Templates — `/api/v1/ingest/mappingTemplates`

| Method | Path | Description |
|---|---|---|
| `GET` | `/` | List templates |
| `GET` | `/:templateId` | Get template |
| `POST` | `/` | Create template |
| `PATCH` | `/:templateId` | Update template |
| `DELETE` | `/:templateId` | Delete template |

### 7.4 DLQ — `/api/v1/ingest/dlq`

| Method | Path | Description |
|---|---|---|
| `GET` | `/` | List DLQ messages |
| `GET` | `/stats` | DLQ stats |
| `GET` | `/:id` | Get message |
| `POST` | `/retry` | Retry message |
| `POST` | `/replay` | Replay message |

---

## 8. Approval Gate

Validate before publishing to Kafka. Required `targetPath` values must exist in `fieldMappings` and resolve to non-empty values from `rawBody`:

| Required targetPath | Notes |
|---|---|
| `source.deviceId` | |
| `occurredAt` | must parse as valid timestamp |
| `eventType` | |
| `location.lat` | must be valid float |
| `location.lng` | must be valid float |

Failure — `HTTP 422`:

```json
{
  "code": "MAPPING_REQUIRED",
  "status": false,
  "details": {
    "missingTargets": ["source.deviceId", "occurredAt"],
    "invalidTargets": ["location.lat"]
  }
}
```

---

## 9. Template Workflows

### Flow A — Event First

```
1. Event arrives → statusName: "pending", fieldMappings: []
2. Operator opens event, maps fields manually
3. Operator clicks "Save as Template"
   → POST /ingest/mappingTemplates  { name, match, mappings }
4. Template created, future matching events auto-bind
```

### Flow B — Template First

```
1. Admin creates template
   → POST /ingest/mappingTemplates
2. Event arrives → system computes fingerprint
3. 1 match → auto-bind templateId + copy Mappings snapshot → fieldMappings
4. statusName = "mapped"  (ready to approve without manual mapping)
```

---

## 10. File Structure

```
models/
  ingestmod/
    pending.go
    fieldmapping.go
    mappingTemplate.go
    normalization.go
    dlq.go
    dashboard.go

internal/
  repo/
    ingestrepo/
      pending.go            ← CRUD for event_management
      template.go           ← CRUD for mapping_templates
      mongoBootstrap.go     ← EnsureIndexes registration
  services/
    ingestsvc/
      receive.go            ← ingest + decode rawBody + fingerprint + auto-match
      fingerprint.go        ← BuildFingerprint()
      mapping.go            ← apply/update fieldMappings
      approve.go            ← validate gate + publish Kafka
      bulk.go               ← bulk operations
      errors.go             ← sentinel errors
      types.go              ← service-specific types
    templatesvc/
      crud.go
      errors.go
      types.go
  controllers/
    ingestctl/
      management.go         ← list, get, update, approve, reject, delete
      bulk.go               ← bulk operations
      template.go           ← template CRUD
      dlq.go                ← DLQ endpoints
  router/
    ingest.go               ← ALL ingest routes (merged)
```

---

## 11. Import Change Summary

| Before | After |
|---|---|
| `gateway-api/models/eventmod` | `gateway-api/models/ingestmod` |

Services to update: `fieldsvc`, `normalizesvc`, `dlqsvc`, `routingsvc`, `ingeststatsvc`  
Repos to update: `eventrepo`, `fieldmaprepo`, `dlqrepo`

---

## 12. PR Plan

| PR | Scope | Checklist |
|---|---|---|
| **PR1** | Router merge | Merge `fieldmapping.go` + `dlq.go` → `ingest.go` |
| **PR2** | `models/ingestmod` | New model package, remove `eventmod`, update all imports |
| **PR3** | Fix ingest receive | `rawBody` as `map[string]any`, sanitize keys, add `fingerprint`, fix model fields |
| **PR4** | MappingTemplate CRUD | `templatesvc` + `ingestctl/template.go` + swagger + audit prefix |
| **PR5** | Fingerprint auto-match | On receive: compute fingerprint, query templates, auto-bind if 1 match |
| **PR6** | Bulk operations | `ingestsvc/bulk.go` + `ingestctl/bulk.go` + swagger |
| **PR7** | Approval gate | Validate required targets + publish Kafka `raw.events` |
| **PR8** | Log level review | Promote TRACE → DEBUG/INFO on all stabilized paths |
| **PR9** | Docs + Postman | Swagger complete, Postman collection updated |

**Every PR must include:**
- ✅ Swagger annotations on all new/changed handlers
- ✅ `logger.Dev` used during implementation (replaced before merge)
- ✅ No `logger.Dev` calls remain — all replaced with `logger.FromCtx` at correct level
- ✅ `traceutil.StartLite` at service function entry
- ✅ Audit prefix registered for new paths
- ✅ `errors.go` / `types.go` in service package

---

## 13. Key Principles

1. `rawBody` is always `map[string]any` — never binary
2. `fieldMappings` is always present as array — never absent
3. Location (`lat`/`lng`) comes from `fieldMappings` — not stored flat on event
4. `statusName` (string) replaces `status` (bool)
5. Approval requires all 5 required targets mapped and resolvable
6. Template stores snapshot — changing template does not affect approved events
7. Router is pure routing — no infra, no logic
8. Start logs at TRACE during development — review and promote before merge to main
