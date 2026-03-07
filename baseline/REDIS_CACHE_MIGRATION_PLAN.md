# REDIS Cache Migration Plan

> Scope: migrate Redis keys from device-approval model to source-family/template-match model
> Last updated: 2026-03-08

## 1. Why this migration is required

Current Redis keys reflect the old ingest concept:

- `evt:device:approved:{tenantId}:device:{deviceId}`
- `evt:status:{tenantId}:{eventId}`

That model assumes:

- approval is tied to a device
- reuse is tied to a device
- event review is per event/device

That is no longer correct.

The new ingest flow is based on:

- `sourceFamily`
- template matching inside family
- review by unresolved payload shape/fingerprint
- device enrichment via `deviceManagement`

So Redis must move from **device approval cache** to **template match cache + pending review cache + profile/enrichment cache**.

---

## 2. Current keys to deprecate

### 2.1 Remove device approval cache

Deprecated:

```text
evt:device:approved:{tenantId}:device:{deviceId}
```

Why it is wrong:

- binds approval to device instead of source contract
- cannot reuse templates across devices
- breaks AIBOX / AILPR multi-device onboarding
- causes repeated review for same payload shape on new devices

Action:

- stop writing new values immediately
- delete old keys gradually or let TTL expire

---

### 2.2 Re-evaluate event status cache

Current:

```text
evt:status:{tenantId}:{eventId}
```

This may still be useful only for technical processing state, such as:

- received
- rawPublished
- normalized
- delivered
- failed

But it must **not** remain the business approval source of truth.

Action:

- keep only if used for dedupe / observability / pipeline state
- otherwise deprecate

---

## 3. New Redis cache model

## 3.1 Template match cache

Purpose:

- remember which template matched a payload shape
- allow reuse across devices
- avoid repeated DB/template-engine work

Key:

```text
tmpl:match:{tenantId}:{orgId}:{sourceFamily}:{fingerprint}
```

Example:

```text
tmpl:match:aisom:8c6226f3-411d-412a-8a8a-36c1259076bb:aibox:5e52febf
```

Value example:

```json
{
  "templateId": "fe42ba58-27fa-4a5c-aec5-0d30a7ce781c",
  "finalEventType": "vehicleCapture",
  "sourceFamily": "aibox",
  "fingerprint": "5e52febf",
  "matchedAt": "2026-03-08T10:00:00Z"
}
```

Recommended TTL:

- `6h` to `24h`

Use case:

- DEV-001 creates/reuses template match
- DEV-002 with same shape should hit this cache and skip review

---

## 3.2 Pending template review lock

Purpose:

- avoid duplicate review records for the same unresolved payload shape
- replace `DEVICE_PENDING_LOCKED`

Key:

```text
tmpl:review:pending:{tenantId}:{orgId}:{sourceFamily}:{fingerprint}
```

Example:

```text
tmpl:review:pending:aisom:8c6226f3-411d-412a-8a8a-36c1259076bb:aibox:5e52febf
```

Value example:

```json
{
  "reviewId": "8db433d1-2ef4-4fd9-9f6d-b0bb6a0f65b2",
  "createdAt": "2026-03-08T10:05:00Z"
}
```

Recommended TTL:

- `1h` to `24h`

Notes:

- lock is by unresolved shape, not by device
- same unresolved shape should not create many review documents

---

## 3.3 Source profile cache

Purpose:

- cache `sourceProfile` lookup for hot ingest path
- avoid repeated Mongo/DB reads on every event

Key:

```text
source:profile:{sourceFamily}
```

Example:

```text
source:profile:aibox
```

Value example:

```json
{
  "sourceFamily": "aibox",
  "displayName": "AI Box",
  "refRules": {
    "primaryRefFields": ["raw.channelId"],
    "secondaryRefFields": ["raw.device", "raw.sn"]
  },
  "suggestedMatchFields": ["raw.type", "raw.typeValue", "raw.eventAttribute.listType"]
}
```

Recommended TTL:

- `1h` to `24h`

---

## 3.4 Device management / enrichment cache

Purpose:

- cache device/site/channel enrichment used to fill or override fields such as:
  - `deviceId`
  - `lat`
  - `lng`
  - `site`
  - `zone`

Key options:

### Option A — entity based

```text
device:mgmt:{tenantId}:{orgId}:{entityType}:{entityId}
```

Example:

```text
device:mgmt:aisom:8c6226f3-411d-412a-8a8a-36c1259076bb:channel:31
```

### Option B — sourceFamily + primaryRefKey

```text
device:mgmt:{tenantId}:{orgId}:{sourceFamily}:{primaryRefKey}
```

Value example:

```json
{
  "deviceId": "CAM-031",
  "lat": 13.7563,
  "lng": 100.5018,
  "site": "BANGPU",
  "zone": "Gate-A"
}
```

Recommended TTL:

- `10m` to `1h`

---

## 3.5 Optional processing state cache

Purpose:

- technical state only
- not business approval state

Key:

```text
evt:proc:{tenantId}:{eventId}
```

Value example:

```text
received
rawPublished
normalized
delivered
failed
```

Recommended TTL:

- `1h` to `24h`

Only keep this if it is actually used for:

- idempotency
- pipeline observability
- troubleshooting

---

## 4. Key naming conventions

Use lowercase, colon-delimited namespaces.

Recommended namespaces:

```text
tmpl:match
tmpl:review:pending
source:profile
device:mgmt
evt:proc
```

Do not introduce mixed concepts like:

```text
evt:device:approved
```

because that belongs to the old model.

---

## 5. TTL recommendations summary

| Key pattern | TTL | Reason |
|---|---:|---|
| `tmpl:match:*` | 6h–24h | hot-path match reuse |
| `tmpl:review:pending:*` | 1h–24h | prevent duplicate review creation |
| `source:profile:*` | 1h–24h | profile rarely changes |
| `device:mgmt:*` | 10m–1h | enrichment may change |
| `evt:proc:*` | 1h–24h | technical state only |

---

## 6. Cache invalidation strategy

This is the critical part. A cache plan without invalidation is fake architecture.

## 6.1 Invalidate on template create/update/delete

Affected keys:

```text
tmpl:match:{tenantId}:{orgId}:{sourceFamily}:*
```

Options:

### Option A — wildcard delete

Simple but potentially expensive.

### Option B — versioned key prefix (recommended)

Use a family/org version key.

Example:

```text
tmpl:match:v3:{tenantId}:{orgId}:{sourceFamily}:{fingerprint}
```

When template changes:

- bump version from `v3` to `v4`
- old keys become cold naturally

This is better than scanning large Redis keyspaces.

---

## 6.2 Invalidate on sourceProfile update

Affected keys:

```text
source:profile:{sourceFamily}
```

Action:

- delete exact key or bump profile version

---

## 6.3 Invalidate on deviceManagement update

Affected keys:

```text
device:mgmt:{tenantId}:{orgId}:*
```

Recommended:

- delete exact entity key if possible
- if many records affected, use version prefix

---

## 7. Versioned cache key strategy (recommended)

Version keys prevent expensive wildcard deletes.

## 7.1 Template match version

Supporting key:

```text
tmpl:match:version:{tenantId}:{orgId}:{sourceFamily}
```

Value:

```text
3
```

Final runtime key:

```text
tmpl:match:v3:{tenantId}:{orgId}:{sourceFamily}:{fingerprint}
```

When template changes:

- increment version key
- new events read/write under new namespace

---

## 7.2 Device management version

Supporting key:

```text
device:mgmt:version:{tenantId}:{orgId}
```

Final key:

```text
device:mgmt:v7:{tenantId}:{orgId}:{entityType}:{entityId}
```

---

## 8. Migration phases

## Phase 1 — Stop writing old device approval cache

Remove writes to:

```text
evt:device:approved:{tenantId}:device:{deviceId}
```

Do this first.

If you keep writing old keys while introducing new keys, the system will carry both mental models and become harder to debug.

---

## Phase 2 — Add template match cache

Implement:

```text
tmpl:match:{tenantId}:{orgId}:{sourceFamily}:{fingerprint}
```

Flow:

1. compute fingerprint
2. lookup Redis
3. hit -> use template directly
4. miss -> evaluate template engine
5. if matched -> cache result

---

## Phase 3 — Add pending review lock

Implement:

```text
tmpl:review:pending:{tenantId}:{orgId}:{sourceFamily}:{fingerprint}
```

Flow:

1. template match miss
2. check pending review lock
3. if exists -> do not create duplicate review
4. if not exists -> create review + set lock

---

## Phase 4 — Add source profile cache

Implement:

```text
source:profile:{sourceFamily}
```

Flow:

1. route receives `sourceFamily`
2. load `sourceProfile` from Redis
3. fallback to DB if miss
4. cache result

---

## Phase 5 — Add device enrichment cache

Implement:

```text
device:mgmt:{tenantId}:{orgId}:{entityType}:{entityId}
```

Flow:

1. resolve primary ref
2. load enrichment data from cache
3. fallback to DB if miss
4. cache result

---

## Phase 6 — Remove old event status business usage

If `evt:status:*` is still used as approval state, remove that responsibility.

Keep only if needed as technical processing state.

---

## 9. Runtime flow with new Redis model

```text
POST /events/:orgId/:sourceFamily
   │
   ├─ load source profile from `source:profile:*`
   │
   ├─ resolve fingerprint
   │
   ├─ lookup `tmpl:match:*`
   │      │
   │      ├─ hit  -> use template
   │      └─ miss -> evaluate template engine
   │                    │
   │                    ├─ matched -> cache in `tmpl:match:*`
   │                    └─ unmatched -> check `tmpl:review:pending:*`
   │                                      │
   │                                      ├─ exists -> do not create duplicate review
   │                                      └─ not exists -> create review + set lock
   │
   ├─ resolve device/site enrichment from `device:mgmt:*`
   │
   └─ publish raw.events if template matched
```

---

## 10. Recommended implementation changes

## 10.1 Remove old cache functions

Deprecate functions like:

```go
SetDeviceEventTypeApproved(...)
GetDeviceEventTypeApproved(...)
```

Replace with:

```go
SetTemplateMatch(...)
GetTemplateMatch(...)
SetPendingTemplateReview(...)
GetPendingTemplateReview(...)
GetSourceProfile(...)
SetSourceProfile(...)
GetDeviceManagement(...)
SetDeviceManagement(...)
```

---

## 10.2 Suggested Go cache API

```go
type TemplateMatchCacheValue struct {
    TemplateId     string    `json:"templateId"`
    FinalEventType string    `json:"finalEventType,omitempty"`
    SourceFamily   string    `json:"sourceFamily"`
    Fingerprint    string    `json:"fingerprint"`
    MatchedAt      time.Time `json:"matchedAt"`
}

type PendingTemplateReviewCacheValue struct {
    ReviewId  string    `json:"reviewId"`
    CreatedAt time.Time `json:"createdAt"`
}

type DeviceManagementCacheValue struct {
    DeviceId string  `json:"deviceId,omitempty"`
    Lat      float64 `json:"lat,omitempty"`
    Lng      float64 `json:"lng,omitempty"`
    Site     string  `json:"site,omitempty"`
    Zone     string  `json:"zone,omitempty"`
}
```

---

## 11. Risks and warnings

## 11.1 Do not use Redis as source of truth

Redis is only cache / lock / accelerator.

Source of truth should remain:

- MongoDB for template, review, source profile, device management
- Kafka for event pipeline

---

## 11.2 Do not reintroduce device-based approval logic in cache

Even if it feels convenient, it will recreate the same architectural problem.

If you need to reason about matching, reason by:

- `sourceFamily`
- `fingerprint`
- `templateId`

not by `deviceId`.

---

## 11.3 Wildcard invalidation can hurt Redis at scale

If traffic grows, scanning and deleting large keyspaces will become expensive.

Use versioned cache namespaces where possible.

---

## 12. Acceptance criteria

- [ ] no new writes to `evt:device:approved:*`
- [ ] `tmpl:match:*` implemented and used on ingest hot path
- [ ] `tmpl:review:pending:*` prevents duplicate review creation
- [ ] `source:profile:*` implemented
- [ ] `device:mgmt:*` implemented for enrichment lookup
- [ ] `evt:status:*` no longer used as business approval source of truth
- [ ] template update invalidates or versions related match cache
- [ ] sourceProfile update invalidates related cache
- [ ] deviceManagement update invalidates related cache

---

## 13. Final migration summary

Old mental model:

```text
device approved -> event allowed
```

New mental model:

```text
sourceFamily + fingerprint -> template match
no match -> pending template review
resolved ref -> deviceManagement enrichment
```

That is the actual migration.
It is not a rename. It is a change in architecture.
