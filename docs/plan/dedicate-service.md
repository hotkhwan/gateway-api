# Dedicated Service Plan: phibek + klynx-api Separation

**Branch:** `feature/dedicate-phibek-service`  
**Repos:**
- `klynx-api` → `github.com/pointitconsulting/klynx-api` (Core Platform API)
- `phibek` → `github.com/hotkhwan/gateway-api` (Event Normalization & Integration Gateway)

---

## Product Vision

สองเหตุผลหลักที่แยก:

1. **แยก responsibility ให้ชัดเจน** — phibek เป็น event/asset ingestion platform, klynx เป็น visualization/ops platform
2. **phibek สามารถแยกขายเป็น standalone product ได้** — event + asset (camera/cctv/iot/location) platform มี package เป็นของตัวเอง

### Deployment Models

| Model | Description |
|-------|-------------|
| **Appliance Box** | phibek-api + klynx-api co-located บน box เดียวกัน, ใช้ Kafka internal |
| **SaaS Split** | phibek SaaS และ klynx SaaS deploy แยกกัน, integrate ผ่าน webhook (deliveryOrchestrator) |

### Product Boundary

| Product | หน้าที่ |
|---------|---------|
| **phibek** | Event ingress, normalization, asset runtime (camera/iot/location), delivery, pipeline, ingest entitlement |
| **klynx-api** | Org/user/access management, dashboard/VR/AR/MAP, operator UX, BI, commercial subscription |

klynx-api ยกเลิกการรับ raw event โดยตรง — รอรับเฉพาะ normalized events จาก phibek เท่านั้น

> **หลักการสำคัญ:** phibek ต้องมี domain language ของตัวเอง ห้าม reuse business vocabulary ของ klynx แบบตรง ๆ และห้าม copy service/repo/model ทั้งก้อน — ถ้า phibek จะขายแยกได้จริง มันต้องเป็นเจ้าของ event domain ของตัวเองอย่างแท้จริง

---

## phibek Domain Model

phibek มี domain ของตัวเอง ไม่ใช้ organization/subscription ของ klynx:

### Core Domains

| Domain | หน้าที่ |
|--------|---------|
| `workspace` | operational boundary (แทน organization ของ klynx) |
| `site` | location group ระดับ business (office, building, zone) |
| `asset` | camera/cctv/iot/sensor/gateway runtime record |
| `source` | แหล่ง event ingress (webhook endpoint, MQTT topic, stream) |
| `pipeline` | normalization/routing/enrichment flow config |
| `deliveryTarget` | webhook outbound target (gRPC/kafkaPrivate: future option) |
| `ingestPolicy` | runtime rules ระดับ workspace/source |
| `runtimeEntitlement` | snapshot ที่ enforcement ใช้จริง |

> `workspace` ≠ `organization` — workspace คือ operational container, ไม่มี orgUnit, ไม่มี recursive permission hierarchy

### Runtime Entitlement Fields

phibek มี subscription/entitlement แยกจาก klynx commercial plan:

```go
type RuntimeEntitlement struct {
    WorkspaceID           string   `json:"workspaceId"`
    PlanCode              string   `json:"planCode"`
    MaxEventsPerSecond    int      `json:"maxEventsPerSecond"`
    MaxPayloadBytes       int      `json:"maxPayloadBytes"`
    MaxAssets             int      `json:"maxAssets"`
    MaxSources            int      `json:"maxSources"`
    MaxPipelines          int      `json:"maxPipelines"`
    MaxSites              int      `json:"maxSites"`
    AllowedSourceFamilies []string `json:"allowedSourceFamilies"`
    RetentionDays         int      `json:"retentionDays"`
    WebhookTargetsLimit   int      `json:"webhookTargetsLimit"`
    EventExportEnabled    bool     `json:"eventExportEnabled"`
    AssetTrackingEnabled  bool     `json:"assetTrackingEnabled"`
}
```

klynx-api เป็นผู้แปลง commercial plan → runtime entitlement แล้ว publish snapshot ให้ phibek ผ่าน `klynx.entitlement.snapshot.v1`

### Permission Model (Simplified)

ไม่ใช้ ReBAC ลึก ใช้ workspace-scoped RBAC ธรรมดา:

| Role | Permissions |
|------|-------------|
| `owner` | all |
| `admin` | manageAssets, manageSources, managePipelines, manageDeliveryTargets, viewEvents |
| `operator` | managePipelines, viewEvents |
| `viewer` | viewEvents |

---

## Vision (ย่อ)

แยก responsibility ออกจากกันชัดเจน:

| Service | หน้าที่ |
|---------|---------|
| **phibek** | รับ raw events จากทุกแหล่ง → normalize → forward ออกไป |
| **klynx-api** | Core platform: devices, orgs, users, permissions (Permify), subscriptions, BI/dashboard |

---

## Communication Architecture

### Two Planes

| Plane | หน้าที่ | Transport |
|-------|---------|-----------|
| **Event/Data Plane** | normalized event handoff | Kafka internal (appliance), Webhook (saasPublic) |
| **Control Plane** | config push, policy, health, admin | gRPC หรือ REST |

### Transport Decision Matrix

| Profile | phibek→klynx handoff | Transport |
|---------|----------------------|-----------|
| `appliance` | EventBridge (internal service handoff) | Kafka internal |
| `saasPublic` | deliveryOrchestrator (cross-product delivery) | Webhook HTTPS + HMAC |

> **ห้าม expose Kafka broker สู่ภายนอก** — Kafka เป็น internal backbone เท่านั้น

### phibek Delivery Connector (Outbound)

phibek มี delivery layer ที่เลือก connector ตาม deliveryTarget config:

```go
// deliveryTarget types (current scope)
type: "webhook"       // HTTPS, HMAC signed — supported
type: "grpc"          // mTLS — future option, not in Phase 1-4
type: "kafkaPrivate"  // private network — future option, not in Phase 1-4
```

ทุก outbound event ต้องผ่าน phibek delivery orchestrator เสมอ — ห้าม service ยิงออกตรงโดยไม่ผ่าน delivery layer (จะเสีย retry/DLQ/audit/entitlement enforcement)

### klynx-api Inbound Connectors (3 connectors)

klynx-api มี 2 connector ปัจจุบัน + **ingestFacade** กลาง:

```
kafkaConnector   ─┐
                  ├─→ ingestFacade → ingestsvc.HandleNormalized()
webhookConnector ─┘
```

> gRPC connector เป็น future option — ไม่ implement ใน Phase 1-4

ingestFacade ทำหน้าที่แปลง event จากทุก transport ให้เข้า canonical contract เดียว ก่อนเข้า pipeline ต่อ

### Deployment Profiles (2 profiles เท่านั้น)

> phibek รองรับ 2 deployment profiles — ไม่มีมากกว่านี้

| Profile | Boundary | phibek→klynx handoff | Infra sharing |
|---------|----------|----------------------|---------------|
| `appliance` | same box / co-located | EventBridge → Kafka internal | shared Kafka, MQTT, Keycloak realm |
| `saasPublic` | internet boundary | deliveryOrchestrator → webhookAdapter | ไม่มี shared infra |

```
DEPLOYMENT_PROFILE=appliance   # co-located, Kafka internal EventBridge
DEPLOYMENT_PROFILE=saasPublic  # internet-facing, klynx เป็น delivery target
```

> **สำคัญ:** ใน `saasPublic` klynx-api ไม่ได้รับ event ผ่าน EventBridge แล้ว — klynx กลายเป็น delivery target ธรรมดารายหนึ่งใน deliveryOrchestrator เหมือน webhook target อื่น ๆ
>
> `INTER_SERVICE_MODE` เป็น legacy alias ของ Stage 1 migration เท่านั้น

---

## Phase 0 — Contract Definition

> ทำก่อนเขียนโค้ดใด ๆ ทั้งสิ้น

### 0.1 Normalized Event Schema (canonical)

กำหนด schema กลางที่ phibek ส่งออก และ klynx-api รับเข้า — **transport-agnostic** (ใช้ได้ทั้ง Kafka, webhook, gRPC)

Schema แบ่งเป็น 3 layer:

```go
// shared/eventschema/normalized.go
// ⚠️  ไม่ copy file ระหว่าง repo — ต้องเป็น shared versioned module หรือ generated artifact
type NormalizedEvent struct {
    // --- Envelope (stable, required) ---
    EventID       string    `json:"eventId"`        // dedupeKey ด้วย
    SchemaVersion string    `json:"schemaVersion"`  // "1.0" — ใส่ตั้งแต่วันแรก
    WorkspaceID   string    `json:"workspaceId"`    // phibek workspace (≠ orgId ของ klynx)
    OrgID         string    `json:"orgId,omitempty"` // klynx org ref (ถ้า mapped)
    SourceType    string    `json:"sourceType"`     // "iot", "analytic", "streamzkt", ...
    SourceFamily  string    `json:"sourceFamily"`
    OccurredAt    time.Time `json:"occurredAt"`     // event timestamp (RFC3339 UTC)
    ReceivedAt    time.Time `json:"receivedAt"`     // phibek ingest timestamp

    // --- Normalized Fields (strongly named) ---
    AssetID   string `json:"assetId,omitempty"`
    SiteID    string `json:"siteId,omitempty"`
    DeviceID  string `json:"deviceId,omitempty"`
    CamID     string `json:"camId,omitempty"`

    // --- Payload (raw + mapped) ---
    MappedFields  map[string]any `json:"mappedFields"`
    RawPayloadRef string         `json:"rawPayloadRef,omitempty"` // S3 key

    // --- Metadata ---
    TraceID string `json:"traceId,omitempty"` // traceId หลักเท่านั้น (ไม่ embed full headers)
    S3Key   string `json:"s3Key,omitempty"`   // backward compat — deprecated ใช้ RawPayloadRef
}
```

> **ข้อควรระวัง:** `TraceHeaders` ใน event payload ถูกเอาออก — trace propagation ใช้ Kafka message headers หรือ gRPC metadata headers แทน ไม่ embed transport concern ลง business event
>
> **Shared contract:** ต้องเป็น versioned shared module (Go module กลาง หรือ proto repo กลาง) — **ห้าม copy file ด้วยมือ** ระหว่าง phibek repo และ klynx-api repo เด็ดขาด

### 0.2 Kafka Topics

**phibek internal:**

| Topic | Producer | Consumer | หน้าที่ |
|-------|----------|----------|---------|
| `phibek.raw.events.v1` | phibek ingest controllers | phibek normalizedcons | raw event buffer |
| `phibek.delivery.events.v1` | phibek normalizedcons | phibek deliverycons | delivery queue |

**phibek → klynx (cross-service):**

| Topic | Producer | Consumer | หน้าที่ |
|-------|----------|----------|---------|
| `phibek.events.normalized.v1` | **phibek** | klynx-api | normalized event handoff |
| `phibek.assets.changed.v1` | **phibek** | klynx-api | asset state change |
| `phibek.sources.changed.v1` | **phibek** | klynx-api | source state change |
| `phibek.delivery.status.v1` | **phibek** | klynx-api | delivery outcome |

**klynx → phibek (control/config):**

| Topic | Producer | Consumer | หน้าที่ |
|-------|----------|----------|---------|
| `klynx.entitlement.snapshot.v1` | **klynx-api** | phibek | runtime entitlement cache |

> **หมายเหตุ migration:** ระหว่าง transition อาจยังมี `raw.events` จาก klynx-api เดิม — ให้ phibek consume ต่อ แต่ target state คือ phibek เป็น producer ของ raw events เอง (webhook controllers ย้ายมา phibek)

> **ห้าม** ใช้ `raw.events` topic เดิมของ klynx-api เป็น long-term contract — ต้อง migrate ไปใช้ `phibek.raw.events.v1` เมื่อ webhook controllers ย้ายมาแล้ว

### 0.3 gRPC Proto (future option — ไม่ใช้ใน 2 profiles ปัจจุบัน)

> **หมายเหตุ:** `appliance` ใช้ Kafka, `saasPublic` ใช้ webhook delivery orchestrator  
> gRPC proto เก็บไว้สำหรับ future private integration เท่านั้น — ไม่ implement ใน Phase 1-4

```protobuf
// proto/eventbridge/v1/bridge.proto  ← future option
syntax = "proto3";
package eventbridge.v1;

service EventBridge {
    rpc PushNormalizedEvent(NormalizedEventRequest) returns (AckResponse);
    rpc StreamNormalizedEvents(stream NormalizedEventRequest) returns (stream AckResponse);
}

message NormalizedEventRequest {
    string event_id      = 1;
    string org_id        = 2;
    string source_type   = 3;
    string source_family = 4;
    string device_id     = 5;
    string cam_id        = 6;
    int64  timestamp_ms  = 7;
    bytes  payload_json  = 8;
    bytes  mapped_fields_json = 9;
    string s3_key        = 10;
    // ❌ trace_headers ถูกเอาออก — trace ส่งผ่าน gRPC metadata headers ไม่ใช่ proto field
}

message AckResponse {
    string event_id    = 1;
    bool   accepted    = 2;
    string status_code = 3; // "OK" | "REJECTED" | "DUPLICATE" | "RETRY_LATER" | "INVALID_SCHEMA"
    bool   retryable   = 4;
    string reason_code = 5;
    string message     = 6;
}
```

---

## Phase 1 — phibek: Access Control (Ingest-scoped)

> phibek ต้องการ access control เฉพาะ ingest gate เท่านั้น — **ไม่ copy authzsvc ทั้งก้อนจาก klynx-api**

### 1.1 phibek Access Control Scope

phibek ใช้ access control เพื่อ:
- validate ว่า source credential นี้อนุญาตให้ ingest ไปยัง workspace นี้ได้ไหม
- validate asset/device ownership ก่อน process event
- workspace member permission ระดับ RBAC (owner/admin/operator/viewer)

**ไม่ต้องการ:** full Permify authz management, tuple authoring, complex permission tree ของ klynx

### 1.2 Permify ใน phibek (optional / ingest-scoped only)

ถ้าใช้ Permify ใน phibek ให้ใช้ในฐานะ PDP client interface เท่านั้น:

```go
// phibek/internal/gateways/authzgw/ — lightweight client เท่านั้น
type IngestAuthzGateway interface {
    CanIngest(ctx context.Context, workspaceId, sourceId string) (bool, error)
    IsAssetOwned(ctx context.Context, workspaceId, assetId string) (bool, error)
}
```

ห้าม copy authzsvc/authzgw เต็มชุดจาก klynx-api มาใส่ phibek — phibek ไม่ต้องจัดการ permission authoring หรือ relationship management เอง

### 1.3 Wire ใน AppContainer

```go
// phibek/internal/app/container.go
type AppContainer struct {
    // ...existing...
    IngestAuthzGw ingestAuthzGateway
}
```

### 1.4 Phase 1 Scope Summary

| จาก klynx-api | phibek ควรทำ |
|---------------|-------------|
| `authzsvc` full copy | ❌ ห้าม |
| `authzgw` full copy | ❌ ห้าม |
| lightweight `IngestAuthzGateway` interface | ✅ สร้างใหม่ใน phibek |
| Permify config/client | ✅ ใช้ได้ (config เท่านั้น) |

---

## Phase 2 — phibek: Runtime Entitlement

> phibek ต้องการ runtime enforcement quota เท่านั้น — **ไม่ copy subscriptionsvc/subscriprepo จาก klynx-api**

### 2.1 Separation: Commercial Plan vs Runtime Entitlement

| ของ klynx-api | ของ phibek |
|---------------|-----------|
| Subscription plans (Starter/Pro/Enterprise) | Runtime entitlement snapshot |
| Billing cycle, seat count | maxEventsPerSecond, maxPayloadBytes |
| Package catalog, add-ons | maxAssets, retentionDays, allowedSourceFamilies |
| Commercial CRUD workflows | webhookTargetsLimit, maxPipelines |

phibek ต้องการแค่ **entitlement snapshot** ที่ klynx-api สร้างให้ ไม่ต้องรู้ commercial plan structure

> ⚠️ **Transitional Bridge Warning**
>
> Phase 2 design นี้เป็น **transitional architecture** — klynx-api เป็น entitlement authority ชั่วคราวระหว่าง migration
>
> - phibek business logic **ห้ามรู้จัก klynx commercial plan shape** (ชื่อ plan, package fields, billing fields)
> - `RuntimeEntitlement` struct ต้องเป็น **product-neutral snapshot** เท่านั้น — ไม่มี field ที่ map กับ klynx plan โดยตรง
> - Stage 2 target: phibek มี entitlement control plane ของตัวเอง, klynx กลายเป็น consumer รายหนึ่งของ phibek entitlement API แทน

### 2.2 Entitlement Sync Pattern

```
klynx-api  →  topic: klynx.entitlement.snapshot.v1  →  phibek (consume + cache Redis TTL)
```

klynx-api เป็นผู้แปลง commercial plan → runtime entitlement แล้ว publish
phibek consume มา cache ไว้ใน `entitlementCache` — ใช้ gate ingest โดยไม่ต้อง query klynx ทุก event

### 2.3 phibek Entitlement Module (ใหม่ ไม่ใช่ copy)

```go
// phibek/internal/services/entitlementsvc/  ← สร้างใหม่ ไม่ copy จาก klynx
type EntitlementService interface {
    GetWorkspaceEntitlement(ctx context.Context, workspaceId string) (*RuntimeEntitlement, error)
    CheckIngestAllowed(ctx context.Context, workspaceId string, payloadBytes int) error
}
```

```
internal/kafka/entitlementcons/consumer.go  ← consume klynx.entitlement.snapshot.v1
```

### 2.4 Phase 2 Scope Summary

| จาก klynx-api | phibek ควรทำ |
|---------------|-------------|
| `subscriptionsvc` copy | ❌ ห้าม |
| `subscriprepo` copy | ❌ ห้าม |
| `subscripmod` copy | ❌ ห้าม |
| consume entitlement snapshot | ✅ สร้างใหม่ |
| `entitlementsvc` (phibek-specific) | ✅ สร้างใหม่ |
| Redis cache for entitlement | ✅ ใช้ TTL cache |

---

## Phase 3 — phibek: Event Normalization Pipeline (Core)

> นี่คือหัวใจของ phibek

### 3.1 สิ่งที่ย้ายจาก klynx-api → phibek

| Component | klynx-api (เดิม) | phibek (ใหม่) |
|-----------|-----------------|----------------|
| Webhook handlers | `controllers/webhooks/` | **ย้ายมา phibek** |
| Raw event Kafka consumer | `normalizedcons/` | **ย้ายมา phibek** |
| Template matching | `ingestsvc/templateMatch.go` | **ย้ายมา phibek** |
| Normalizer consumer | `normalizedcons/normalize.go` | **ย้ายมา phibek** |
| S3 payload writer | `normalizedcons/s3writer.go` | **ย้ายมา phibek** |
| Geo enrichment | `normalizedcons/geo.go` | **ย้ายมา phibek** |
| Delivery dispatch | `deliverycons/dispatch.go` | **ย้ายมา phibek** |

### 3.2 phibek Event Pipeline (ใหม่)

```
External Sources
    ↓
[phibek controllers/webhooks/]          ← IoT, third-party, streaming
    ↓ publish to Kafka
[topic: phibek.raw.events.v1]
    ↓
[phibek normalizedcons]                 ← applyTemplate + geo + S3
    ↓ entitlement gate (Redis TTL cache)
    ↓ authz gate (Permify: canIngest)
    ↓ publish via EventBridgePublisher
[topic: phibek.events.normalized.v1]   ← Appliance Profile only
    ↓
[klynx-api consumer]                    ← receives only from phibek
```

### 3.3 phibek EventBridge Publisher (Appliance Profile เท่านั้น)

> **EventBridgePublisher ใช้เฉพาะ `appliance` profile** — internal service-to-service handoff  
> `saasPublic` ไม่ใช้ EventBridgePublisher — klynx รับผ่าน deliveryOrchestrator แทน

```go
// phibek/internal/eventbridge/publisher.go
// ใช้เฉพาะ DEPLOYMENT_PROFILE=appliance
type EventBridgePublisher interface {
    Publish(ctx context.Context, event NormalizedEvent) error
}

func NewKafkaEventBridgePublisher(writer *kafka.Writer) EventBridgePublisher {
    return &KafkaPublisher{writer: writer, topic: "phibek.events.normalized.v1"}
}
```

**saasPublic klynx handoff ผ่าน deliveryOrchestrator:**

```go
// ใน normalizedcons หลัง canonicalize สำเร็จ
switch os.Getenv("DEPLOYMENT_PROFILE") {
case "appliance":
    // internal handoff: Kafka EventBridge
    eventBridge.Publish(ctx, normalizedEvent)
case "saasPublic":
    // klynx เป็น delivery target ธรรมดา — ไม่ special-case
    // deliveryOrchestrator จะ route ตาม workspace deliveryTarget config
    // (klynx webhook target ถูก config ระดับ workspace/platform)
    kafkaPublisher.Publish(ctx, "phibek.delivery.events.v1", normalizedEvent)
}
```

---

## Phase 4 — klynx-api: Remove Direct Event Handling

> klynx-api หยุดรับ raw event โดยตรง

### 4.1 สิ่งที่ Remove ออกจาก klynx-api

```
controllers/webhooks/iot/       → ลบออก (ย้ายไป phibek)
controllers/webhooks/analytic/  → ลบออก
controllers/webhooks/streamzkt/ → ลบออก
internal/kafka/normalizedcons/  → ลบออก (phibek ทำแทน)
internal/services/ingestsvc/    → ลบออก (หรือเหลือเฉพาะ stats/review)
```

> ตรวจสอบก่อนลบ: อาจมี ingestsvc บางส่วนที่ยังใช้อยู่ใน klynx-api (เช่น ingeststatsvc, unknownpayloadreviewsvc)

### 4.2 สิ่งที่เพิ่มใน klynx-api (3 Connectors + ingestFacade)

**ingestFacade (canonical entry point):**

```go
// klynx-api/internal/eventbridge/ingestFacade.go
type IngestFacade struct {
    svc *ingestsvc.IngestService
}

func (f *IngestFacade) HandleEvent(ctx context.Context, event NormalizedEvent) error {
    ctx, end, _ := traceutil.StartLite(ctx, "klynx.ingestfacade", "ingestfacade.HandleEvent", "ingestfacade", "HandleEvent")
    defer end()
    return f.svc.HandleNormalized(ctx, event)
}
```

**Connector 1 — Kafka:**

```go
// klynx-api/internal/kafka/phibekconsumer/consumer.go
func StartKafkaConnector(brokers []string, facade *IngestFacade) {
    kafka.StartConsumerWithHeaders(brokers, "phibek.events.normalized.v1", "klynx-phibek-grp",
        func(msg NormalizedEvent, headers map[string]string) error {
            parentCtx := traceutil.ExtractHeaders(context.Background(), headers)
            ctx, end, _ := traceutil.StartLite(parentCtx, "klynx.phibekconsumer", "phibek.consume", "phibekconsumer", "handler")
            defer end()
            return facade.HandleEvent(ctx, msg)
        })
}
```

**Connector 2 — Webhook receiver:**

```go
// klynx-api/controllers/phibekwebhook/receiver.go
// รับ event จาก phibek SaaS ผ่าน HTTPS webhook + HMAC verification
func ReceivePhibekEvent(c *fiber.Ctx) error {
    ctx, end, _ := traceutil.StartLite(c.UserContext(), "klynx.phibekwebhook", "phibekwebhook.Receive", "phibekwebhook", "Receive")
    defer end()
    // verify HMAC signature
    // parse NormalizedEvent
    // facade.HandleEvent(ctx, event)
}
```

**Connector 3 — gRPC receiver (future option — ไม่ implement ใน Phase 1-4):**

> ดู `0.3 gRPC Proto` สำหรับ proto definition ถ้าจะ implement ในอนาคต

### 4.3 Startup ใน klynx-api

```go
// main.go หรือ container.go
// appliance: klynx-api consume จาก phibek Kafka EventBridge
// saasPublic: klynx-api รับผ่าน webhook receiver (phibek เป็น caller ผ่าน deliveryOrchestrator)
profile := os.Getenv("DEPLOYMENT_PROFILE") // "appliance" | "saasPublic"
facade := eventbridge.NewIngestFacade(container.IngestSvc)
switch profile {
case "saasPublic":
    // webhook receiver registered in router — phibek calls us via deliveryOrchestrator
    // no extra goroutine needed here
    log.Info().Msg("klynx running as phibek delivery target (saasPublic)")
default: // "appliance"
    go phibekconsumer.StartKafkaConnector(config.KafkaBrokers, facade)
}
```

---

## Phase 5 — Communication Layer Detail

### 5.1 Appliance Profile (same machine / monolith)

```
ENV: DEPLOYMENT_PROFILE=appliance
     KAFKA_BROKER=localhost:9092
```

```
phibek  --(publish)--> [Kafka: phibek.events.normalized.v1] --(consume)--> klynx-api
```

- ไม่ต้อง expose port เพิ่ม
- trace propagate ผ่าน Kafka message headers
- ง่ายที่สุด, latency ต่ำสุดบนเครื่องเดียวกัน

### 5.2 SaaS Public Profile (internet boundary)

```
ENV: DEPLOYMENT_PROFILE=saasPublic
     KLYNX_DELIVERY_WEBHOOK_URL=https://api.klynx.io/phibek/events
     KLYNX_DELIVERY_WEBHOOK_SECRET=<hmac_secret>
```

```
phibek deliveryOrchestrator
    └──▶ webhookAdapter → POST https://api.klynx.io/phibek/events
                          X-Phibek-Timestamp: <unix>
                          X-Phibek-Signature: sha256=<hmac>
                               │
                               ▼
                          klynx-api webhookConnector → ingestFacade
```

- klynx-api เป็น delivery target ธรรมดา — ไม่ต่างจาก webhook target อื่น
- retry/DLQ/backoff จัดการโดย deliveryOrchestrator
- trace propagate ผ่าน `X-Phibek-TraceId` header (ไม่ใช่ gRPC metadata)

### 5.3 Trace Propagation

> ⚠️ **Trace propagation ใช้ transport headers เท่านั้น — ห้าม embed ใน event payload**
>
> `TraceHeaders` ถูกเอาออกจาก `NormalizedEvent` struct แล้ว  
> event payload เก็บแค่ `traceId` (string) เพื่อ business correlation เท่านั้น  
> trace context ต้องส่งผ่าน transport headers แยกออกจาก business payload เสมอ

**Appliance Profile — Kafka message headers:**
```go
// phibek publish — inject trace into Kafka message headers (ไม่ใช่ event body)
kafkaHeaders := map[string]string{}
traceutil.InjectHeaders(ctx, kafkaHeaders)
// pass kafkaHeaders as Kafka message headers, not into event struct

// klynx-api consume — extract from Kafka message headers
func handler(msg NormalizedEvent, kafkaHeaders map[string]string) error {
    parentCtx := traceutil.ExtractHeaders(context.Background(), kafkaHeaders)
    ctx, end, _ := traceutil.StartLite(parentCtx, ...)
    defer end()
    // msg.TraceID ใช้ได้เฉพาะ logging correlation — ไม่ใช่ trace context
    return facade.HandleEvent(ctx, msg)
}
```

**SaaS Public Profile — HTTP headers (webhook delivery):**
```go
// phibek deliveryOrchestrator — inject trace into webhook HTTP headers
headers := map[string]string{}
traceutil.InjectHeaders(ctx, headers)
req.Header.Set("X-Phibek-TraceId", headers["traceparent"])
// X-Phibek-Timestamp และ X-Phibek-Signature ใช้ HMAC anti-replay (ดู Webhook Security section)

// klynx-api webhook receiver — extract from HTTP headers
func ReceivePhibekEvent(c *fiber.Ctx) error {
    parentCtx := traceutil.ExtractHeaders(c.UserContext(), httpHeadersToMap(c))
    ctx, end, _ := traceutil.StartLite(parentCtx, ...)
    defer end()
    return facade.HandleEvent(ctx, event)
}
```

---

## Phase 6 — Environment Config

### phibek `.env` additions

```env
# Deployment topology (2 profiles เท่านั้น)
DEPLOYMENT_PROFILE=appliance      # appliance | saasPublic

# Appliance profile: Kafka EventBridge
KAFKA_BROKERS=localhost:9092

# saasPublic profile: klynx เป็น delivery target
KLYNX_DELIVERY_WEBHOOK_URL=https://api.klynx.io/phibek/events   # saasPublic เท่านั้น
KLYNX_DELIVERY_WEBHOOK_SECRET=<hmac_secret>                      # saasPublic เท่านั้น

# Permify — phibek tenant (แยกจาก klynx tenant เสมอ)
PERMIFY_GRPC_URI=permify:3478
PERMIFY_TENANT_ID=phibek          # ← "phibek" เสมอ ไม่ใช่ "klynx"
PERMIFY_SCHEMA_VERSION=<version>

# Keycloak — shared realm (appliance) หรือ realm ของตัวเอง (saasPublic)
KEYCLOAK_REALM=klynx              # appliance: shared realm / saasPublic: phibek realm

# Entitlement sync
ENTITLEMENT_SYNC_SOURCE=kafka     # appliance: kafka | saasPublic: webhook/api
```

### klynx-api `.env` additions

```env
# Deployment topology
DEPLOYMENT_PROFILE=appliance      # appliance | saasPublic

# Remove raw event webhook env vars (if any)
# IOT_WEBHOOK_SECRET → moved to phibek
```

---

## Phase 7 — Migration Checklist

### phibek Tasks

**Phase 1 — Access Control (ingest-scoped)**
- [ ] **P-1** สร้าง `internal/gateways/authzgw/` — lightweight IngestAuthzGateway (ไม่ copy จาก klynx)
- [ ] **P-2** เพิ่ม Permify client config เฉพาะ gate check (canIngest, isAssetOwned)

**Phase 2 — Runtime Entitlement**
- [ ] **P-3** สร้าง `internal/services/entitlementsvc/` — phibek-specific entitlement (ไม่ copy subscriptionsvc)
- [ ] **P-4** สร้าง `internal/kafka/entitlementcons/` — consume `klynx.entitlement.snapshot.v1` → Redis TTL cache

**Phase 3 — Event Pipeline**
- [ ] **P-5** ย้าย `controllers/webhooks/` ทั้งหมดจาก klynx-api มา phibek
- [ ] **P-6** Port `internal/kafka/normalizedcons/` จาก klynx-api
- [ ] **P-7** Port `internal/kafka/deliverycons/` จาก klynx-api
- [ ] **P-8** สร้าง `internal/eventbridge/publisher.go` — publish ไป `phibek.events.normalized.v1`
- [ ] **P-9** สร้าง delivery connector adapters: webhookAdapter, grpcAdapter, kafkaPrivateAdapter
- [ ] ~~**P-10** สร้าง proto + generate code~~ — future option เท่านั้น
- [ ] **P-11** Wire ทุกอย่างใน AppContainer + main.go
- [ ] **P-12** เพิ่ม env vars ทั้งหมดใน `.env.example` / deploy config

**phibek Domain (ถ้าเริ่ม productize)**
- [ ] **P-D1** สร้าง workspace domain (แทน organization)
- [ ] **P-D2** สร้าง site, asset, source, pipeline, deliveryTarget domains
- [ ] **P-D3** สร้าง phibek subscription/plan catalog แยกจาก klynx

### klynx-api Tasks

- [ ] **K-1** สร้าง `internal/eventbridge/ingestFacade.go` — canonical entry point สำหรับทุก connector
- [ ] **K-2** สร้าง `internal/kafka/phibekconsumer/` — Kafka connector → ingestFacade
- [ ] **K-3** สร้าง `controllers/phibekwebhook/` — webhook receiver + HMAC verify → ingestFacade
- [ ] ~~**K-4** สร้าง gRPC server~~ — future option เท่านั้น
- [ ] ~~**K-5** generate proto~~ — future option เท่านั้น
- [ ] **K-6** อัปเดต `ingestsvc` ให้รองรับ `HandleNormalized(ctx, NormalizedEvent)`
- [ ] **K-7** สร้าง `internal/kafka/entitlementpub/` — publish entitlement snapshots ให้ phibek
- [ ] **K-8** Remove `controllers/webhooks/iot/`, `webhooks/analytic/`, `webhooks/streamzkt/`
- [ ] **K-9** Remove `internal/kafka/normalizedcons/` (ย้ายไป phibek แล้ว)
- [ ] **K-10** ตรวจสอบ `ingestsvc` ว่า sub-package ใดยังต้องการ (stats, review, fingerprint)
- [ ] **K-11** Wire ingestFacade + 3 connectors ใน main.go ตาม `DEPLOYMENT_PROFILE`
- [ ] **K-12** เพิ่ม env vars ใน `.env.example` / deploy config
- [ ] **K-13** อัปเดต router — ลบ webhook routes ที่ย้ายไปแล้ว + เพิ่ม phibek webhook receiver

### Shared Tasks

- [ ] **S-1** ตกลง NormalizedEvent schema (layered: envelope + normalized fields + payload ref) พร้อม `schemaVersion`
- [ ] **S-2** เลือก shared contract strategy: Go module กลาง หรือ proto repo กลาง (ห้าม copy file)
- [ ] **S-3** สร้าง Kafka topics: `phibek.events.normalized.v1`, `phibek.assets.changed.v1`, `klynx.entitlement.snapshot.v1`
- [ ] **S-4** ทดสอบ Appliance Profile บน local docker-compose (Kafka EventBridge)
- [ ] **S-5** ทดสอบ SaaS Public Profile บน local ด้วย 2 process แยก (deliveryOrchestrator webhook)
- [ ] ~~**S-6** ทดสอบ gRPC profile~~ — ตัดออก (ไม่ implement ใน 2 profiles ปัจจุบัน)
- [ ] **S-7** อัปเดต Swagger docs (ลบ webhook endpoints จาก klynx-api docs)
- [ ] **S-8** อัปเดต docker-compose / k8s manifests ให้รองรับ `DEPLOYMENT_PROFILE`

---

## Architecture Diagram (Target State)

### Model A — Appliance Box / Same Infra

```
External Sources (IoT / Webhooks / Third-party)
        │
        ▼
┌────────────────────────────────────┐
│              phibek                │  event + asset platform
│                                    │
│  [controllers/webhooks/]           │  ← รับ raw events
│         │ publish                  │
│  [phibek.raw.events.v1]            │
│         │ consume                  │
│  [normalizedcons]                  │  ← template match, geo, S3
│         │ gate: IngestAuthzGw      │  ← canIngest check
│         │ gate: EntitlementSvc     │  ← quota check (from cache)
│         │ publish                  │
│  [phibek.events.normalized.v1]     │
│         │                          │
│  [deliverycons]─→[deliveryTarget]  │  ← webhook outbound (gRPC/kafkaPrivate: future)
└────────────────────────────────────┘
          │ Kafka (internal/private)
          ▼
┌────────────────────────────────────┐
│            klynx-api               │  visualization + ops platform
│                                    │
│  kafkaConnector ──┐                │
│                   ├→ ingestFacade  │  ← canonical entry
│  webhookConnector─┘    │           │
│  (grpcConnector: future option)    │
│                        ▼           │
│              ingestsvc.HandleNormalized()
│                                    │
│  [Permify]   ← authz management    │
│  [Subscription] ← billing         │
│  → publish klynx.entitlement.snapshot.v1 → phibek
└────────────────────────────────────┘
```

### Model B — SaaS Platform-to-Platform

```
External Sources
        │
        ▼
┌─────────────────┐         ┌─────────────────┐
│  phibek SaaS    │         │  klynx SaaS     │
│  delivery       │─webhook─▶  webhookConnector│
│  orchestrator   │         │         │        │
│                 │         │  ingestFacade    │
│  (Kafka internal│         │         │        │
│   backbone)     │         │  ingestsvc       │
└─────────────────┘         └─────────────────┘

  klynx.entitlement.snapshot.v1
  klynx SaaS ─────────────────▶ phibek SaaS (via webhook)
```

> **ห้าม expose Kafka broker สู่ internet** — SaaS integration ใช้ webhook ผ่าน deliveryOrchestrator เท่านั้น

---

## Architecture Principles (ต้องยึดถือตลอด)

| หลักการ | รายละเอียด |
|---------|-----------|
| **ห้าม copy domain จาก klynx** | phibek ต้องมี domain language ของตัวเอง: workspace/site/asset/source ≠ org/device ของ klynx |
| **ห้าม copy service/repo/model ทั้งก้อน** | ถ้า phibek ต้องการ "บางอย่างเหมือนกัน" ให้ทำ abstraction ใหม่ ไม่ใช่ clone |
| **แยก commercial plan จาก runtime entitlement** | klynx บริหาร plan → แปลงเป็น entitlement snapshot → phibek cache ใช้ enforce |
| **Kafka = event backbone** | ไม่ expose Kafka สู่ภายนอก, ใช้เป็น internal/private transport เท่านั้น |
| **Webhook = default SaaS integration** | cross-platform / cross-boundary ใช้ webhook ก่อน |
| **gRPC = future option** | ไม่ implement ใน Phase 1-4 — reserved สำหรับ private high-trust integration |
| **ทุก outbound ผ่าน delivery layer** | ห้าม service ยิง event ออกตรงโดยไม่ผ่าน delivery orchestrator |
| **Shared contract = versioned module** | ห้าม copy file schema/proto ด้วยมือระหว่าง repo |
| **klynx เป็น consumer รายหนึ่ง ไม่ใช่ consumer คนเดียว** | phibek delivery design ต้องรองรับหลาย downstream |

## Notes

- **Shared MongoDB**: ใช้ได้เฉพาะ migration bridge ชั่วคราว — target state คือแยก DB ให้ขาด ไม่ใช่ shared DB ถาวร
- **Permify ใน phibek**: ใช้เฉพาะ ingest gate (canIngest, isAssetOwned) — ไม่ใช่ full authz management
- **Entitlement ใน phibek**: runtime enforcement เท่านั้น — billing/plan CRUD ยังอยู่ใน klynx-api
- **Delivery consumer**: ยังอยู่ใน phibek เพราะ delivery เป็นส่วนหนึ่งของ event pipeline
- **ไม่ต้อง migrate ทีเดียว**: ทำ Phase 1-3 ใน phibek ก่อน → parallel mode (ทั้งสองรับ raw event ไปพร้อมกัน) → Phase 4 ลบออกจาก klynx-api เมื่อ phibek stable
- **DEPLOYMENT_PROFILE**: ใช้เป็น deployment topology declaration — `appliance | saasPublic` (2 profiles เท่านั้น) lock ต่อ environment ไม่ toggle runtime (`INTER_SERVICE_MODE` เดิมเป็น alias ชั่วคราว)

---

## EventBridge vs deliveryOrchestrator

> สองตัวนี้ transport เหมือนกันได้ (webhook) แต่ **หน้าที่ต่างกันสิ้นเชิง**

| | EventBridgePublisher | deliveryOrchestrator |
|--|----------------------|---------------------|
| **Role** | internal service-to-service handoff | outbound delivery workflow |
| **ใครรู้จัก klynx** | รู้ — hard-wired ไปหา klynx | ไม่รู้ — klynx เป็น delivery target ธรรมดา |
| **Retry/DLQ** | minimal (transport-level) | full retry/backoff/DLQ |
| **Fan-out** | ไม่มี | รองรับหลาย target |
| **Policy/entitlement** | ไม่มี | enforce entitlement/quota |
| **Audit/metrics** | ไม่มี | delivery status tracking |
| **ใช้ใน** | `appliance` (Kafka internal) | `saasPublic` (และทุก delivery target) |

**Analogy:**
- EventBridgePublisher = คนขับรถส่งพัสดุไปปลายทางเดียว
- deliveryOrchestrator = ศูนย์กระจายสินค้าที่ตัดสินใจว่าส่งไปไหน, ใช้รถอะไร, retry ยังไง, เก็บ DLQ ไว้ที่ไหน

---

## Deployment Rollout Strategy

### Stage 1 — Appliance / Co-located (เริ่มที่นี่)

```
┌─────────────────────────────────────────────┐
│              Appliance Box                  │
│                                             │
│   klynx-api  ←─Kafka─→  phibek-api         │
│      │                      │               │
│   MongoDB_klynx          MongoDB_phibek     │
│      │                      │               │
│   Redis_shared (หรือแยก)  Redis_shared     │
│      │                      │               │
│   DEPLOYMENT_PROFILE=appliance              │
└─────────────────────────────────────────────┘
```

- Kafka broker เดียวกัน, internal only
- Keycloak realm เดียวกัน (`klynx`)
- Permify server เดียวกัน, แยก tenant
- MongoDB แยก database (ไม่ใช่ shared collection)
- ไม่ต้อง expose port เพิ่มเติม

### Stage 2 — SaaS Public (migrate ต่อ)

```
DEPLOYMENT_PROFILE=saasPublic

┌─────────────────────────┐    HTTPS Webhook (HMAC signed)    ┌─────────────────────────┐
│      phibek SaaS        │ ─────────────────────────────────▶ │       klynx SaaS        │
│   deliveryOrchestrator  │    klynx = delivery target รายหนึ่ง │  webhookConnector        │
│                         │                                    │  → ingestFacade          │
│  Kafka (internal)       │                                    │  Kafka (internal)        │
│  MongoDB (phibek)       │                                    │  MongoDB (klynx)         │
│  Keycloak (phibek realm)│                                    │  Keycloak (klynx realm)  │
│  Permify (phibek tenant)│                                    │  Permify (klynx tenant)  │
└─────────────────────────┘                                    └─────────────────────────┘
```

> **ลำดับ**: ทำ Stage 1 (appliance) ให้ stable ก่อน — Stage 2 (saasPublic) follow เมื่อ delivery orchestrator พร้อม

---

## Auth Infrastructure Sharing

### Keycloak — Shared Realm (Appliance Mode)

| ข้อ | รายละเอียด |
|-----|-----------|
| Realm | `klynx` — ใช้ร่วมกันทั้งสอง service |
| JWT | phibek validate token จาก Keycloak realm เดียวกัน |
| Client | phibek มี Keycloak client ของตัวเอง (`phibek-api`) |
| User | user account เดียวกัน — ไม่ซ้ำซ้อน |

> ใน Stage 2 (SaaS แยก): phibek มี Keycloak realm ของตัวเอง และ klynx ทำ federation/trust

### Permify — แยก Tenant (ทั้ง Appliance และ SaaS)

| Service | Tenant ID | Schema | Permission Model |
|---------|-----------|--------|-----------------|
| **klynx-api** | `klynx` | orgUnit-based (parent/child hierarchy) | ReBAC full |
| **phibek** | `phibek` | workspace-scoped flat | RBAC simple |

**เหตุผลที่ต้องแยก tenant:**
- klynx schema มี `orgUnit` (parent/child), `device`, `user` ที่ซับซ้อน
- phibek schema มีแค่ `workspace`, `member` (owner/admin/operator/viewer)
- ถ้าใช้ tenant เดียวกัน schema จะชนกัน และ permission check จะปนกัน

```go
// phibek — ใช้ Permify tenant แยก
permifyClient.Check(ctx, &v1.CheckRequest{
    TenantId: "phibek",  // ← ไม่ใช่ "klynx"
    Metadata: &v1.CheckRequestMetadata{SchemaVersion: "..."},
    Entity:   &v1.Entity{Type: "workspace", Id: workspaceId},
    Permission: "ingest",
    Subject: &v1.Subject{Type: "user", Id: userId},
})
```

**Permify Schema ของ phibek (ตัวอย่าง):**

```dsl
entity workspace {
    relation owner  @user
    relation admin  @user
    relation operator @user
    relation viewer @user

    permission ingest        = owner or admin or operator
    permission manage_assets = owner or admin
    permission view_events   = owner or admin or operator or viewer
}

entity user {}
```

---

## Org-Workspace Provisioning Flow

### Overview

เมื่อ klynx สร้าง org → ต้องสร้าง phibek workspace ด้วยอัตโนมัติ  
klynx org record ต้องเก็บ `workspaceId` และ `eventIngestUri` ที่ได้จาก phibek

### Event URI Format

```
POST /events/{workspaceId}/{sourceFamily}
```

เช่น:
```
POST /events/ws_abc123/iot
POST /events/ws_abc123/analytic
POST /events/ws_abc123/streamzkt
```

> **`sourceFamily`** คือ taxonomy ของแหล่ง event (iot, analytic, streamzkt, ...) — ไม่ใช่ event class ย่อย ๆ ภายใน payload (ซึ่งระบุใน body)

### Provisioning Flow (Control Plane)

```
klynx-api                              phibek
    │                                      │
    │── create org (local) ─────────────── │
    │── publish klynx.org.created.v1 ────▶ │
    │                                      │── create workspace record
    │                                      │── assign workspaceId (UUID)
    │                                      │── register ingest URI
    │                                      │── write Permify tuple (owner)
    │                                      │── publish phibek.workspace.provisioned.v1
    │◀──────────────────────────────────── │
    │── update org: workspaceId, eventIngestUri
    │── done ─────────────────────────────
```

### Kafka Topics (Control Plane เพิ่มเติม)

| Topic | Producer | Consumer | หน้าที่ |
|-------|----------|----------|---------|
| `klynx.org.created.v1` | klynx-api | phibek | trigger workspace creation |
| `klynx.org.deleted.v1` | klynx-api | phibek | suspend/archive workspace |
| `phibek.workspace.provisioned.v1` | phibek | klynx-api | ส่ง workspaceId + eventIngestUri กลับ |

### klynx Org Model (เพิ่ม field)

```go
// klynx-api: org model เพิ่ม phibek ref
type Organization struct {
    // ...existing fields...
    WorkspaceID    string `bson:"workspaceId,omitempty"    json:"workspaceId,omitempty"`
    EventIngestURI string `bson:"eventIngestUri,omitempty" json:"eventIngestUri,omitempty"`
}
```

### phibek Workspace Model

```go
// phibek/internal/models/workspacemod/workspace.go
type Workspace struct {
    WorkspaceID  string    `bson:"workspaceId"  json:"workspaceId"`
    KlynxOrgID   string    `bson:"klynxOrgId"   json:"klynxOrgId"`   // ref กลับ
    TenantID     string    `bson:"tenantId"     json:"tenantId"`      // Keycloak realm
    Name         string    `bson:"name"         json:"name"`
    Status       string    `bson:"status"       json:"status"`        // active | suspended | archived
    EventURI     string    `bson:"eventUri"     json:"eventUri"`      // /events/{workspaceId}/
    CreatedAt    time.Time `bson:"createdAt"    json:"createdAt"`
}
```

### Provisioning Consumer (phibek)

```go
// phibek/internal/kafka/workspaceprovcons/consumer.go
kafka.StartConsumerWithHeaders(brokers, "klynx.org.created.v1", "phibek-workspace-prov-grp",
    func(msg OrgCreatedEvent, headers map[string]string) error {
        parentCtx := traceutil.ExtractHeaders(context.Background(), headers)
        ctx, end, _ := traceutil.StartLite(parentCtx, "phibek.workspaceprovcons", "orgCreated.handle", "workspaceprovcons", "handler")
        defer end()
        return workspacesvc.ProvisionFromOrg(ctx, msg)
    })
```

### Checklist เพิ่มเติม (Provisioning)

**phibek:**
- [ ] **P-W1** สร้าง `internal/models/workspacemod/` — Workspace domain model
- [ ] **P-W2** สร้าง `internal/repo/workspacerepo/` — workspace CRUD
- [ ] **P-W3** สร้าง `internal/services/workspacesvc/` — ProvisionFromOrg, Suspend, Archive
- [ ] **P-W4** สร้าง `internal/kafka/workspaceprovcons/` — consume `klynx.org.created.v1`
- [ ] **P-W5** publish `phibek.workspace.provisioned.v1` หลัง provision สำเร็จ
- [ ] **P-W6** register ingest URI route: `POST /events/:workspaceId/:sourceFamily`
- [ ] **P-W7** write Permify workspace owner tuple เมื่อ provision

**klynx-api:**
- [ ] **K-W1** publish `klynx.org.created.v1` เมื่อสร้าง org สำเร็จ
- [ ] **K-W2** consume `phibek.workspace.provisioned.v1` → update org `workspaceId` + `eventIngestUri`
- [ ] **K-W3** เพิ่ม `workspaceId`, `eventIngestUri` field ใน org model + response DTO

---

## Dual-Path Canonical Event Pipeline

> นี่คือ architecture หลักของ phibek event processing — แยกเส้นทาง fast alert กับ canonical persistence ชัดเจน

### Overview

```
[ Edge AI / CCTV / IoT Nodes ]
          │
          ▼
[ phibek webhook controllers ]  ←── POST /events/{workspaceId}/{sourceFamily}
          │
          ├──▶ PATH A: FAST ALERT (Realtime)
          │
          └──▶ PATH B: CANONICAL PERSISTENCE (Source of Truth)
```

### Path A — Fast Alert Path (Realtime)

```
phibek webhook controller
    │── parse payload
    │── detect alert key/values (configurable rules)
    │── entitlement gate check (Redis cache)
    │── authz gate check (Permify: canIngest)
    │
    └──▶ MQTT publish: phibek/ws/{workspaceId}/alert/{sourceFamily}
              │
              ▼
         klynx-app (MQTT → WebSocket → Browser UI)
```

**กฎ Fast Alert Path:**
- ทำงานใน-memory, ไม่รอ Kafka consumer
- trigger เมื่อ payload มี alert fields ที่ config ไว้ต่อ workspace/source
- ต้องผ่าน entitlement gate ก่อน (ใช้ Redis TTL cache — ไม่ query DB)
- latency target: < 100ms end-to-end

> ⚠️ **Path A ไม่ใช่ Source of Truth**
>
> - event ที่ส่งผ่าน Path A เป็น **transient alert** เท่านั้น — อาจถูก reject/dedupe/drop ใน Path B ได้
> - UI/operator ต้องถือว่า fast alert เป็น **provisional state** ที่ยังไม่ยืนยัน
> - **canonical state ต้องมาจาก Path B เท่านั้น** — ห้ามใช้ Path A event ตัดสิน business flow
> - event จาก Path A ต้องมี marker ชัดเจน: `"provisional": true, "canonical": false`

**Provisional Alert Envelope:**
```go
type FastAlertEnvelope struct {
    EventID      string         `json:"eventId"`      // same eventId ที่ Path B จะใช้ (dedupeKey)
    WorkspaceID  string         `json:"workspaceId"`
    SourceFamily string         `json:"sourceFamily"`
    OccurredAt   time.Time      `json:"occurredAt"`
    Provisional  bool           `json:"provisional"`  // always true
    Canonical    bool           `json:"canonical"`    // always false
    AlertFields  map[string]any `json:"alertFields"`  // detected alert fields only
}
```

> UI ต้อง reconcile กับ canonical event จาก Path B เมื่อมาถึง (match ด้วย `eventId`)

### Path B — Canonical Persistence Path (Source of Truth)

```
phibek webhook controller
    │── publish to Kafka: phibek.raw.events.v1
    │
    ▼
[ phibek normalizedcons ]
    │── applyTemplate (template matching)
    │── geo enrichment
    │── write raw payload → S3 (RawPayloadRef)
    │── write canonical event → MongoDB
    │── entitlement gate (quota tracking)
    │── authz gate (asset ownership)
    │
    ├──▶ Kafka publish: phibek.events.normalized.v1
    │         └──▶ klynx-api consumer (kafkaConnector → ingestFacade)
    │
    ├──▶ MQTT publish: phibek/ws/{workspaceId}/events/{sourceFamily}
    │         └──▶ klynx-app (MQTT → WebSocket → canonical event display)
    │
    └──▶ deliverycons → deliveryTarget (per workspace config)
              ├── webhook (HMAC signed HTTPS)
              ├── gRPC (mTLS)
              └── SOP connectors (n8n / LINE / Discord / Telegram)
```

### n8n Positioning

> n8n อยู่ใน **SOP path เท่านั้น** — ห้ามอยู่ใน high-throughput streaming path

| Path | n8n ใช้ได้? | เหตุผล |
|------|------------|--------|
| Fast Alert (Path A) | ❌ | latency requirement, n8n overhead สูงเกิน |
| Canonical Persistence (Path B) | ❌ ใน normalizedcons | Kafka consumer ต้องเร็ว |
| SOP Delivery (deliverycons) | ✅ | alerting, watchlist sync, notification workflows |

### MQTT Topic Structure

```
phibek/ws/{workspaceId}/alert/{sourceFamily}     ← Path A (immediate alert)
phibek/ws/{workspaceId}/events/{sourceFamily}    ← Path B (canonical)
phibek/ws/{workspaceId}/status/{assetId}         ← asset state change
```

klynx-app subscribe ผ่าน MQTT broker เดียวกัน แล้ว forward ไป WebSocket → Browser

### Dual-Path Implementation Notes

```go
// phibek controller — trigger both paths concurrently
func IngestEvent(c *fiber.Ctx) error {
    ctx, end, log := traceutil.StartLite(c.UserContext(), "phibek.ingest", "ingest.IngestEvent", "ingest", "IngestEvent")
    defer end()

    payload, err := parseAndValidate(c)
    if err != nil { return httputil.BadRequest(c, err.Error()) }

    // gate checks (both paths share)
    if err := entitlementSvc.CheckIngestAllowed(ctx, workspaceId, len(payload.Raw)); err != nil {
        return httputil.TooManyRequests(c, "quota exceeded")
    }

    // Path A: fast alert — ต้องผ่าน bounded AlertDispatcher เสมอ (ห้าม raw goroutine)
    if alertDetector.HasAlert(payload) {
        alert := FastAlertEnvelope{
            EventID:      payload.EventID,
            WorkspaceID:  workspaceId,
            SourceFamily: sourceFamily,
            OccurredAt:   payload.OccurredAt,
            Provisional:  true,
            Canonical:    false,
            AlertFields:  alertDetector.Extract(payload),
        }
        alertDispatcher.Dispatch(alert) // drop-newest if full — non-blocking
    }

    // Path B: canonical (reliable, async via Kafka)
    if err := kafkaPublisher.Publish(ctx, "phibek.raw.events.v1", payload); err != nil {
        log.Error().Err(err).Msg("failed to publish raw event")
        return httputil.InternalServerError(c, "ingest failed")
    }

    return httputil.Accepted(c, fiber.Map{"eventId": payload.EventID})
}
```

### Checklist เพิ่มเติม (Dual-Path Pipeline)

- [ ] **P-E1** สร้าง `internal/services/alertdetectorsvc/` — detect alert key/value ตาม workspace config
- [ ] **P-E2** MQTT publish ใน Fast Alert Path ต้องผ่าน MQTT adapter (ไม่ inline ใน controller)
- [ ] **P-E3** MQTT topic structure: `phibek/ws/{workspaceId}/alert/{sourceFamily}`
- [ ] **P-E4** normalizedcons: เพิ่ม MQTT publish หลัง canonical event เขียน MongoDB สำเร็จ
- [ ] **P-E5** deliverycons: เพิ่ม SOP connector (n8n webhook, LINE Notify, Discord, Telegram)
- [ ] **P-E6** Fast Alert Path ต้องใช้ `traceutil.DetachWithParent` สำหรับ goroutine ที่ fire-and-forget
- [ ] **P-E7** กำหนด MQTT topic ACL ต่อ workspace ใน MQTT broker config
- [ ] **P-E8** Fast Alert path ต้องใช้ bounded worker pool แทน raw goroutine เพื่อป้องกัน goroutine storm ตอน burst
- [ ] **P-E9** Path A และ Path B ต้องใช้ `eventId` เดียวกัน — UI reconcile event ด้วย `eventId`

---

## Consistency & Idempotency Rules

### Provisioning Saga (Org → Workspace)

provisioning flow เป็น eventually consistent saga — ต้องออกแบบให้ทุก step เป็น idempotent

| Step | Idempotency Key | Guard |
|------|----------------|-------|
| klynx publish `klynx.org.created.v1` | `orgId` | publish-at-least-once, consumer dedupes |
| phibek `ProvisionFromOrg()` | `klynxOrgId` | upsert — ถ้า workspace มีแล้วให้ return existing, อย่า create ซ้ำ |
| phibek publish `phibek.workspace.provisioned.v1` | `workspaceId` | idempotent publish |
| klynx update org `workspaceId` | `orgId` | upsert — `$set` แทน `$insert` |

**Org Provisioning Status:**

```go
type OrgProvisionStatus string

const (
    OrgProvisionPending   OrgProvisionStatus = "pending"
    OrgProvisioning       OrgProvisionStatus = "provisioning"
    OrgProvisionActive    OrgProvisionStatus = "active"
    OrgProvisionFailed    OrgProvisionStatus = "provisionFailed"
)
```

klynx org ต้องเก็บ `provisionStatus` เพื่อให้ admin เห็นว่า workspace ยังไม่ได้รับ ack กลับมาหรือ failed

**Failure scenarios:**

| Scenario | Resolution |
|----------|-----------|
| phibek create workspace สำเร็จ แต่ publish กลับล้มเหลว | phibek retry publish (at-least-once) — klynx update idempotent |
| klynx consume provisioned แล้ว update org ล้มเหลว | Kafka retry — upsert idempotent |
| org ถูกลบระหว่าง provisioning | phibek consume `klynx.org.deleted.v1` → suspend/archive workspace |
| event ซ้ำ (replay) | ProvisionFromOrg() check existing workspaceId → skip |

### Event Ingest Idempotency

| Key | Rule |
|-----|------|
| `eventId` | deduplication key — generated ที่ ingest controller |
| Path A | fire-and-forget, no dedup needed (transient) |
| Path B (normalizedcons) | dedup ด้วย `eventId` ก่อน write MongoDB — ใช้ unique index หรือ Redis seen-key |
| Delivery | `eventId` ใช้ track delivery status — ห้าม deliver ซ้ำ |

---

## Target-State vs Transitional Architecture

| Component | Appliance (Stage 1) | saasPublic (Stage 2) |
|-----------|---------------------|---------------------|
| Keycloak | shared realm `klynx` | phibek มี realm ของตัวเอง + klynx federation/trust |
| Permify | shared server, **แยก tenant** (ทั้งสอง stage) | same — tenant แยกอยู่แล้ว |
| MongoDB | แยก database บน shared server | แยก server |
| Kafka | shared broker (internal EventBridge) | แยก broker — cross ผ่าน delivery webhook |
| phibek→klynx handoff | EventBridgePublisher (Kafka) | deliveryOrchestrator (webhook) |
| Entitlement | klynx publish snapshot → phibek | phibek มี plan/entitlement control plane ของตัวเอง |
| MQTT broker | shared broker (appliance) | phibek broker ของตัวเอง — cross-product ใช้ webhook |
| Auth semantics | phibek map Keycloak roles ของ klynx | phibek มี workspace role ของตัวเอง |

> **กฎเหล็ก Transitional → Target:**
> - shared realm/broker ใช้ได้เป็น deployment optimization ในระยะ migrate
> - ห้าม hard-code assumption ของ klynx realm/role/schema ลงใน phibek business logic
> - phibek ต้อง map ทุก external claim ผ่าน access model ของตัวเองก่อนใช้

---

## Connector Security Matrix

| Connector | Auth | Transport | Used in |
|-----------|------|-----------|---------|
| Kafka (internal) | SASL_SSL + private subnet | TLS (internal CA) | Appliance |
| Webhook (phibek → klynx) | HMAC-SHA256 + timestamp anti-replay | HTTPS (TLS 1.2+) | saasPublic |
| Webhook (phibek → SOP targets) | HMAC-SHA256 + timestamp anti-replay | HTTPS | SOP delivery path |
| MQTT (internal) | username/password + ACL per workspace | TLS (internal CA) | Appliance |
| MQTT (cross-boundary) | ❌ ห้าม | — | ไม่รองรับ ใช้ webhook แทน |
| gRPC | — | — | ไม่ implement ใน 2 profiles ปัจจุบัน (future option) |

**HMAC Signing Pattern (webhook):**

```go
// phibek signs outbound delivery
mac := hmac.New(sha256.New, []byte(secret))
mac.Write(body)
sig := hex.EncodeToString(mac.Sum(nil))
req.Header.Set("X-Phibek-Signature", "sha256="+sig)

// klynx verifies inbound from phibek
expected := computeHMAC(secret, body)
if !hmac.Equal([]byte(sig), []byte(expected)) {
    return httputil.Unauthorized(c, "invalid signature")
}
```

---

## Event Lifecycle Semantics

### State Machine

```
ingestReceived
    │── Path A ──▶ alertDispatched (provisional, canonical=false)
    │                   └── reconciled (when Path B canonical arrives)
    │
    └── Path B ──▶ rawBuffered (in Kafka phibek.raw.events.v1)
                        │
                        ▼
                   normalizing
                        │── templateMissed ──▶ unknownPayloadQueue
                        │
                        ▼
                   canonicalized (written to MongoDB, S3)
                        │
                        ├──▶ downstreamQueued
                        │         │
                        │         ├── [appliance]  → Kafka phibek.events.normalized.v1
                        │         │       └── deliveredToKlynx | retryingHandoff | handoffFailed
                        │         │
                        │         └── [saasPublic] → phibek.delivery.events.v1
                        │                 └── deliveryDispatched → delivered | retrying | dlq
                        │
                        └──▶ deliveryDispatched (SOP/webhook targets per workspace config)
                                  └── delivered | retrying | dlq
```

### Event State Definitions

| State | หมายความว่า |
|-------|------------|
| `ingestReceived` | controller รับ request สำเร็จ, `eventId` ถูก assign |
| `alertDispatched` | Path A ส่ง MQTT fast alert ออกไปแล้ว — provisional |
| `rawBuffered` | publish ลง Kafka `phibek.raw.events.v1` สำเร็จ |
| `normalizing` | normalizedcons กำลัง process |
| `canonicalized` | เขียน MongoDB + S3 สำเร็จ — นี่คือ source of truth |
| `downstreamQueued` | queued สำหรับ downstream handoff — transport-neutral (appliance: Kafka EventBridge, saasPublic: delivery queue) |
| `deliveredToKlynx` | appliance: klynx-api consume จาก Kafka สำเร็จ |
| `deliveryDispatched` | deliverycons ส่งออกผ่าน webhook/SOP target |
| `reconciled` | UI reconcile fast alert กับ canonical event ด้วย `eventId` สำเร็จ |
| `dlq` | delivery หมด retry แล้ว — อยู่ใน DLQ รอ review |

### Dedup & Replay Rules

- `eventId` ต้องเป็น globally unique per workspace (UUID v4 หรือ ULID)
- normalizedcons ต้อง dedup ด้วย `eventId` ก่อน write — ใช้ MongoDB unique index บน `{workspaceId, eventId}`
- Kafka consumer ต้อง at-least-once แต่ normalizedcons absorbs duplicate ผ่าน dedup
- replay ทำได้โดย re-consume จาก `phibek.raw.events.v1` (topic ต้องมี retention พอ)

### State Transition Ownership

| Transition | Owner component |
|-----------|----------------|
| `→ ingestReceived` | `controllers/webhooks/` (ingest controller) |
| `→ alertDispatched` | `internal/adapters/mqttadapter/` (fast alert dispatcher) |
| `rawBuffered →` | Kafka broker (at-least-once) |
| `→ normalizing` | `internal/kafka/normalizedcons/` |
| `normalizing → canonicalized` | `normalizedcons` (MongoDB write + S3 write) |
| `canonicalized → downstreamQueued` | `normalizedcons` (publish to EventBridge or delivery queue) |
| `downstreamQueued → deliveredToKlynx` | appliance: klynx-api `phibekconsumer` |
| `downstreamQueued → deliveryDispatched` | saasPublic: `internal/kafka/deliverycons/` |
| `deliveryDispatched → delivered/retrying/dlq` | `deliverycons` (per target config) |
| `alertDispatched → reconciled` | UI/frontend (match `eventId` กับ canonical) |

> ทุก state transition ต้องมี metrics/trace ที่จับได้ — ห้ามให้ state เปลี่ยนโดยไม่มีใครเห็น

---

## Glossary

### sourceFamily vs sourceType

| Term | หมายความว่า | ตัวอย่าง |
|------|------------|---------|
| `sourceFamily` | vendor/protocol/integration taxonomy — ใช้ใน ingest URI path | `hikvision`, `dahua`, `streamzkt`, `mqtt`, `iot`, `analytic` |
| `sourceType` | ingestion mode/category — ใช้ใน `NormalizedEvent` envelope | `webhookPush`, `mqttMessage`, `streamAnalytic`, `iotDevice` |

> **กฎ:** `sourceFamily` กำหนด routing/normalization template ที่จะใช้ — ห้ามปนกับ `sourceType` ซึ่งเป็น how event ถูก delivered เข้ามา
>
> Route: `POST /events/{workspaceId}/{sourceFamily}` → sourceFamily เป็น path segment  
> Payload: `{"sourceType": "webhookPush", "sourceFamily": "hikvision", ...}` → ทั้งคู่อยู่ใน body

### deploymentProfile values

| Value | ความหมาย | phibek→klynx handoff | Transport |
|-------|----------|----------------------|-----------|
| `appliance` | co-located บน box เดียวกัน | EventBridgePublisher | Kafka internal |
| `saasPublic` | internet-facing SaaS | deliveryOrchestrator | Webhook HTTPS |

> **หลักการแยก:**
> - `appliance` = internal service handoff (EventBridge เหมาะ)
> - `saasPublic` = cross-product delivery (deliveryOrchestrator เหมาะ, klynx เป็น target รายหนึ่ง)
>
> `INTER_SERVICE_MODE` เป็น legacy alias ของ Stage 1 migration — ให้ใช้ `DEPLOYMENT_PROFILE` แทนในทุก code ใหม่

---

## Fast Alert Dispatcher Architecture Contract

> ⚠️ **นี่คือ architecture constraint — ไม่ใช่แค่ checklist item**

Fast Alert Path **ห้าม** ใช้ raw goroutine `go func()` ใน hot path — ต้องผ่าน **bounded async dispatcher** เสมอ

```go
// internal/adapters/mqttadapter/alertdispatcher.go
type AlertDispatcher struct {
    queue  chan FastAlertEnvelope
    worker int
}

func NewAlertDispatcher(bufferSize, workers int) *AlertDispatcher {
    d := &AlertDispatcher{
        queue:  make(chan FastAlertEnvelope, bufferSize), // bounded
        worker: workers,
    }
    for i := 0; i < workers; i++ {
        go d.run()
    }
    return d
}

func (d *AlertDispatcher) Dispatch(alert FastAlertEnvelope) bool {
    select {
    case d.queue <- alert:
        return true
    default:
        // queue full → drop + emit metric (ไม่ block ingest path)
        metrics.AlertDropped.Inc()
        return false
    }
}
```

| Config | Recommended (appliance) |
|--------|------------------------|
| `bufferSize` | 1000 events |
| `workers` | 4–8 goroutines |
| drop policy | **drop newest** (non-blocking) + emit metric — channel `select/default` เป็น drop-newest โดย design |
| coalesce | optional: coalesce same `{workspaceId, sourceFamily}` ภายใน 100ms window |

> **drop newest** (ไม่ใช่ drop oldest) — เมื่อ queue เต็ม incoming alert ที่มาใหม่จะถูก drop ทันที  
> เหตุผล: การ drop oldest ต้องใช้ ring buffer / custom queue ที่ pop ได้ ซับซ้อนเกินความจำเป็น  
> fast alert เป็น transient อยู่แล้ว — dropping newest ไม่กระทบ canonical path

> Telemetry ที่ต้องมี: `alert_queue_depth`, `alert_dropped_total`, `alert_dispatch_latency_ms`

---

## Webhook Security: Anti-Replay

> สำหรับ `saasPublic` — cross-boundary webhook delivery

### Signed Canonical String

```
signedString = timestamp + "." + hex(sha256(body))
signature    = hmac-sha256(secret, signedString)
```

```go
// phibek signs outbound delivery
timestamp := strconv.FormatInt(time.Now().Unix(), 10)
bodyHash  := hex.EncodeToString(sha256.Sum256(body)[:])
signed    := timestamp + "." + bodyHash

mac := hmac.New(sha256.New, []byte(secret))
mac.Write([]byte(signed))
sig := hex.EncodeToString(mac.Sum(nil))

req.Header.Set("X-Phibek-Timestamp", timestamp)
req.Header.Set("X-Phibek-Signature", "sha256="+sig)
```

```go
// klynx verifies inbound from phibek
func verifyWebhook(secret string, body []byte, headers http.Header) error {
    ts    := headers.Get("X-Phibek-Timestamp")
    sig   := strings.TrimPrefix(headers.Get("X-Phibek-Signature"), "sha256=")

    // anti-replay: reject requests older than 5 minutes
    tsInt, _ := strconv.ParseInt(ts, 10, 64)
    if time.Now().Unix()-tsInt > 300 {
        return errors.New("webhook timestamp too old")
    }

    bodyHash := hex.EncodeToString(sha256.Sum256(body)[:])
    expected := computeHMAC(secret, ts+"."+bodyHash)
    if !hmac.Equal([]byte(sig), []byte(expected)) {
        return errors.New("invalid signature")
    }
    return nil
}
```

| Field | Rule |
|-------|------|
| `X-Phibek-Timestamp` | Unix epoch seconds — reject if > 5 min old |
| `X-Phibek-Signature` | `sha256=hex(hmac(secret, ts+"."+sha256(body)))` |
| Replay window | 5 minutes (300 seconds) |

---

## MQTT Boundary Rules

### Appliance Mode
- MQTT broker เดียวกัน, shared internal
- phibek publish → klynx-app subscribe ตรง broker เดียวกัน ✅
- topic ACL ต่อ workspace ต้อง config ใน broker

### SaaS Mode — MQTT เป็น internal phibek channel เท่านั้น

> **ตัดสินใจ:** MQTT ไม่ใช่ cross-product transport ใน SaaS

| Scenario | Transport |
|----------|-----------|
| phibek → klynx-app (same box) | MQTT ✅ |
| phibek SaaS → klynx SaaS | **Webhook เท่านั้น** (via deliveryOrchestrator) ❌ MQTT |
| klynx-app realtime (own frontend) | klynx มี MQTT/WebSocket ของตัวเอง (subscribe จาก klynx-api's stream) |

klynx-app ใน SaaS ไม่ subscribe จาก phibek MQTT broker โดยตรง — klynx-api รับ normalized event จาก phibek ผ่าน webhook (deliveryOrchestrator) แล้ว push realtime ให้ klynx-app ผ่าน WebSocket/MQTT ของ klynx เอง

```
phibek SaaS ──webhook──▶ klynx-api ──WebSocket──▶ klynx-app browser
                                   └──MQTT(klynx)──▶ klynx-app mobile
```
