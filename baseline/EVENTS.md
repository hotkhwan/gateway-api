# EVENTS.md

# Ingest Event Management Refactor Plan (Gateway API)
Please referance baseline/README.md of project structure
This document defines the **refactor plan, architecture rules, and
workflow** for the **Ingest Event Management system**.

Goal:

-   Support **scalable field mapping**
-   Allow **mapping templates**
-   Enable **bulk operations**
-   Enforce **approval gate**
-   Align with **baseline architecture**

------------------------------------------------------------------------

# 0. createdAt vs occurredAt

These fields are **not the same**.

  Field        Meaning
  ------------ ------------------------------------------------
  createdAt    When our system created the record
  occurredAt   When the event actually happened on the device

Example:

    Device captured image at 10:00
    Gateway received event at 10:00:02

    occurredAt = 10:00
    createdAt  = 10:00:02

Rules:

-   `createdAt` always exists
-   `occurredAt` may be derived from rawBody via mapping
-   All timestamps must be **RFC3339 UTC**

------------------------------------------------------------------------

# 1. Baseline Architecture Rules

Architecture flow must always be:

    repo → service → controller → router

Responsibilities:

  Layer        Responsibility
  ------------ ---------------------
  repo         database access
  service      business logic
  controller   HTTP layer
  router       routes + middleware

Router **must not**:

-   create repositories
-   call EnsureIndexes
-   perform business logic

------------------------------------------------------------------------

# 2. Event Pipeline

    external device
    → POST /events/:orgId
    → event_management (pending)

    operator mapping

    → approve event
    → Kafka raw.events

    → normalizer
    → canonical_events

    → S3 binary storage
    → delivery workers
    → webhook / retry / DLQ

------------------------------------------------------------------------

# 3. Problem: Field Mapping Does Not Scale

If each device must be mapped manually:

    1000 devices = 1000 mappings

This is not feasible.

Solution:

    Mapping Template System

Templates allow reuse of mappings across many events.

------------------------------------------------------------------------

# 4. Mapping Template Concept

A **mappingTemplate** defines reusable mappings.

Pending events store a **snapshot copy** of the template.

Why snapshot?

Because templates may change later and we must preserve the mapping used
at approval time.

Example template:

``` json
{
  "templateId": "tmpl_face_ata",
  "match": {
    "vendor": "ATA",
    "deviceType": "camera",
    "eventType": "face.detected"
  },
  "mappings": [
    { "targetPath": "source.deviceId", "sourcePath": "rawBody.cameraId" },
    { "targetPath": "occurredAt", "sourcePath": "rawBody.timestamp" },
    { "targetPath": "location.lat", "sourcePath": "rawBody.lat" },
    { "targetPath": "location.lng", "sourcePath": "rawBody.lng" }
  ]
}
```

------------------------------------------------------------------------

# 5. Two Supported Template Workflows

## Flow A --- Event First

1.  Event arrives
2.  Operator opens event detail
3.  Operator maps fields
4.  Operator clicks **Save as Template**
5.  Future events auto‑match template

## Flow B --- Template First

1.  Admin creates template
2.  Event arrives
3.  System auto‑matches template

------------------------------------------------------------------------

# 6. Template Auto Matching

Matching uses **fingerprint**.

Fingerprint fields:

    vendor
    protocol
    deviceType
    subType
    eventType
    rawSchemaVersion
    rawBodyKeyHash

Example fingerprint:

    ATA|camera|edgeai|face.detected|hash_abc123

Matching logic:

    1 template match → auto bind
    0 matches → manual selection
    >1 matches → manual selection

------------------------------------------------------------------------

# 7. Router Refactor

Before:

    router/
      fieldmapping.go
      dlq.go
      ingest.go

After:

    router/
      ingest.go

Routes from:

    router/fieldmapping.go
    router/dlq.go

must be merged into:

    router/ingest.go

Router must remain **pure routing**.

------------------------------------------------------------------------

# 8. Final Route Structure

Base group:

    /api/v1/ingest

Subgroups:

    /management
    /mappingTemplates
    /dlq

------------------------------------------------------------------------

# 9. Management Endpoints

List pending events:

    GET /api/v1/ingest/management

Get event:

    GET /api/v1/ingest/management/:eventId

Update event metadata:

    PATCH /api/v1/ingest/management/:eventId

Update field mappings:

    PATCH /api/v1/ingest/management/:eventId/fieldMappings

Approve event:

    POST /api/v1/ingest/management/:eventId/approve

Reject event:

    POST /api/v1/ingest/management/:eventId/reject

Delete event:

    DELETE /api/v1/ingest/management/:eventId

------------------------------------------------------------------------

# 10. Bulk Operations

Required for large fleets.

Bulk apply template:

    POST /api/v1/ingest/management/bulk/applyTemplate

Body:

``` json
{
  "eventIds": ["id1","id2","id3"],
  "templateId": "tmpl_face_ata"
}
```

Bulk approve:

    POST /api/v1/ingest/management/bulk/approve

Bulk reject:

    POST /api/v1/ingest/management/bulk/reject

Bulk delete:

    POST /api/v1/ingest/management/bulk/delete

Bulk operations must support **partial success**.

------------------------------------------------------------------------

# 11. Mapping Template Endpoints

List templates:

    GET /api/v1/ingest/mappingTemplates

Get template:

    GET /api/v1/ingest/mappingTemplates/:templateId

Create template:

    POST /api/v1/ingest/mappingTemplates

Update template:

    PATCH /api/v1/ingest/mappingTemplates/:templateId

Delete template:

    DELETE /api/v1/ingest/mappingTemplates/:templateId

------------------------------------------------------------------------

# 12. Approval Gate Rules

Before approving an event the system must validate required mappings.

Required fields:

    source.deviceId
    occurredAt
    eventType
    location.lat
    location.lng

If validation fails:

    HTTP 409 or 422

Example response:

``` json
{
  "code": "MAPPING_REQUIRED",
  "status": false,
  "details": {
    "missingTargets": ["source.deviceId"],
    "invalidTargets": ["location.lat"]
  }
}
```

------------------------------------------------------------------------

# 13. DLQ Routes

DLQ endpoints are grouped under ingest.

    GET  /api/v1/ingest/dlq
    GET  /api/v1/ingest/dlq/stats
    POST /api/v1/ingest/dlq/retry
    POST /api/v1/ingest/dlq/replay
    GET  /api/v1/ingest/dlq/:id

------------------------------------------------------------------------

# 14. PR Implementation Plan

## PR1 --- Router merge

-   move routes from
    -   router/fieldmapping.go
    -   router/dlq.go

→ router/ingest.go

## PR2 --- Mapping Template system

-   add mappingTemplates collection
-   add template endpoints
-   add fingerprint matching

## PR3 --- Bulk operations

-   bulk applyTemplate
-   bulk approve
-   bulk reject
-   bulk delete

## PR4 --- Approval gate

-   enforce mapping validation
-   publish to Kafka raw.events

## PR5 --- Docs + Postman

-   update documentation
-   update Postman collections

------------------------------------------------------------------------

# 15. Key Principles

1.  Mapping templates must be reusable
2.  Pending events store mapping snapshot
3.  Approval requires validated mapping
4.  Bulk operations must be supported
5.  Router must remain infrastructure‑free

------------------------------------------------------------------------

---

# 16. Model Refactor (Remove eventmod → Use ingestmod)

เพื่อให้ domain ชัดเจนและสอดคล้องกับ ingest pipeline

**ให้ยกเลิก package**


models/eventmod


และรวม model ทั้งหมดไปไว้ที่


models/ingestmod


เหตุผล:

- event lifecycle ทั้งหมดอยู่ใน ingest pipeline
- ลดการกระจาย domain model
- ลด circular import risk
- ทำให้ service layer ชัดเจนขึ้น

---

## 16.1 โครงสร้างใหม่ของ Models

ก่อน refactor:


models/
eventmod/
pending.go
fieldmapping.go
normalization.go
dlq.go

ingestmod/
dashboard.go


หลัง refactor:


models/
ingestmod/
pending.go
fieldmapping.go
mappingTemplate.go
normalization.go
dlq.go
dashboard.go


---

## 16.2 Model Responsibilities

### pending.go

pending event ใน collection:


event_management


example:

```go
type PendingEvent struct {
    EventId string `json:"eventId" bson:"eventId"`

    OrgId string `json:"orgId" bson:"orgId"`

    RawBody map[string]any `json:"rawBody" bson:"rawBody"`

    FieldMappings []FieldMapping `json:"fieldMappings"`

    TemplateId *string `json:"templateId,omitempty"`

    StatusName string `json:"statusName"`

    CreatedAt time.Time `json:"createdAt"`
    UpdatedAt time.Time `json:"updatedAt"`
}
fieldmapping.go

snapshot mapping stored on event

type FieldMapping struct {
    TargetPath string `json:"targetPath"`
    SourcePath string `json:"sourcePath"`

    Transform string `json:"transform"`

    Required bool `json:"required"`

    Confidence float64 `json:"confidence"`

    UpdatedAt time.Time `json:"updatedAt"`
}
mappingTemplate.go

reusable mapping template

type MappingTemplate struct {
    TemplateId string `json:"templateId"`

    OrgId string `json:"orgId"`

    Name string `json:"name"`

    Match MatchRule `json:"match"`

    Mappings []FieldMapping `json:"mappings"`

    CreatedAt time.Time `json:"createdAt"`
    UpdatedAt time.Time `json:"updatedAt"`
}
normalization.go

canonical event model

used by normalizer

type CanonicalEvent struct {
    EventId string `json:"eventId"`

    EventType string `json:"eventType"`

    OccurredAt time.Time `json:"occurredAt"`

    Source SourceInfo `json:"source"`

    Location LocationInfo `json:"location"`

    Payload map[string]any `json:"payload"`

    CreatedAt time.Time `json:"createdAt"`
}
dlq.go

dead letter queue model

type DLQMessage struct {
    MessageId string `json:"messageId"`

    EventId string `json:"eventId"`

    Reason string `json:"reason"`

    RetryCount int `json:"retryCount"`

    CreatedAt time.Time `json:"createdAt"`
}
16.3 Import Changes Required

หลัง refactor ต้องเปลี่ยน import ทุกที่จาก

models/eventmod

เป็น

models/ingestmod

ตัวอย่าง

ก่อน:

import "gateway-api/models/eventmod"

หลัง:

import "gateway-api/models/ingestmod"
16.4 Service Layer Impact

services ที่ต้อง update import:

internal/services/fieldsvc
internal/services/normalizesvc
internal/services/dlqsvc
internal/services/routingsvc
internal/services/ingeststatsvc

ทั้งหมดต้องใช้

models/ingestmod
16.5 Repo Layer Impact

repo ที่ใช้ event models:

internal/repo/eventrepo
internal/repo/fieldmaprepo
internal/repo/dlqrepo

ต้องเปลี่ยน import model เป็น

models/ingestmod
16.6 Benefits

หลัง refactor:

✔ domain model ชัด
✔ ลด package fragmentation
✔ service import สั้นลง
✔ ingestion pipeline เข้าใจง่ายขึ้น
✔ ลด circular dependency risk


---

# ⭐ คำแนะนำจากมุม Architecture

สิ่งที่คุณกำลังทำตอนนี้ **ถูกทิศแล้ว**  
เพราะ ingest pipeline ของคุณจริง ๆ คือ domain เดียว:


ingest
├ pending events
├ fieldMappings
├ mappingTemplates
├ normalization
├ canonical events
├ DLQ

END OF DOCUMENT
