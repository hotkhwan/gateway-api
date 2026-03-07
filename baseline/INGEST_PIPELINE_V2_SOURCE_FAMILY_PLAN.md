# Ingest Pipeline V2 — Source Family Architecture Plan

> Extension of: Ingest Pipeline — Implementation Plan
> Last updated: 2026-03-08

---

## 1. Purpose

Current pipeline already implemented:

```text
Device
 → POST /events/:orgId
 → approval / template match
 → Kafka raw.events
 → Normalizer
 → Mongo event_details
 → Kafka normalized.events
 → delivery
```

This plan introduces **Source Family architecture** to solve these root problems:

1. Device-based approval duplication
2. Vendor payload differences inside the same broad source family
3. Multi-reference sources such as AIBOX / AILPR
4. Template reuse across devices
5. Contract-based ingest, not device-based ingest

Current issue example:

- `DEV-001` gets reviewed and approved
- `DEV-002` sends the same payload shape later
- system still creates pending review again

Root cause:

Template matching is tied too much to **device/event instance thinking** instead of **source family + payload contract**.

---

## 2. New Core Concept

### 2.1 sourceFamily

Every ingest request belongs to a **source family**.

Examples:

| sourceFamily | description |
|---|---|
| `aibox` | AI gateway / multi-channel edge AI family |
| `ailpr` | AI LPR gateway family |
| `xconnector` | site / integration connector family |
| `genetec` | Genetec connector family |
| `hikvision` | Hikvision event API family |
| `dahua` | Dahua event API family |

### 2.2 Route change

Old:

```text
POST /events/:orgId
```

New:

```text
POST /events/:orgId/:sourceFamily
```

Examples:

```text
/events/{orgId}/aibox
/events/{orgId}/ailpr
/events/{orgId}/xconnector
/events/{orgId}/genetec
```

### 2.3 Naming decision

Use **`sourceFamily`** instead of `eventType` in the route.

Why:

- `eventType` gets confused with final canonical business type
- `sourceFamily` clearly means connector / contract family
- it matches the real intent: producer chooses which family this payload belongs to
- route stays dynamic and controlled by us

---

## 3. Final Flow (V2)

```text
Device / Gateway
   │
   ▼
POST /events/:orgId/:sourceFamily
   │
   ▼
Load SourceProfile / SourceContract
   │
   ▼
Resolve references (device / channel / site / gateway)
   │
   ▼
Apply deviceManagement override (lat / lng / refs if configured)
   │
   ▼
Template matching inside sourceFamily
   │
   ├─ matched
   │      │
   │      ▼
   │   build CanonicalEvent → publish raw.events
   │
   └─ unmatched
          │
          ▼
      create templateReview sample
      optional drop / ack
```

Then existing downstream pipeline remains:

```text
Kafka raw.events
     │
     ▼
Normalizer Consumer
     │
     ▼
Mongo event_details
     │
     ▼
Kafka normalized.events
     │
     ▼
Delivery Consumer
```

---

## 4. Architectural Separation

### 4.1 sourceFamily

Used to identify which contract family the payload belongs to.

Examples:

- `aibox`
- `ailpr`
- `xconnector`
- `genetec`

### 4.2 Template Matching

Used to decide **which template inside that family** should process the event.

### 4.3 deviceManagement

Used to override or enrich physical/logical source identity, for example:

- override `lat`
- override `lng`
- override `deviceId`
- derive missing site / channel / gateway mapping

### 4.4 finalEventType

Used only after template match / normalization.

Examples:

- `vehicleCapture`
- `blacklistHit`
- `waterLevelNotification`
- `intrusionAlarm`

Do **not** use `finalEventType` as route identity.

---

## 5. Important Answer to the Design Question

Yes — if template matching is prepared ahead of time, then:

1. request enters with `sourceFamily`
2. system filters templates by that `sourceFamily`
3. if a matching template exists, it should go straight through
4. no event-level manual approval is needed
5. `deviceManagement` becomes the correct place for lat/lng or identity overwrite

So the correct split is:

- `sourceFamily` = route-level family selector
- `templateMatching` = selects normalization logic
- `deviceManagement` = manages override/enrichment of refs and geo
- `templateReview` = only for new/unmatched payloads

That is much cleaner than pushing all of this into `event_management`.

---

## 6. SourceProfile / SourceContract (NEW)

Collection:

```text
source_profiles
```

Purpose:

- define source family behavior
- define reference extraction hints
- define suggested matching fields
- define whether family is likely single-ref or multi-ref

### Example: AIBOX

```json
{
  "sourceFamily": "aibox",
  "displayName": "AI Box",
  "multiRef": true,
  "refRules": {
    "primaryRefFields": ["raw.channelId"],
    "secondaryRefFields": ["raw.device", "raw.sn"],
    "siteFields": ["raw.zone", "raw.address"]
  },
  "suggestedMatchFields": [
    "raw.type",
    "raw.typeValue",
    "raw.eventAttribute.listType"
  ]
}
```

### Example: AILPR

```json
{
  "sourceFamily": "ailpr",
  "displayName": "AI LPR",
  "multiRef": true,
  "refRules": {
    "primaryRefFields": ["raw.channelId", "raw.cameraId"],
    "secondaryRefFields": ["raw.device", "raw.sn"]
  },
  "suggestedMatchFields": [
    "raw.type",
    "raw.typeValue",
    "raw.eventAttribute.listType"
  ]
}
```

### Example: XConnector

```json
{
  "sourceFamily": "xconnector",
  "displayName": "XConnector",
  "multiRef": false,
  "refRules": {
    "primaryRefFields": ["ref_id"],
    "siteFields": ["site_name"]
  },
  "suggestedMatchFields": [
    "event_type",
    "event_name",
    "from_device"
  ]
}
```

### Example: Genetec

```json
{
  "sourceFamily": "genetec",
  "displayName": "Genetec",
  "multiRef": false,
  "refRules": {
    "primaryRefFields": ["ref_id"],
    "siteFields": ["site_name"]
  },
  "suggestedMatchFields": [
    "event_type",
    "event_name",
    "alarm_triggered_by"
  ]
}
```

---

## 7. Multi-Reference Support

AIBOX / AILPR must be treated as first-class multi-reference families.

### Normalized reference model

```json
{
  "primaryRef": {
    "entityType": "channel",
    "entityId": "31"
  },
  "resolvedRefs": [
    {
      "role": "origin",
      "entityType": "channel",
      "entityId": "31"
    },
    {
      "role": "gateway",
      "entityType": "device",
      "entityId": "EDGEAI-8ch"
    },
    {
      "role": "serial",
      "entityType": "sourceSerial",
      "entityId": "60038008431d6040"
    }
  ]
}
```

### Family classification

Single-ref families:

| sourceFamily | primary ref |
|---|---|
| `xconnector` | `ref_id` |
| `genetec` | `ref_id` |

Multi-ref families:

| sourceFamily | refs |
|---|---|
| `aibox` | channel + gateway + serial |
| `ailpr` | channel + gateway + serial |

---

## 8. deviceManagement (NEW / promoted concept)

This is where physical/logical source override should happen.

It should own rules like:

- overwrite `lat`
- overwrite `lng`
- overwrite `deviceId`
- derive channel mapping
- map `site_name + ref_id` to internal device
- fill missing device metadata when raw payload is incomplete

### Why this should not stay in event_management

Because these are reusable source resolution rules, not one-off event review decisions.

### Suggested collection

```text
device_management
```

### Example use cases

1. raw payload has no lat/lng → use configured device geo
2. raw payload has `ref_id` only → resolve to internal device/channel
3. raw payload has external `channelId` → map to internal camera/device

---

## 9. MappingTemplate V2 Changes

Add `sourceFamily` into template identity.

### Before

```json
{
  "match": {
    "deviceType": "AI_BOX"
  }
}
```

### After

```json
{
  "sourceFamily": "aibox",
  "matchAll": [
    {
      "field": "raw.type",
      "operator": "eq",
      "values": ["Motor Vehicle Capture"]
    }
  ],
  "finalEventType": "vehicleCapture"
}
```

### Matching rules

Template lookup algorithm:

```text
1. filter templates by orgId + sourceFamily
2. evaluate matchAll / matchAny
3. choose highest priority
4. if one clear match → use it
5. if none match → create templateReview
6. if ambiguous → create templateReview
```

---

## 10. templateReview (replace event_management responsibility)

Current `event_management` concept is too overloaded.

New role:

- keep only unresolved template review samples
- no more device-based approval queue
- no more repeated approval per device instance

### Suggested collection

```text
template_reviews
```

### Example document

```json
{
  "reviewId": "uuid",
  "orgId": "8c6226f3-411d-412a-8a8a-36c1259076bb",
  "sourceFamily": "aibox",
  "fingerprint": "abc123",
  "samplePayload": {"raw": {"type": "Motor Vehicle Capture"}},
  "suggestedMatchFields": ["raw.type", "raw.typeValue"],
  "status": "pending",
  "createdAt": "2026-03-08T10:00:00Z"
}
```

### Operator flow

1. inspect review sample
2. create new template for that family
3. archive or delete review item

If unmatched events should not be stored long-term, they can be sampled and dropped after review creation.

---

## 11. Remove Device-Level Approval Lock

Remove these concepts from ingest hot path:

- `DEVICE_PENDING_LOCKED`
- `SetDeviceEventTypeApproved`
- event approval tied to device instance

Replace with:

```text
template match by sourceFamily + rule
```

### New behavior

| condition | action |
|---|---|
| template found | publish `raw.events` immediately |
| template not found | create `templateReview` |
| template unresolved | optional drop / ack / sample only |

This removes repeated manual review for same source contract across different devices.

---

## 12. API Changes

### 12.1 Ingest Route

Old:

```text
POST /events/:orgId
```

New:

```text
POST /events/:orgId/:sourceFamily
```

### 12.2 SourceProfile APIs

```text
GET  /api/v1/ingest/sourceProfiles
GET  /api/v1/ingest/sourceProfiles/:sourceFamily
POST /api/v1/ingest/sourceProfiles
PUT  /api/v1/ingest/sourceProfiles/:sourceFamily
```

### 12.3 TemplateReview APIs

```text
GET  /api/v1/ingest/templateReviews
GET  /api/v1/ingest/templateReviews/:id
POST /api/v1/ingest/templateReviews/:id/createTemplate
POST /api/v1/ingest/templateReviews/:id/archive
```

### 12.4 deviceManagement APIs

```text
GET  /api/v1/deviceManagement
POST /api/v1/deviceManagement
PUT  /api/v1/deviceManagement/:id
GET  /api/v1/deviceManagement/:id
```

---

## 13. Canonical responsibilities after the redesign

### sourceFamily

- selects source family / source contract family
- dynamic route key
- provided by producer

### sourceProfile

- defines extraction / ref hints / suggested match fields
- defines multi-ref vs single-ref expectation

### deviceManagement

- override or enrich source identity and geo
- not responsible for template matching

### mappingTemplate

- normalize event inside one source family
- produce finalEventType and normalized payload

### templateReview

- unresolved sample queue only
- no longer a general approval inbox

---

## 14. Implementation Plan

### Phase 1 — Route and request context

Files:

- `router/ingest.go`
- `controllers/ingestapi/...`
- `internal/services/ingestsvc/ingest.go`

Tasks:

- change route to `POST /events/:orgId/:sourceFamily`
- add `sourceFamily` to ingest context
- validate sourceFamily is present

---

### Phase 2 — SourceProfile support

New files:

- `models/ingestmod/sourceProfile.go`
- `internal/repo/sourceprofilerepo/repo.go`
- `internal/services/sourceprofilesvc/service.go`
- `controllers/ingestapi/sourceProfile.go`
- `router/sourceProfile.go`

Tasks:

- create CRUD for source profiles
- load source profile by `sourceFamily`
- use source profile hints during ingest

---

### Phase 3 — deviceManagement resolution layer

New files:

- `internal/services/ingestsvc/sourceResolver.go`
- `internal/repo/devicemanagementrepo/...`
- `models/devicemod/...` or `models/ingestmod/deviceManagement.go`

Tasks:

- resolve `primaryRef`
- resolve `resolvedRefs`
- apply `lat/lng/deviceId` override
- support single-ref and multi-ref families

---

### Phase 4 — Template matcher V2

Files:

- `models/ingestmod/mappingTemplate.go`
- `internal/services/mappingtemplatesvc/...`
- `internal/services/ingestsvc/templateMatch.go`

Tasks:

- add `sourceFamily`
- add `matchAll`, `matchAny`, `priority`
- filter templates by `orgId + sourceFamily`
- remove dependency on device-based approval cache

---

### Phase 5 — templateReview queue

New files:

- `models/ingestmod/templateReview.go`
- `internal/repo/templatereviewrepo/...`
- `internal/services/templatereviewsvc/...`
- `controllers/ingestapi/templateReview.go`
- `router/templateReview.go`

Tasks:

- create unresolved sample queue
- archive/delete after template creation
- support payload preview for operator

---

### Phase 6 — Remove old event_management approval path

Tasks:

- stop using `event_management` as approval queue for ingest templates
- keep only temporary compatibility if needed during migration
- remove device pending lock behavior

---

## 15. Migration Plan

1. add new ingest route with `:sourceFamily`
2. seed `source_profiles`
3. migrate existing templates to include `sourceFamily`
4. enable new template matching flow
5. move ref/geo override to `deviceManagement`
6. disable old device-based approval path
7. retire `event_management` from main ingest path

### Compatibility note

Downstream stays unchanged:

- `raw.events`
- Normalizer consumer
- `event_details`
- `normalized.events`
- delivery
- DLQ
- geo enrichment

Only ingest entry logic changes.

---

## 16. Expected Results

### Example 1

Template exists for:

- `sourceFamily = aibox`
- `raw.type = Motor Vehicle Capture`

Then:

- `DEV-001` enters → template matched
- `DEV-002` enters with same family and same matching shape → same template reused
- no review duplication

### Example 2

`xconnector` event:

```json
{
  "site_name": "BANGPU",
  "ref_id": 5823092,
  "event_type": "Water Level Notification"
}
```

Then:

- route = `/events/{orgId}/xconnector`
- source profile says single-ref = `ref_id`
- template match inside family by `event_type`
- normalize to `finalEventType = waterLevelNotification`

---

## 17. Acceptance Criteria

- [ ] route `/events/:orgId/:sourceFamily` works
- [ ] source profiles load by `sourceFamily`
- [ ] templates are filtered by `sourceFamily`
- [ ] `deviceManagement` can override `lat`, `lng`, `deviceId`
- [ ] AIBOX / AILPR multi-ref resolves correctly
- [ ] unmatched payload creates `templateReview`
- [ ] same family + same payload shape reuses existing template across devices
- [ ] old `DEVICE_PENDING_LOCKED` logic is removed from hot path
- [ ] `event_management` no longer blocks ingest

---

## 18. Final Decision Summary

Lock these terms in the system:

- route param: `sourceFamily`
- source config: `sourceProfile`
- source override layer: `deviceManagement`
- unresolved queue: `templateReview`
- normalized business type: `finalEventType`

Do not use:

- route param = `eventType`
- `sourceContact`
- event-level device approval as the primary ingest gate

This keeps the architecture clean, scalable, and reusable across AIBOX, AILPR, XConnector, Genetec, and future families.
