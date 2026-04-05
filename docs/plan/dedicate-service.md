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
| **SaaS Split** | phibek SaaS และ klynx SaaS deploy แยกกัน, integrate ผ่าน webhook/gRPC |

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
| `deliveryTarget` | webhook/gRPC/kafkaPrivate outbound target |
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
| **Event/Data Plane** | normalized event handoff | Kafka (internal), Webhook/gRPC (cross-boundary) |
| **Control Plane** | config push, policy, health, admin | gRPC หรือ REST |

### Transport Decision Matrix

| Case | Transport | เหตุผล |
|------|-----------|--------|
| Appliance box | **Kafka** (internal) | co-located, latency ต่ำ, replay ได้ |
| SaaS same infra / private network | **Kafka** private | throughput, fan-out, backpressure |
| **SaaS platform-to-platform (default)** | **Webhook** | HTTPS, ซ่อน infra, retry/DLQ, ง่าย ops |
| SaaS private high-trust / deep partner | **gRPC** | structured contract, low latency, bidirectional |

> **ห้าม expose Kafka broker สู่ภายนอก** — Kafka เป็น internal backbone เท่านั้น

### phibek Delivery Connector (Outbound)

phibek มี delivery layer ที่เลือก connector ตาม deliveryTarget config:

```go
// deliveryTarget types
type: "webhook"       // HTTPS, HMAC signed
type: "grpc"          // mTLS, strongly typed
type: "kafkaPrivate"  // private network only
```

ทุก outbound event ต้องผ่าน phibek delivery orchestrator เสมอ — ห้าม service ยิงออกตรงโดยไม่ผ่าน delivery layer (จะเสีย retry/DLQ/audit/entitlement enforcement)

### klynx-api Inbound Connectors (3 connectors)

klynx-api มี 3 connector รับ event จาก phibek + **ingestFacade** กลาง:

```
kafkaConnector   ─┐
webhookConnector ─┤─→ ingestFacade → ingestsvc.HandleNormalized()
grpcConnector    ─┘
```

ingestFacade ทำหน้าที่แปลง event จากทุก transport ให้เข้า canonical contract เดียว ก่อนเข้า pipeline ต่อ

### INTER_SERVICE_MODE

```
INTER_SERVICE_MODE=kafka    # appliance / same infra (default)
INTER_SERVICE_MODE=webhook  # SaaS cross-boundary (default for SaaS)
INTER_SERVICE_MODE=grpc     # private high-trust partner
```

> อยู่ระหว่าง migration ใช้ dual-mode ได้ชั่วคราว แต่ target ควร lock ให้ชัดต่อ deployment model

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

### 0.3 gRPC Proto (สำหรับ mode=grpc)

```protobuf
// proto/eventbridge/v1/bridge.proto
syntax = "proto3";
package eventbridge.v1;

service EventBridge {
    // phibek → klynx-api: push normalized event
    rpc PushNormalizedEvent(NormalizedEventRequest) returns (AckResponse);
    // streaming variant สำหรับ high-throughput
    rpc StreamNormalizedEvents(stream NormalizedEventRequest) returns (stream AckResponse);
}

message NormalizedEventRequest {
    string event_id     = 1;
    string org_id       = 2;
    string source_type  = 3;
    string source_family = 4;
    string device_id    = 5;
    string cam_id       = 6;
    int64  timestamp_ms = 7;
    bytes  payload_json = 8;
    bytes  mapped_fields_json = 9;
    string s3_key       = 10;
    map<string, string> trace_headers = 11;
}

message AckResponse {
    string event_id    = 1;
    bool   accepted    = 2;
    string status_code = 3; // "OK", "REJECTED", "DUPLICATE", "RETRY_LATER", "INVALID_SCHEMA"
    bool   retryable   = 4;
    string reason_code = 5; // machine-readable reason
    string message     = 6; // human-readable detail
}
```

สร้างไว้ใน `proto/` ทั้งสองโปรเจกต์ และ generate ด้วย `buf generate` / `protoc`

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
[phibek controllers/webhooks/]     ← IoT, third-party, streaming
    ↓ publish to Kafka
[topic: raw.events]
    ↓
[phibek normalizedcons]            ← applyTemplate + geo + S3
    ↓ subscription gate check
    ↓ permission gate check (Permify)
    ↓ publish to Kafka/gRPC
[topic: phibek.normalized.events]  ← or gRPC stream
    ↓
[klynx-api consumer]               ← receives only from phibek
```

### 3.3 phibek Inter-Service Publisher

สร้าง publisher ที่ switch ตาม mode:

```go
// phibek/internal/eventbridge/publisher.go
type EventBridgePublisher interface {
    Publish(ctx context.Context, event NormalizedEvent) error
}

func NewPublisher(mode string, kafkaWriter *kafka.Writer, grpcClient EventBridgeClient) EventBridgePublisher {
    switch mode {
    case "grpc":
        return &GRPCPublisher{client: grpcClient}
    default: // "kafka"
        return &KafkaPublisher{writer: kafkaWriter, topic: "phibek.normalized.events"}
    }
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

**Connector 3 — gRPC receiver:**

```go
// klynx-api/internal/eventbridge/grpcServer.go
func (s *EventBridgeServer) PushNormalizedEvent(ctx context.Context, req *pb.NormalizedEventRequest) (*pb.AckResponse, error) {
    ctx, end, _ := traceutil.Start(ctx, "klynx.eventbridge", "eventbridge.PushNormalizedEvent", "eventbridge", "PushNormalizedEvent")
    defer end()
    event := mapProtoToNormalized(req)
    if err := s.facade.HandleEvent(ctx, event); err != nil {
        return &pb.AckResponse{EventId: req.EventId, Accepted: false, StatusCode: "RETRY_LATER", Retryable: true}, err
    }
    return &pb.AckResponse{EventId: req.EventId, Accepted: true, StatusCode: "OK"}, nil
}
```

### 4.3 Startup ใน klynx-api

```go
// main.go หรือ container.go
mode := os.Getenv("INTER_SERVICE_MODE") // "kafka" | "webhook" | "grpc"
facade := eventbridge.NewIngestFacade(container.IngestSvc)
switch mode {
case "grpc":
    go eventbridge.StartGRPCServer(facade, ":50051")
case "webhook":
    // webhook receiver registered in router — no extra goroutine needed
    log.Info().Msg("phibek webhook connector active")
default: // "kafka"
    go phibekconsumer.StartKafkaConnector(config.KafkaBrokers, facade)
}
```

---

## Phase 5 — Communication Layer Detail

### 5.1 Kafka Mode (same machine / monolith)

```
ENV: INTER_SERVICE_MODE=kafka
     KAFKA_BROKER=localhost:9092
```

```
phibek  --(publish)--> [Kafka: phibek.normalized.events] --(consume)--> klynx-api
```

- ไม่ต้อง expose port เพิ่ม
- trace propagate ผ่าน Kafka headers (เหมือน pattern เดิม)
- ง่ายที่สุด, latency ต่ำสุดบนเครื่องเดียวกัน

### 5.2 gRPC Mode (cloud / separate deployment)

```
ENV: INTER_SERVICE_MODE=grpc
     PHIBEK_GRPC_ADDR=phibek-service:50051    # ใน klynx-api
     KLYNX_GRPC_ADDR=klynx-api-service:50051  # ใน phibek
```

```
phibek  --(gRPC stream)--> klynx-api:50051
```

- TLS required ใน production
- trace propagate ผ่าน gRPC metadata headers
- phibek เป็น client, klynx-api เป็น server (EventBridgeServer)
- ใช้ streaming RPC เพื่อ throughput สูง

### 5.3 Trace Propagation

**Kafka mode:**
```go
// phibek publish
headers := map[string]string{}
traceutil.InjectHeaders(ctx, headers)
event.TraceHeaders = headers

// klynx-api consume
parentCtx := traceutil.ExtractHeaders(context.Background(), event.TraceHeaders)
```

**gRPC mode:**
```go
// phibek client (inject into gRPC metadata)
md := metadata.New(traceHeaders)
ctx = metadata.NewOutgoingContext(ctx, md)
client.PushNormalizedEvent(ctx, req)

// klynx-api server (extract from gRPC metadata)
md, _ := metadata.FromIncomingContext(ctx)
headers := flattenMetadata(md)
parentCtx := traceutil.ExtractHeaders(ctx, headers)
```

---

## Phase 6 — Environment Config

### phibek `.env` additions

```env
# Inter-service communication
INTER_SERVICE_MODE=kafka          # kafka | grpc
KLYNX_GRPC_ADDR=klynx-api:50051  # used when mode=grpc

# Permify (port จาก klynx-api)
PERMIFY_GRPC_URI=permify:3478
PERMIFY_SCHEMA_ID=<schema_id>
KEYCLOAK_REALM=klynx              # used as permify tenantId

# Subscription sync
SUBSCRIPTION_SYNC_MODE=kafka      # kafka | mongo-readonly
```

### klynx-api `.env` additions

```env
# Inter-service communication
INTER_SERVICE_MODE=kafka          # kafka | grpc
EVENTBRIDGE_GRPC_PORT=50051       # used when mode=grpc

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
- [ ] **P-10** สร้าง proto + generate code (`proto/eventbridge/v1/bridge.proto`)
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
- [ ] **K-4** สร้าง `internal/eventbridge/grpcServer.go` — gRPC connector → ingestFacade
- [ ] **K-5** สร้าง proto + generate code (เหมือน phibek)
- [ ] **K-6** อัปเดต `ingestsvc` ให้รองรับ `HandleNormalized(ctx, NormalizedEvent)`
- [ ] **K-7** สร้าง `internal/kafka/entitlementpub/` — publish entitlement snapshots ให้ phibek
- [ ] **K-8** Remove `controllers/webhooks/iot/`, `webhooks/analytic/`, `webhooks/streamzkt/`
- [ ] **K-9** Remove `internal/kafka/normalizedcons/` (ย้ายไป phibek แล้ว)
- [ ] **K-10** ตรวจสอบ `ingestsvc` ว่า sub-package ใดยังต้องการ (stats, review, fingerprint)
- [ ] **K-11** Wire ingestFacade + 3 connectors ใน main.go ตาม INTER_SERVICE_MODE
- [ ] **K-12** เพิ่ม env vars ใน `.env.example` / deploy config
- [ ] **K-13** อัปเดต router — ลบ webhook routes ที่ย้ายไปแล้ว + เพิ่ม phibek webhook receiver

### Shared Tasks

- [ ] **S-1** ตกลง NormalizedEvent schema (layered: envelope + normalized fields + payload ref) พร้อม `schemaVersion`
- [ ] **S-2** เลือก shared contract strategy: Go module กลาง หรือ proto repo กลาง (ห้าม copy file)
- [ ] **S-3** สร้าง Kafka topics: `phibek.events.normalized.v1`, `phibek.assets.changed.v1`, `klynx.entitlement.snapshot.v1`
- [ ] **S-4** ทดสอบ Kafka mode (appliance/same infra) บน local docker-compose
- [ ] **S-5** ทดสอบ webhook mode (SaaS cross-boundary) บน local ด้วย 2 process แยก
- [ ] **S-6** ทดสอบ gRPC mode (private connector) บน local
- [ ] **S-7** อัปเดต Swagger docs (ลบ webhook endpoints จาก klynx-api docs)
- [ ] **S-8** อัปเดต docker-compose / k8s manifests ให้รองรับ INTER_SERVICE_MODE

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
│  [deliverycons]─→[deliveryTarget]  │  ← webhook/gRPC/kafkaPrivate outbound
└────────────────────────────────────┘
          │ Kafka (internal/private)
          ▼
┌────────────────────────────────────┐
│            klynx-api               │  visualization + ops platform
│                                    │
│  kafkaConnector ──┐                │
│                   ├→ ingestFacade  │  ← canonical entry
│  webhookConnector─┘    │           │
│  grpcConnector ───┘    │           │
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
│                 │─webhook─▶  webhookConnector│
│  delivery layer │─(gRPC)──▶  grpcConnector   │
│                 │         │         │        │
│  (Kafka internal│         │  ingestFacade    │
│   backbone)     │         │         │        │
└─────────────────┘         │  ingestsvc       │
                            └─────────────────┘

  klynx.entitlement.snapshot.v1
  klynx SaaS ─────────────────▶ phibek SaaS (via webhook/API)
```

> **ห้าม expose Kafka broker สู่ internet** — SaaS integration ใช้ webhook เป็น default, gRPC เป็น premium/private option เท่านั้น

---

## Architecture Principles (ต้องยึดถือตลอด)

| หลักการ | รายละเอียด |
|---------|-----------|
| **ห้าม copy domain จาก klynx** | phibek ต้องมี domain language ของตัวเอง: workspace/site/asset/source ≠ org/device ของ klynx |
| **ห้าม copy service/repo/model ทั้งก้อน** | ถ้า phibek ต้องการ "บางอย่างเหมือนกัน" ให้ทำ abstraction ใหม่ ไม่ใช่ clone |
| **แยก commercial plan จาก runtime entitlement** | klynx บริหาร plan → แปลงเป็น entitlement snapshot → phibek cache ใช้ enforce |
| **Kafka = event backbone** | ไม่ expose Kafka สู่ภายนอก, ใช้เป็น internal/private transport เท่านั้น |
| **Webhook = default SaaS integration** | cross-platform / cross-boundary ใช้ webhook ก่อน |
| **gRPC = premium/private connector** | ใช้เมื่อต้องการ structured contract + private network |
| **ทุก outbound ผ่าน delivery layer** | ห้าม service ยิง event ออกตรงโดยไม่ผ่าน delivery orchestrator |
| **Shared contract = versioned module** | ห้าม copy file schema/proto ด้วยมือระหว่าง repo |
| **klynx เป็น consumer รายหนึ่ง ไม่ใช่ consumer คนเดียว** | phibek delivery design ต้องรองรับหลาย downstream |

## Notes

- **Shared MongoDB**: ใช้ได้เฉพาะ migration bridge ชั่วคราว — target state คือแยก DB ให้ขาด ไม่ใช่ shared DB ถาวร
- **Permify ใน phibek**: ใช้เฉพาะ ingest gate (canIngest, isAssetOwned) — ไม่ใช่ full authz management
- **Entitlement ใน phibek**: runtime enforcement เท่านั้น — billing/plan CRUD ยังอยู่ใน klynx-api
- **Delivery consumer**: ยังอยู่ใน phibek เพราะ delivery เป็นส่วนหนึ่งของ event pipeline
- **ไม่ต้อง migrate ทีเดียว**: ทำ Phase 1-3 ใน phibek ก่อน → parallel mode (ทั้งสองรับ raw event ไปพร้อมกัน) → Phase 4 ลบออกจาก klynx-api เมื่อ phibek stable
- **INTER_SERVICE_MODE**: ใช้เป็น migration/deployment switch ชั่วคราว — แต่ละ deployment model ควร lock mode ให้ชัด ไม่ toggle runtime
