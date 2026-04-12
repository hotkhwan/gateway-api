# Event Flow: AIBOX/PVS → gateway-api → klynx — Combined Implementation Plan

**Date:** 2026-04-11  
**Repos:** `github.com/hotkhwan/gateway-api` (gw), `github.com/pointitconsulting/klynx-api` (klynx)  
**Supersedes:** `event-flow-klynx-integration.md` (gw), `event-flow-grpc-query.md` (klynx)

---

## 1. Full Architecture

```
AIBOX / PVS device
    │
    │  POST /events/{orgId}/{sourceFamily}   (no JWT, rate-limited)
    ▼
┌─────────────────────────────────────────────────────────────┐
│                      gateway-api (gw)                       │
│                                                             │
│  IngestController                                           │
│  ├─ check Redis "realtime_targets:{workspaceId}"  ← gw only │
│  │    ├─ HIT + MatchFields match:                           │
│  │    │    alertdispatcher.Extract() → provisional fields   │
│  │    │    MQTT ──▶ events/ws/{ws}/alert/{sf}              │──▶ Path A (~0ms, provisional)
│  │    └─ MISS: skip                                         │
│  └─ ingestsvc.Ingest() ──▶ [raw.events]                    │
│                                                             │
│  normalizedcons (consume raw.events)                        │
│  ├─ template apply + geo enrichment + binary → S3          │
│  ├─ upsert event_details (gw MongoDB)                       │
│  ├─ MQTT ──▶ events/ws/{ws}/events/{sf}                    │──▶ Path B (canonical notify)
│  ├─ delivery targets mode=normalize → webhook dispatch      │──▶ Path C (webhook)
│  └─ delivery targets mode=klynx → EventBridgePublisher     │
│       └─▶ [gw.events.normalized.v1]                        │──▶ Path D (Kafka, appliance)
│                                                             │
│  assetsvc CRUD / normalizedcons asset delta:               │
│  └─▶ publish gw.assets.changed.v1                         │
│  sourcesvc CRUD:                                           │
│  └─▶ publish gw.sources.changed.v1                        │
│  deliverycons after dispatch attempt:                      │
│  └─▶ publish gw.delivery.status.v1                        │
└─────────────────────────────────────────────────────────────┘
                    │ Path D (Kafka)   │ gw.assets/sources/delivery topics
                    ▼                  ▼
┌─────────────────────────────────────────────────────────────┐
│                        klynx-api                            │
│                                                             │
│  gweventscons    → ingestFacade → upsert event_refs         │
│  gwassetcons     → assetSvc.SyncFromGW                      │
│  gwsourcecons    → orgSvc.UpdateSourceConfig                │
│  gwdeliverycons  → update event_refs.deliveryStatus         │
│                                                             │
│  klynx API: GET /events?orgId=x&from=x&to=x                │
│      ├─ query event_refs → eventIds                         │
│      ├─ BatchGetEvents(eventIds[], workspaceId) ──gRPC──▶   │
│      └─ merge + return                                      │
└─────────────────────────────────────────────────────────────┘
```

### Kafka Topics

| Topic | Producer | Consumer | Profile | หน้าที่ |
|-------|----------|----------|---------|--------|
| `raw.events` | gw (ingestsvc) | gw (normalizedcons) | all | raw event buffer |
| `normalized.events` | gw (normalizedcons) | gw (deliverycons) | all | webhook delivery pipeline |
| `gw.events.normalized.v1` | gw (EventBridge) | klynx (gweventscons) | appliance ✅ | normalized event handoff |
| `gw.assets.changed.v1` | gw (assetsvc / normalizedcons) | klynx (gwassetcons) | appliance | asset state change |
| `gw.sources.changed.v1` | gw (sourcesvc) | klynx (gwsourcecons) | appliance | source config change |
| `gw.delivery.status.v1` | gw (deliverycons) | klynx (gwdeliverycons) | appliance | delivery outcome → `event_refs.deliveryStatus` |
| `gw.workspace.provisioned.v1` | gw (workspacesvc) | klynx (workspaceprovcons) | appliance ✅ | workspace ack |
| `klynx.entitlement.snapshot.v1` | klynx (entitlementpub) | gw (entitlementcons) | appliance | ingest quota |
| `klynx.org.created.v1` | klynx (orgpub) | gw | appliance | trigger workspace creation |
| `klynx.org.deleted.v1` | klynx (orgpub) | gw | appliance | suspend workspace |

### MQTT Topics

| Topic | Path | Payload | เมื่อไหร่ |
|-------|------|---------|---------|
| `events/ws/{ws}/alert/{sf}` | A — provisional | `{ eventId, alertFields }` | IngestController (ก่อน Kafka) |
| `events/ws/{ws}/events/{sf}` | B — canonical | `{ eventId, workspaceId, canonical: true }` | normalizedcons (หลัง normalize) |

---

## 2. Shared Contracts

### 2.1 DeliveryTarget.mode

```go
type DeliveryTarget struct {
    // ...existing fields...
    Type        string         `bson:"type"          json:"type"`                  // "webhook" — required
    Mode        string         `bson:"mode"          json:"mode"`                  // see table below
    MatchFields map[string]any `bson:"matchFields,omitempty" json:"matchFields,omitempty"`
}
```

| mode | สร้างโดยใคร | ยิงจาก | เหมาะกับ |
|------|-----------|--------|---------|
| `normalize` (default) | workspace admin / API | normalizedcons | ทุก tier, webhook delivery |
| `realtime` | workspace admin / API | IngestController | appliance, MQTT alert clients |
| `klynx` | **klynx-api auto-register** | normalizedcons → EventBridge | **appliance / enterprise only** — ❌ not valid for saasPublic |

> **`mode: realtime` — klynx-api ไม่สร้างและไม่ manage targets ประเภทนี้**  
> `realtime` targets ถูกสร้างโดย workspace admin ผ่าน UI หรือ API — สำหรับ MQTT-subscribed frontends, dashboards, monitoring clients  
> klynx-api auto-registers เฉพาะ `mode: klynx` ตอน workspace provisioned เท่านั้น

> ⚠️ **`mode: klynx` — tier restriction (GAP 4)**  
> gw must reject `mode: klynx` requests from saasPublic workspaces with `400 Bad Request`:  
> ```json
> { "code": "INVALID_MODE", "message": "mode=klynx is not available for saasPublic workspaces" }
> ```  
> saasPublic uses webhook (mode=normalize) for event handoff — no Kafka EventBridge path.

**mode: klynx — auto-register payload (`type` required):**

```json
POST /workspaces/{workspaceId}/delivery-targets
{
  "name": "klynx-platform",
  "type": "webhook",
  "mode": "klynx"
}
```

> `type: "webhook"` ต้องส่งมาเสมอ — gw rejects requests without it.  
> `mode: klynx` targets ไม่มี `url` / ไม่มี HMAC — gw routes through Kafka EventBridge internally.

### 2.2 delivery-targets API Schema (GAP 3)

**Request:**
```json
POST /workspaces/{workspaceId}/delivery-targets
{
  "name":        "klynx-platform",          // required, unique per workspace
  "type":        "webhook",                 // required — only "webhook" supported in Phase 1-4
  "mode":        "klynx",                   // required: "normalize" | "realtime" | "klynx"
  "url":         "",                        // required for mode=normalize/realtime; omit for mode=klynx
  "matchFields": { "eventType": "motion" }  // optional filter
}
```

**Response 201:**
```json
{
  "code": "SUCCESS",
  "details": {
    "id":          "dt-xxx",
    "workspaceId": "ws-yyy",
    "name":        "klynx-platform",
    "type":        "webhook",
    "mode":        "klynx",
    "createdAt":   "2026-04-11T10:00:00Z"
  }
}
```

**Error cases:**
- `400` — missing `type`, invalid `mode`, or `url` absent when required for mode=normalize/realtime
- `404` — workspace not found
- `409` — target with same `name` already exists for this workspace (klynx should upsert on 409, not fail)

> **klynx idempotency:** `orgSvc.UpdateWorkspaceRef` should handle 409 by treating it as success (target already registered = already done).

### 2.3 Redis Cache — realtime targets (gw-internal only)

> **Scope: gw-only.** klynx does not read or write this cache.

```
Key:     realtime_targets:{workspaceId}
Value:   JSON array of DeliveryTarget (mode=realtime)
TTL:     none — invalidate on change

Warm:    create/update target mode=realtime → redis.Set(key, all realtime targets for workspace)
Evict:   delete target mode=realtime → redis.Del(key)
Startup: query all mode=realtime targets → warm per workspace
```

### 2.4 EventService Proto (gw gRPC server)

```protobuf
// proto/eventservice/v1/event.proto
syntax = "proto3";
package gw.eventservice.v1;

service EventService {
    rpc GetEvent(GetEventRequest) returns (NormalizedEventResponse);
    rpc BatchGetEvents(BatchGetEventsRequest) returns (BatchGetEventsResponse);
    rpc ListEvents(ListEventsRequest) returns (ListEventsResponse);
}

message GetEventRequest {
    string event_id     = 1;
    string workspace_id = 2;
}

message BatchGetEventsRequest {
    repeated string event_ids = 1;
    string workspace_id        = 2;
}

message BatchGetEventsResponse {
    repeated NormalizedEventResponse events = 1;
}
```

**RPC ownership:**

| RPC | klynx uses? | Notes |
|-----|------------|-------|
| `GetEvent` | ✅ yes | `GET /events/{eventId}` → single fetch |
| `BatchGetEvents` | ✅ yes | `GET /events` → event_refs → batch fetch |
| `ListEvents` | ❌ no | gw-internal use only (direct filter search without event_refs first) |

### 2.5 event_refs Schema (klynx MongoDB)

```go
// collection: event_refs
type EventRef struct {
    EventID        string    `bson:"eventId"`
    OrgID          string    `bson:"orgId"`
    WorkspaceID    string    `bson:"workspaceId"`
    DeviceID       string    `bson:"deviceId,omitempty"`
    EventType      string    `bson:"eventType"`
    SourceFamily   string    `bson:"sourceFamily,omitempty"`
    Score          float64   `bson:"score,omitempty"`
    OccurredAt     time.Time `bson:"occurredAt"`
    DeliveryStatus string    `bson:"deliveryStatus,omitempty"`
    CreatedAt      time.Time `bson:"createdAt"`
}
```

**Indexes:**
```go
{ "orgId": 1, "occurredAt": -1 }          // list by org, time-sorted
{ "orgId": 1, "eventId": 1 }  unique: true // single event lookup + dedup
{ "workspaceId": 1, "occurredAt": -1 }    // workspace-scoped list
```

### 2.6 EventGateway interface (klynx)

```go
// internal/gateways/gwgw/event.go
type EventGateway interface {
    GetEvent(ctx context.Context, workspaceId, eventId string) (*NormalizedEvent, error)
    BatchGetEvents(ctx context.Context, workspaceId string, eventIds []string) ([]*NormalizedEvent, error)
    // ListEvents not used by klynx — klynx queries event_refs first, then BatchGetEvents
}
```

### 2.7 NormalizedEvent Shared Contract (S-2)

> ⚠️ **Decision required before any field change**  
> Both repos carry separate copies of `NormalizedEvent`. Any field change on one side silently breaks the other.
>
> **Options:**
>
> | Option | Mechanism | Trade-off |
> |--------|-----------|----------|
> | A — shared Go module | `github.com/hotkhwan/event-contracts` imported by both | cleanest; requires a third repo to manage |
> | B — proto-first | `NormalizedEvent` in `event.proto`; codegen in both | aligns with gRPC work; adds proto dependency to Kafka path |
> | C — copy + CI guard | each repo keeps its copy; compatibility test in CI (`json.Unmarshal` round-trip) | zero infra; breaks loudly instead of silently |
>
> **Interim rule (until S-2 is decided):** no add/rename/remove of fields in `NormalizedEvent` on either side without an atomic coordinated update to both repos.

---

## 3. Deployment Tiers

| Tier | Event handoff | MQTT | Event query (klynx → gw) |
|------|--------------|------|--------------------------|
| **Appliance** | Kafka `gw.events.normalized.v1` | shared broker | gRPC internal (`gw-api:50051`, **no auth**) |
| **saasPublic** | HTTPS webhook `/gw/events` (HMAC) | — | **REST** `GET /ingest/details/{eventId}` (already exists) |
| **saasKlynx + saasPhibek** | Kafka MirrorMaker 2 | MQTT Bridge | gRPC internet TLS + **serviceToken** + Redis cache |

> **C1 resolved — saasPublic uses REST, not gRPC:**  
> saasPublic is an internet-boundary webhook profile. klynx on saasPublic calls gw's existing REST endpoint for full event data.  
> gRPC over internet (with serviceToken) is the **saasKlynx** tier. The two must not be conflated.

**gRPC auth decision per tier:**

| Tier | Auth | Rationale |
|------|------|-----------|
| Appliance | none (bind localhost / internal network only — port not exposed) | network-level isolation sufficient |
| saasPublic | n/a — REST instead | no shared gRPC surface |
| saasKlynx + saasPhibek | serviceToken per call (from `WorkspaceProvisionedEvent`) | internet-facing, per-workspace isolation |

**Env vars (separate sides of the same socket):**

| Repo | Env var | Value |
|------|---------|-------|
| gw (server) | `GRPC_PORT` | `50051` |
| klynx (client) | `GW_GRPC_URI` | appliance: `gw-api:50051` / saasKlynx: `gw-api.phibek.io:443` |

**klynx `DEPLOYMENT_PROFILE` values:**

```env
DEPLOYMENT_PROFILE=appliance    # Kafka direct + gRPC internal
DEPLOYMENT_PROFILE=saasPublic   # webhook only, REST event fetch
DEPLOYMENT_PROFILE=saasKlynx   # MirrorMaker 2 + gRPC TLS + serviceToken
```

---

## 4. Cross-repo Phase Dependencies (GAP 1)

```
gw Phase 1 (delivery-targets model + API)
    └─▶ KL Phase 5 (mode=klynx auto-register) — needs POST /workspaces/{wsId}/delivery-targets

gw Phase 2+3 (Redis cache + IngestController Path A)
    └─▶ independent (gw internal only)

gw Phase 4 (normalizedcons publish 3 topics)
    └─▶ KL Phase 6 (consumers + event_refs) — needs gw.assets, gw.sources, gw.delivery.status messages

gw Phase 7 (gRPC EventService)
    └─▶ KL Phase 8 (gRPC gateway + query API) — needs EventService running

gw Phase 9.1 (serviceToken in WorkspaceProvisionedEvent)
    └─▶ KL workspaceprovcons handler update — needs to save serviceToken field
    └─▶ KL Phase 9 (gRPC with token) — needs token stored before gRPC calls
```

**Parallel work safe:**
- gw Phase 2, 3, 4 + klynx Phase 6 — can proceed in parallel after gw Phase 1 done
- klynx event_refs schema (KL-3) can be created before gw topics are live

---

## 5. Implementation Steps

### Phase 1 — delivery/targets model + API (gw) 🔒 blocks KL Phase 5

- [ ] **1.1** Add `Mode string` + `MatchFields map[string]any` in `internal/models/deliverymod/`
- [ ] **1.2** Default migration: existing targets → `mode: "normalize"`
- [ ] **1.3** Repo CRUD: index + filter by mode; warm/evict Redis on create/update/delete
- [ ] **1.4** `POST /workspaces/{workspaceId}/delivery-targets`:
  - validate `type` required
  - accept `mode`, `matchFields`, `url` (required only for normalize/realtime)
  - return 409 on duplicate name (klynx must handle 409 as idempotent success)
- [ ] **1.5** Swagger doc update

### Phase 2 — Redis realtime cache (gw internal)

- [ ] **2.1** `internal/services/deliverysvc/realtimeCache.go` — `WarmRealtimeCache`, `InvalidateRealtimeCache`
- [ ] **2.2** Warm on create/update mode=realtime target
- [ ] **2.3** Evict on delete mode=realtime target
- [ ] **2.4** Startup warmup: query all mode=realtime → set per workspace

### Phase 3 — IngestController Path A (gw)

```go
targets, _ := redis.Get(ctx, "realtime_targets:"+workspaceId)
for _, t := range targets {
    if matchesFields(event, t.MatchFields) {
        alertFields := alertdispatcher.Extract(event)
        mqtt.Publish("events/ws/"+workspaceId+"/alert/"+sourceFamily,
            map[string]any{"eventId": eventId, "alertFields": alertFields})
    }
}
// always continue to ingestsvc.Ingest()
```

- [ ] **3.1** Redis check → MQTT provisional (Path A)
- [ ] **3.2** MISS: skip Path A, continue ingest

### Phase 4 — normalizedcons dispatch + 3 topic publishes (gw) 🔒 blocks KL Phase 6

- [ ] **4.1** Path B: MQTT `events/ws/{ws}/events/{sf}` → `{ eventId, workspaceId, canonical: true }`
- [ ] **4.2** Path C: mode=normalize webhook dispatch (MatchFields filter)
- [ ] **4.3** Path D: mode=klynx → `EventBridgePublisher.Publish` → `gw.events.normalized.v1`
- [ ] **4.4** `gw.assets.changed.v1` — **publish from:**
  - `assetsvc`: CRUD operations (camera created/updated/deleted/online/offline)
  - `normalizedcons`: event that changes asset last-seen state
  - Package: `kafka/assetseventpub/publisher.go` → `PublishAssetChanged(ctx, AssetChangedEvent)`
- [ ] **4.5** `gw.sources.changed.v1` — **publish from:**
  - `sourcesvc`: source config created/updated/deleted
  - Package: `kafka/sourceseventpub/publisher.go` → `PublishSourceChanged(ctx, SourceChangedEvent)`
- [ ] **4.6** `gw.delivery.status.v1` — **publish from:**
  - `deliverycons` / `deliveryOrchestrator`: after each dispatch attempt → outcome (delivered / retrying / dlq)
  - Package: `kafka/deliverystatuspub/publisher.go` → `PublishDeliveryStatus(ctx, DeliveryStatusEvent)`

### Phase 5 — mode=klynx auto-register (klynx) — requires Phase 1 done

> **Appliance / enterprise only** — klynx must NOT call this for saasPublic workspaces  
> (saasPublic has no Kafka EventBridge path; gw will return 400 for mode=klynx requests)

- [ ] **5.1** `internal/gateways/gwgw/deliveryTarget.go`
  ```go
  gwClient.CreateDeliveryTarget(ctx, workspaceId, DeliveryTargetRequest{
      Name: "klynx-platform", Type: "webhook", Mode: "klynx",
  })
  ```
- [ ] **5.2** Call in `orgSvc.UpdateWorkspaceRef` after workspace saved; guard on `DEPLOYMENT_PROFILE != saasPublic`; treat 409 as success
- [ ] **5.3** Env: `GW_API_URL`, `GW_API_TOKEN`

### Phase 6 — event_refs + 3 consumers (klynx) — requires Phase 4 done

> ⚠️ **GAP 3 — consumers are ready but producers are not**  
> klynx consumers for `gw.assets.changed.v1`, `gw.sources.changed.v1`, `gw.delivery.status.v1` are already implemented.  
> **Do not enable these consumers in production** until gw Phase 4 (GW-10, GW-11, GW-12) is deployed.  
> Enabling consumers before producers exist will leave `event_refs.deliveryStatus` empty and asset/source sync silently idle.

- [ ] **6.1** `internal/repo/eventrefsrepo/` — collection `event_refs` + all 3 indexes (§2.5)
- [ ] **6.2** `ingestsvc.HandleNormalized`: upsert EventRef only (no full event copy in klynx DB)
- [ ] **6.3** `gwdeliverycons`: consume `gw.delivery.status.v1` → update `event_refs.deliveryStatus`
- [ ] **6.4** `gwassetcons`: consume `gw.assets.changed.v1` → `assetSvc.SyncFromGW(ctx, msg)`
- [ ] **6.5** `gwsourcecons`: consume `gw.sources.changed.v1` → `orgSvc.UpdateSourceConfig(ctx, msg)`

### Phase 7 — gRPC EventService (gw) 🔒 blocks KL Phase 8

- [ ] **7.1** `proto/eventservice/v1/event.proto`
- [ ] **7.2** `internal/grpc/eventservice/server.go`: GetEvent, BatchGetEvents, ListEvents
- [ ] **7.3** Auth interceptor: no auth (appliance) — `serviceToken` validation added in Phase 9
- [ ] **7.4** Register in `main.go`, bind on `GRPC_PORT`

### Phase 8 — klynx gRPC gateway + query API — requires Phase 7 done

- [ ] **8.1** Generate proto client → `internal/gateways/gwgw/client.go` (GetEvent + BatchGetEvents only)
- [ ] **8.2** `GET /events` — event_refs → BatchGetEvents → merge → OkPaginated
- [ ] **8.3** `GET /events/{eventId}` — event_refs lookup → GetEvent → return
- [ ] **8.4** Env: `GW_GRPC_URI`

**GET /events response schema (GAP 6):**

```
GET /events?orgId=&from=&to=&page=1&perPage=20
Authorization: Bearer <jwt>
X-Active-Org: <orgId>
```

Response (`httputil.OkPaginated`):
```json
{
  "code": "SUCCESS",
  "message": "ok",
  "status": true,
  "details": {
    "items": [
      {
        "eventId":        "evt-abc",
        "orgId":          "org-xxx",
        "workspaceId":    "ws-yyy",
        "deviceId":       "dev-zzz",
        "eventType":      "motion",
        "sourceFamily":   "AIBOX",
        "score":          0.92,
        "occurredAt":     "2026-04-11T09:00:00Z",
        "deliveryStatus": "delivered",
        "detail": { }
      }
    ]
  },
  "pagination": {
    "page":         1,
    "perPage":      20,
    "totalRecords": 150,
    "totalPages":   8,
    "sortField":    "occurredAt",
    "sortOrder":    "desc"
  }
}
```

Notes:
- `items[].detail` — merged from `BatchGetEvents` (full NormalizedEvent from gw); omit or empty `{}` if gRPC unavailable
- `deliveryStatus` — from `event_refs.deliveryStatus` (klynx local); may be empty string if gw producer not yet deployed
- `GET /events/{eventId}` returns single item under `details` (not array) via `httputil.Ok`

### Phase 10 — saasPublic Entitlement Sync (gw) — GAP 4

> **Problem:** saasPublic has no shared Kafka — klynx cannot publish `klynx.entitlement.snapshot.v1` via Kafka to reach gw. Neither plan had a gw endpoint for this.

gw exposes a webhook endpoint for klynx to push entitlement snapshots:

```
klynx-api (commercial plan changes)
    └─ POST https://api.phibek.io/klynx/entitlement/sync
       X-Klynx-Timestamp: <unix>
       X-Klynx-Signature: sha256=<hmac>
       Body: EntitlementSnapshot JSON
              │
              ▼
       gw: POST /klynx/entitlement/sync
              └─ entitlementsvc.HandleSnapshot(ctx, snapshot)
                 └─ Redis TTL cache (same as appliance Kafka path)
```

- [ ] **10.1** gw router (saasPublic only):
  ```go
  klynxRoutes := r.Group("/klynx")
  klynxRoutes.All("/entitlement/sync", middleware.AllowMethods("POST"), middleware.VerifyKlynxHMAC())
  klynxRoutes.Post("/entitlement/sync", klynxwebhook.ReceiveEntitlementSync)
  ```
- [ ] **10.2** `controllers/klynxwebhook/receiveEntitlementSync.go` — parse + `entitlementsvc.HandleSnapshot`
- [ ] **10.3** `middleware/klynxhmac.go` — `VerifyKlynxHMAC()` (mirror of `VerifyGwHMAC` on klynx side)
- [ ] **10.4** Env:
  ```env
  # gw .env (saasPublic)
  KLYNX_ENTITLEMENT_WEBHOOK_SECRET=<hmac_secret>

  # klynx .env (saasPublic)
  PHIBEK_ENTITLEMENT_WEBHOOK_URL=https://api.phibek.io/klynx/entitlement/sync
  PHIBEK_ENTITLEMENT_WEBHOOK_SECRET=<hmac_secret>
  ```
- [ ] **10.5** Checklist additions:
  - [ ] **GW-18** `controllers/klynxwebhook/receiveEntitlementSync.go`
  - [ ] **GW-19** `middleware/klynxhmac.go` — `VerifyKlynxHMAC()`
  - [ ] **KL-16** klynx: push entitlement snapshot → gw API (profile guard: saasPublic → HTTP, appliance → Kafka)

---

### Phase 9 — saasKlynx + serviceToken upgrade (GAP 5)

> ⚠️ **GAP 5:** when gw adds `serviceToken` to `WorkspaceProvisionedEvent`, klynx `workspaceprovcons` handler must be updated in the **same release** — schema change is additive (Go ignores unknown fields on unmarshal) so it won't break, but klynx won't save the token until the handler is updated.

- [ ] **9.1** gw: add `ServiceToken string` to `WorkspaceProvisionedEvent`; generate + store in workspace record
- [ ] **9.2** klynx: update `workspaceprovcons` handler signature:
  ```go
  orgSvc.UpdateWorkspaceRef(ctx, msg.KlynxOrgID, msg.WorkspaceID, msg.EventIngestURI, msg.ServiceToken)
  ```
  Save `ServiceToken` to org model for use in gRPC calls
- [ ] **9.3** gw gRPC auth interceptor: validate serviceToken per-call (saasKlynx tier)
- [ ] **9.4** klynx gwGateway: attach token in gRPC metadata
  ```go
  ctx = metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+org.ServiceToken)
  ```
- [ ] **9.5** MirrorMaker 2: source=gw cluster, target=klynx cluster, topic=`gw.events.normalized.v1`
- [ ] **9.6** klynx Redis cache TTL 60s per eventId in gwGateway
- [ ] **9.7** Circuit breaker + timeout 3s in gwGateway
- [ ] **9.8** Env:
  ```env
  DEPLOYMENT_PROFILE=saasKlynx
  KAFKA_BROKERS=<local-mirror>:9092
  GW_GRPC_URI=gw-api.phibek.io:443
  # GW_SERVICE_TOKEN loaded from org record per workspace call
  ```

---

## 6. Test Sequence

### TC1 — AIBOX end-to-end (Kafka path)
```bash
curl -X POST http://<gw>/events/{orgId}/AIBOX \
  -H "Content-Type: application/json" \
  -d '{"deviceId":"cam-001","eventType":"motion","occurredAt":"2026-04-11T10:00:00Z","payload":{"confidence":0.92}}'
```
1. Kafbat: `raw.events` has message
2. gw MongoDB `event_details`: upserted
3. Kafbat: `gw.events.normalized.v1`, group `klynx-gw-events-grp` lag=0
4. klynx log: `gweventscons: received normalized event eventId=xxx`
5. klynx MongoDB `event_refs`: upserted (eventId, orgId, workspaceId, occurredAt, createdAt)

### TC2 — MQTT fast path
1. Subscribe: `events/ws/{ws}/alert/AIBOX` + `events/ws/{ws}/events/AIBOX`
2. POST event (TC1)
3. Path A arrives before normalizedcons finishes (`alertFields` present)
4. Path B arrives after normalize (`canonical: true`)

### TC3 — PVS + binary + geo
```bash
curl -X POST http://<gw>/events/{orgId}/PVS \
  -d '{"cameraId":"pvs-cam-002","alarm":"intrusion","timestamp":1712833200,"image":"<base64>"}'
```
1. `sourceFamily=PVS` template match
2. `event_details.binaryRefs` has S3 URL
3. geo enrichment applied
4. full flow as TC1

### TC4 — gRPC GetEvent
```go
client := eventservicepb.NewEventServiceClient(conn)
resp, _ := client.GetEvent(ctx, &eventservicepb.GetEventRequest{EventId:"evt-xxx", WorkspaceId:"ws-yyy"})
```
1. `GET /events/{eventId}` klynx → gRPC → gw
2. Full NormalizedEvent returned (geo, payload, binaryRefs)
3. Continuous traceId from ingest → gRPC fetch

### TC5 — list query (event_refs → BatchGetEvents)
```bash
curl "http://<klynx>/events?orgId=xxx&from=2026-04-11T00:00:00Z&to=2026-04-11T23:59:59Z"
```
1. klynx queries `event_refs` → eventIds
2. `BatchGetEvents(eventIds[], workspaceId)` → gw
3. Merged + paginated response

### TC6 — delivery status → event_refs sync
1. POST event → normalize → dispatch → outcome (delivered)
2. Kafbat: `gw.delivery.status.v1` has message
3. klynx `gwdeliverycons` log: `delivery status received eventId=xxx status=delivered`
4. klynx `event_refs.deliveryStatus` = `"delivered"`

---

## 7. Consolidated Checklist

### gateway-api

**Phase 1**
- [ ] **GW-1** `Mode` + `MatchFields` in delivery target model
- [ ] **GW-2** Default migration to `mode: normalize`
- [ ] **GW-3** API validates `type` required; returns 409 on duplicate

**Phase 2**
- [ ] **GW-4** `WarmRealtimeCache` / `InvalidateRealtimeCache`
- [ ] **GW-5** Warm on create/update, evict on delete, startup warmup

**Phase 3**
- [ ] **GW-6** IngestController: Redis check → Path A MQTT provisional

**Phase 4**
- [ ] **GW-7** Path B: MQTT canonical notify
- [ ] **GW-8** Path C: webhook mode=normalize dispatch
- [ ] **GW-9** Path D: EventBridge mode=klynx
- [ ] **GW-10** `kafka/assetseventpub/` → publish `gw.assets.changed.v1` from assetsvc/normalizedcons
- [ ] **GW-11** `kafka/sourceseventpub/` → publish `gw.sources.changed.v1` from sourcesvc
- [ ] **GW-12** `kafka/deliverystatuspub/` → publish `gw.delivery.status.v1` from deliverycons

**Phase 7**
- [ ] **GW-13** `proto/eventservice/v1/event.proto`
- [ ] **GW-14** `internal/grpc/eventservice/server.go`
- [ ] **GW-15** Auth interceptor (Phase 7: no auth; Phase 9: serviceToken)
- [ ] **GW-16** Register gRPC server in `main.go` (`GRPC_PORT`)

**Phase 9**
- [ ] **GW-17** `ServiceToken` in `WorkspaceProvisionedEvent` + stored in workspace record

### klynx-api

**Phase 5**
- [ ] **KL-1** `gwgw/deliveryTarget.go` — POST body with `type: "webhook"`; handle 409 as success
- [ ] **KL-2** Call in `orgSvc.UpdateWorkspaceRef`

**Phase 6**
- [ ] **KL-3** `eventrefsrepo/` — collection + all 3 indexes (§2.5)
- [ ] **KL-4** `ingestsvc.HandleNormalized`: upsert EventRef only
- [ ] **KL-5** `gwdeliverycons` → `event_refs.deliveryStatus`
- [ ] **KL-6** `gwassetcons` → `assetSvc.SyncFromGW`
- [ ] **KL-7** `gwsourcecons` → `orgSvc.UpdateSourceConfig`

**Phase 8**
- [ ] **KL-8** gRPC client `gwgw/client.go` (GetEvent, BatchGetEvents)
- [ ] **KL-9** `GET /events` controller
- [ ] **KL-10** `GET /events/{eventId}` controller
- [ ] **KL-11** Env: `GW_GRPC_URI`

**Phase 9**
- [ ] **KL-12** Update `workspaceprovcons` to save `serviceToken` from `WorkspaceProvisionedEvent`
- [ ] **KL-13** gwGateway: attach serviceToken in gRPC metadata per call
- [ ] **KL-14** Redis cache TTL 60s + circuit breaker 3s in gwGateway
- [ ] **KL-15** `DEPLOYMENT_PROFILE=saasKlynx` code path

### Shared
- [ ] **SH-1** TC1: AIBOX end-to-end
- [ ] **SH-2** TC2: MQTT Path A + B
- [ ] **SH-3** TC3: PVS + binary + geo
- [ ] **SH-4** TC4: gRPC GetEvent
- [ ] **SH-5** TC5: GET /events → BatchGetEvents
- [ ] **SH-6** TC6: delivery.status → event_refs sync
- [ ] **SH-7** S-2: decide NormalizedEvent shared contract strategy (Option A/B/C in §2.7)

---

## 8. File Structure

### gateway-api
```
internal/
  models/deliverymod/
    deliveryTarget.go           ← Mode, MatchFields (GW-1)
  services/deliverysvc/
    realtimeCache.go            ← WarmRealtimeCache, InvalidateRealtimeCache (GW-4,5)
  grpc/eventservice/
    server.go                   ← GetEvent, BatchGetEvents, ListEvents (GW-14)
controllers/eventapi/
  ingest.go                     ← Path A Redis check (GW-6)
kafka/
  normalizedcons/handler.go     ← Path B MQTT, Path C, Path D, asset delta publish (GW-7..10)
  assetseventpub/publisher.go   ← publish gw.assets.changed.v1 (GW-10)
  sourceseventpub/publisher.go  ← publish gw.sources.changed.v1 (GW-11)
  deliverystatuspub/publisher.go← publish gw.delivery.status.v1 (GW-12)
proto/eventservice/v1/
  event.proto                   ← (GW-13)
```

### klynx-api
```
internal/
  repo/eventrefsrepo/
    repo.go                     ← event_refs + 3 indexes (KL-3)
  gateways/gwgw/
    client.go                   ← gRPC client: GetEvent, BatchGetEvents (KL-8)
    deliveryTarget.go           ← REST: create delivery target mode=klynx (KL-1)
  services/ingestsvc/
    handleNormalized.go         ← upsert EventRef only (KL-4)
  kafka/
    gwdeliverycons/consumer.go  ← gw.delivery.status.v1 → deliveryStatus (KL-5)
    gwassetcons/consumer.go     ← gw.assets.changed.v1 → SyncFromGW (KL-6)
    gwsourcecons/consumer.go    ← gw.sources.changed.v1 → UpdateSourceConfig (KL-7)
    workspaceprovcons/consumer.go ← save serviceToken field (KL-12)
controllers/eventapi/
  list.go                       ← GET /events (KL-9)
  get.go                        ← GET /events/{eventId} (KL-10)
```
