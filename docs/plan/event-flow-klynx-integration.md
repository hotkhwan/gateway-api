# Plan: Event Flow — AIBOX/PVS → klynx platform + gRPC Fetch Pattern

**Date:** 2026-04-11  
**Repos:** `gateway-api` (gw), `klynx-api-feature` (klynx)

---

## 1. สรุป Event Flow ปัจจุบัน (หลัง rename)

### AIBOX / PVS → gateway-api → klynx

```
AIBOX / PVS device
  │
  ↓  POST /events/{orgId}/{sourceFamily}   (no JWT, rate-limited per org+IP)
  │
IngestController.Ingest()
  ├─ [Path A — Fast]  ──── check Redis "realtime_bindings:{workspaceId}" ───┐
  │    HIT: binding dispatchStage=realtime + matchFields filter              │
  │    alertdispatcher.Extract() → provisional alertFields                   │
  │    dispatch ตาม topology (appliance→MQTT, saasPublic→webhook)            │
  │    topic: events/ws/{workspaceId}/alert/{sourceFamily}                   │
  │    payload: { eventId, alertFields }   ← ยิงก่อน Kafka, provisional      │
  │    MISS: skip Path A                                                      ▼
  └─ IngestService.Ingest()                                        klynx frontend / subscriber
        template match, device resolve, generate eventId                (realtime Path A)
        ↓ publish
      [raw.events]
        ↓ NormalizerConsumer  (group: gateway-normalizer-group)
        normalize + geo enrichment + S3 binary extract
        upsert → MongoDB: event_details
        ├─ [Path B — MQTT canonical]  ─────────────────────────────────────┐
        │    MQTT → events/ws/{workspaceId}/events/{sourceFamily}          │
        │    payload: { eventId, workspaceId, canonical: true }            │
        │    ← lightweight, fired after normalization completes            ▼
        │                                                        klynx frontend / subscriber
        ├─ [Path C — bindings dispatchStage=normalize]  ─────  (canonical Path B)
        │    lookup templateDeliveryBindings → webhook dispatch ตาม matchFields
        ↓
      [DeliveryTarget mode=klynx]  → EventBridgePublisher
      [gw.events.normalized.v1]  ← klynx consumes (appliance/enterprise)
        ↓ gweventscons.StartNormalizedEventsConsumer()
      klynx ingestFacade.HandleEvent()
```

### Topic summary

| Topic | Direction | Profile | หน้าที่ |
|---|---|---|---|
| `raw.events` | internal gw | all | raw event buffer |
| `normalized.events` | internal gw | all | delivery pipeline |
| `gw.events.normalized.v1` | gw → klynx | appliance | normalized event handoff ✅ |
| `gw.workspace.provisioned.v1` | gw → klynx | appliance | workspace provisioning ack ✅ |
| `gw.assets.changed.v1` | gw → klynx | appliance | asset sync (camera/device metadata) ✅ |
| `gw.sources.changed.v1` | gw → klynx | appliance | source config sync ✅ |
| `gw.delivery.status.v1` | gw → klynx | appliance | delivery status → event_refs.deliveryStatus ✅ |
| `klynx.org.created.v1` | klynx → gw | appliance | trigger workspace creation |
| `klynx.org.deleted.v1` | klynx → gw | appliance | suspend workspace |
| `klynx.entitlement.snapshot.v1` | klynx → gw | appliance | ingest quota sync |
| MQTT `events/ws/{ws}/alert/{sf}` | gw → clients | all | fast provisional alert |
| MQTT `events/ws/{ws}/events/{sf}` | gw → clients | all | canonical event notify |

> `gw.events.normalized.v1` — **เสร็จแล้ว** (Phase 1 rename complete)  
> klynx consume ได้ผ่าน `gweventscons.StartNormalizedEventsConsumer()`

---

## 1.5 Delivery Targets — Model Semantics (3-Layer)

> ⚠️ **C-A + C-B LOCKED** — model ใน section นี้ใช้ 3-layer ที่ตัดสินใจแล้ว  
> `mode=normalize/realtime` และ `matchFields` ไม่อยู่บน `DeliveryTarget` อีกต่อไป

### 3-Layer Architecture

```
A. ingestTemplates    → classifier เท่านั้น (match / normalize / extract)
B. deliveryTargets    → destination เท่านั้น (channel / endpoint / config)
C. templateDeliveryBindings → routing เท่านั้น (template + target + dispatchStage + matchFields)
```

### Data Models

**A. ingestTemplate**
```go
type IngestTemplate struct {
    ID           string         `bson:"_id"           json:"id"`
    WorkspaceID  string         `bson:"workspaceId"   json:"workspaceId"`
    Name         string         `bson:"name"          json:"name"`
    SourceFamily string         `bson:"sourceFamily"  json:"sourceFamily"`
    MatchRules   []MatchRule    `bson:"matchRules"    json:"matchRules"`
    FieldMapping map[string]any `bson:"fieldMapping"  json:"fieldMapping"`
    Enabled      bool           `bson:"enabled"       json:"enabled"`
}
```

**B. deliveryTarget**
```go
type DeliveryTarget struct {
    ID          string         `bson:"_id"     json:"id"`
    WorkspaceID string         `bson:"workspaceId" json:"workspaceId"`
    Name        string         `bson:"name"    json:"name"`
    Type        string         `bson:"type"    json:"type"`    // webhook|line|telegram|discord
    Mode        string         `bson:"mode,omitempty" json:"mode,omitempty"` // "klynx" เท่านั้น — routing marker
    Enabled     bool           `bson:"enabled" json:"enabled"`
    Config      map[string]any `bson:"config"  json:"config"`  // url, hmacSecret, channelId, ...
}
// mode=klynx = routing marker เท่านั้น (Kafka EventBridge vs HTTP)
// regular targets ไม่มี mode field
```

**C. templateDeliveryBinding**
```go
type TemplateDeliveryBinding struct {
    ID                string         `bson:"_id"               json:"id"`
    WorkspaceID       string         `bson:"workspaceId"       json:"workspaceId"`
    TemplateID        string         `bson:"templateId"        json:"templateId"`
    TargetID          string         `bson:"targetId"          json:"targetId"`
    DispatchStage     string         `bson:"dispatchStage"     json:"dispatchStage"`     // "normalize" | "realtime"
    MatchFields       map[string]any `bson:"matchFields,omitempty" json:"matchFields,omitempty"`
    MessageTemplateID string         `bson:"messageTemplateId,omitempty" json:"messageTemplateId,omitempty"`
    Enabled           bool           `bson:"enabled"           json:"enabled"`
}
// DispatchStage = pre-Kafka dispatch stage — ไม่ใช่ transport
// MatchFields evaluate บน normalized event context เท่านั้น — ห้าม evaluate บน raw payload
```

### Field Semantics

| Layer | Field | ความหมาย | ค่า |
|-------|-------|----------|-----|
| `deliveryTarget` | `type` | delivery channel | `webhook`, `line`, `telegram`, `discord` — ⚠️ `mqtt` NOT Phase B |
| `deliveryTarget` | `mode` | routing marker (system only) | `klynx` เท่านั้น — Kafka EventBridge; absent สำหรับ regular target |
| `templateDeliveryBinding` | `dispatchStage` | จุดที่ gw ยิง event | `normalize` (หลัง normalize), `realtime` (pre-Kafka ~0ms) |
| `templateDeliveryBinding` | `matchFields` | event selector | `{ "eventType": "intrusion" }` — evaluate on normalized context |

> **dispatchStage คือ dispatch stage เท่านั้น** — ไม่ใช่ transport  
> transport จริง: appliance=MQTT, saasPublic=HTTP webhook  
> `realtime` = pre-Kafka dispatch (~0ms) ไม่ได้การันตี delivery

### Payload ต่อ dispatchStage

| dispatchStage | payload | หมายเหตุ |
|---------------|---------|----------|
| `realtime` | `{ eventId, alertFields }` — provisional snapshot | ผู้รับต้อง fetch full details ด้วย eventId ภายหลัง |
| `normalize` | full `NormalizedEvent` snapshot ผ่าน webhook URL + HMAC | ⚠️ ไม่ใช่ `{ eventId }` — ส่งครบเสมอ |

> **MQTT Path B** (`{ eventId, workspaceId, canonical: true }`) ≠ mode=normalize webhook  
> Path B คือ canonical notify ผ่าน MQTT เท่านั้น — คนละช่องทาง

### ความรับผิดชอบแยกกัน

```
ingestTemplates รับผิดชอบ:
    ├─ match raw payload (key/value classification)
    ├─ extract fields (eventType, deviceId, score, binaryRefs, ...)
    └─ normalize / classify event context

deliveryTargets รับผิดชอบ:
    ├─ destination (URL, channel config)
    ├─ delivery channel (type: webhook / line / telegram)
    └─ routing mechanism (mode=klynx → Kafka EventBridge)

templateDeliveryBindings รับผิดชอบ:
    ├─ เชื่อม template → target
    ├─ dispatch stage (normalize / realtime)
    ├─ event selector (matchFields)
    └─ message template reference
```

**ห้ามย้าย dispatchStage หรือ matchFields ลง template** — template เป็น classifier เท่านั้น  
**ห้ามย้าย dispatchStage หรือ matchFields ลง target** — target เป็น destination เท่านั้น

### dispatchStage: realtime — Redis Cache (กลไกหลัก)

ทุก event ที่เข้า IngestController ต้องตรวจว่า workspace นี้มี realtime binding ไหม  
ถ้า lookup MongoDB ทุก event → latency สูงมาก → ต้อง cache ใน Redis:

```
Redis key:  realtime_bindings:{workspaceId}
Value:      JSON array of TemplateDeliveryBinding (dispatchStage=realtime) สำหรับ workspace นั้น
TTL:        ไม่ expire — invalidate เมื่อ binding เปลี่ยน

IngestController flow:
    redis.Get("realtime_bindings:{workspaceId}")
    ├─ HIT  → สำหรับแต่ละ binding: ตรวจ matchFields → resolve target → ยิง provisional ตาม topology
    └─ MISS → skip Path A (ไม่มี realtime binding สำหรับ workspace นี้)

dispatchStage=realtime + appliance (shared MQTT broker):
    → MQTT publish → events/ws/{workspaceId}/alert/{sourceFamily}
    payload: { eventId, alertFields }   ← provisional

dispatchStage=realtime + saasPublic (ไม่มี shared broker):
    → HTTP webhook POST ตาม target.config.url
    payload: { eventId, alertFields }   ← provisional
```

Cache invalidation — **MUST cover all mutation paths**:
- สร้าง binding dispatchStage=realtime → `redis.Set("realtime_bindings:{workspaceId}", ...)`
- แก้ไข binding dispatchStage=realtime → `redis.Set("realtime_bindings:{workspaceId}", ...)`
- ลบ binding dispatchStage=realtime → `redis.Del("realtime_bindings:{workspaceId}")`
- **ลบ workspace** → `redis.Del("realtime_bindings:{workspaceId}")` ← ห้ามลืม
- Warm cache ตอน startup สำหรับ workspaces ที่มี realtime binding

Guard — saasPublic scale risk:
- rate limit ต่อ target (max 100 req/s ต่อ realtime webhook target)
- max realtime bindings ต่อ workspace (เช่น 5)

### mode=klynx บน DeliveryTarget — System Routing Marker

`mode=klynx` บน `DeliveryTarget` = routing mechanism marker เท่านั้น  
บอกว่า gw ส่งผ่าน Kafka EventBridge แทน HTTP — ไม่ใช่ dispatch stage

```
ตอน workspace provisioned (appliance / enterprise เท่านั้น):
klynx-api  →  POST /workspaces/{workspaceId}/delivery-targets
              { type: "webhook", mode: "klynx", name: "klynx-platform" }

gw รับ → create DeliveryTarget(mode=klynx) → เปิด EventBridge route สำหรับ workspace นี้
ไม่ต้องสร้าง binding สำหรับ system target นี้ — EventBridge route ผ่าน target โดยตรง

Event flow (mode=klynx target):
gw normalizedcons → EventBridgePublisher → [gw.events.normalized.v1] (Kafka)
    → klynx gweventscons → ingestFacade → upsert event_refs → klynx DB
```

Validation rules — gw MUST enforce:
```
ถ้า mode=klynx:
    ├─ url field present → reject 400
    ├─ hmac / signingSecret present → reject 400
    └─ DEPLOYMENT_PROFILE=saasPublic → reject 400
```

### สรุปการทำงานร่วมกัน

```
Event เข้า IngestController:
    ├─ check Redis "realtime_bindings:{workspaceId}"
    │    ├─ HIT → per binding: ตรวจ matchFields → resolve target → ยิง provisional ตาม topology
    │    └─ MISS → skip Path A
    └─ ingestsvc.Ingest() → raw.events

normalizedcons:
    ├─ normalize + geo + S3
    ├─ MQTT Path B canonical notify (events/ws/{ws}/events/{sf})
    ├─ lookup bindings dispatchStage=normalize → webhook dispatch (ตาม matchFields + target)
    └─ check mode=klynx target → EventBridgePublisher → gw.events.normalized.v1
```

**Idempotency contract — receiver MUST handle:**

```
Receiver (klynx / frontend / subscriber) จะได้รับ notification ในลำดับที่ไม่แน่นอน:
    ├─ realtime (Path A) → อาจมาก่อน, หรือ miss ได้ถ้า workspace ไม่มี realtime binding
    ├─ canonical (Path B MQTT) → อาจมาหลัง realtime
    └─ Kafka / webhook → มาหลังสุด
Idempotency rules:
    - Receivers MUST treat realtime and canonical notifications as idempotent
    - eventId คือ stable identifier เดียวที่เชื่อถือได้
    - ห้าม assume ว่า realtime payload = complete data
    - ห้าม assume ว่าจะได้รับ realtime ทุกครั้ง — ให้ treat canonical + on-demand fetch เป็น source of truth
```

---

## 2. กลยุทธ์: klynx ไม่เก็บ event ซ้ำ — ใช้ gRPC fetch

### ปัญหาของการ copy event ลง klynx DB

- event_details อยู่ใน gateway-api MongoDB แล้ว (complete, indexed)
- klynx เก็บซ้ำ = waste storage + risk of inconsistency
- query event จาก klynx ที่ขาดข้อมูล = ต้อง sync หรือ join ยาก

### แนวทางใหม่: Thin index + gRPC on-demand fetch

```
klynx กอนสุม gw.events.normalized.v1
  ↓ รับ NormalizedEvent { eventId, workspaceId, orgId, occurredAt, eventType, sourceFamily, ... }
  ↓ บันทึกเฉพาะ lightweight index ลง klynx DB:
    {
      eventId, workspaceId, orgId, occurredAt,
      eventType, sourceFamily, deviceId,
      score,                   ← optional, จาก analytic/detection events
      deliveryStatus,          ← อัปเดตจาก gw.delivery.status.v1
      createdAt                ← timestamp ที่ klynx บันทึก index
    }
    indexes:
      { orgId: 1, occurredAt: -1 }
      { orgId: 1, eventId: 1 }  unique
      { workspaceId: 1, occurredAt: -1 }

klynx API: GET /events/{eventId}  หรือ  EventQuery
  ↓
  1. lookup lightweight index → ตรวจ permission (orgId/workspaceId)
  2. gRPC → gateway-api: GetEvent(eventId)  ← fetch full NormalizedEvent
  3. return to client
```

### gRPC endpoint ที่ต้องเพิ่มใน gateway-api

ปัจจุบันมีแค่ REST:  `GET /ingest/details/{eventId}`

ต้องเพิ่ม gRPC:
```protobuf
// proto/eventservice/v1/event.proto
syntax = "proto3";
package gw.eventservice.v1;

service EventService {
  rpc GetEvent(GetEventRequest) returns (NormalizedEventResponse);
  rpc BatchGetEvents(BatchGetEventsRequest) returns (BatchGetEventsResponse);  // klynx ใช้สำหรับ list query
  rpc ListEvents(ListEventsRequest) returns (ListEventsResponse);               // filter-based paginated list
}

message GetEventRequest {
  string event_id     = 1;
  string workspace_id = 2;  // for auth scope check
}

message BatchGetEventsRequest {
  repeated string event_ids = 1;  // list of eventIds จาก event_refs query
  string workspace_id        = 2;  // for auth scope check
}

message BatchGetEventsResponse {
  repeated NormalizedEventResponse events = 1;
}
```

> ⚠️ **NormalizedEvent shared contract** — gw และ klynx มีสำเนา `NormalizedEvent` struct แยกกันคนละ repo  
> ห้าม add / rename / remove field ใน NormalizedEvent โดยไม่ sync ทั้งสองฝั่งพร้อมกัน  
> การ deploy gw ก่อน klynx (หรือกลับกัน) ขณะที่ struct ไม่ตรงกัน จะทำให้ consumer deserialize ผิดเงียบๆ

> **klynx event list API — primary query path:**
> 1. query `event_refs` ใน klynx MongoDB เอง (pagination, filter by orgId, time range)
> 2. เอา eventIds → `BatchGetEvents(eventIds[])` → merge full details จาก gw
> 3. return merged list ไป client
>
> `ListEvents` gRPC ถูก define ไว้ใน proto และ implement ได้ แต่ **ไม่ใช่ primary path สำหรับ klynx listing**  
> klynx ต้องมี pagination, org-scoped filter, permission check — ทั้งหมดนี้ทำผ่าน `event_refs` ใน klynx DB ก่อน  
> `ListEvents` เหมาะกับ gw-internal use case หรือ caller ที่ไม่มี klynx DB index (เช่น direct gw client)
>
> **klynx MUST NOT implement `GET /events` by delegating directly to gw `ListEvents`.**  
> เหตุผล:
> - permission check (orgId/workspaceId scope) ทำ gw ไม่ได้ — klynx DB รู้ permission ของ user
> - pagination บน gw ไม่รู้ว่า klynx user มี access กี่ record
> - latency สูงกว่าการ query event_refs local แล้วค่อย BatchGet

**Auth:**
- **Appliance:** static shared secret — ส่งเป็น gRPC metadata header `x-gw-token: <GRPC_SHARED_SECRET>` (env var ทั้งสองฝั่ง)
- **saasKlynx + saasPhibek:** per-workspace `serviceToken` — ออก token ตอน workspace created, ส่งกลับใน `WorkspaceProvisionedEvent`, klynx แนบใน gRPC metadata per-call
- **saasPublic:** ไม่ใช้ gRPC — ใช้ REST fallback (`GET /ingest/details/{eventId}`) แทน; ไม่มี `serviceToken` สำหรับ tier นี้

---

## 3. Fast Event Path ผ่าน MQTT

### Flow

```
1. MQTT Path A (provisional, ~0ms หลัง ingest):
   events/ws/{workspaceId}/alert/{sourceFamily}
   payload: { eventId, alertFields }
   ← klynx / frontend subscribe → แสดง realtime alert ทันที

2. MQTT Path B (canonical, หลัง normalize เสร็จ):
   events/ws/{workspaceId}/events/{sourceFamily}
   payload: { eventId, workspaceId, canonical: true }
   ← klynx ใช้ eventId นี้ไป fetch รายละเอียด

3. klynx API call (on-demand):
   GET /events/{eventId}
   → lookup index → gRPC GetEvent(eventId) → gateway-api
   → return full NormalizedEvent to client
```

### klynx MQTT subscription pattern

```go
// klynx subscribes ผ่าน MQTT broker เดียวกับ gateway-api (appliance)
// Topic: events/ws/{workspaceId}/events/#
// QoS: 0 (fire-and-forget)
// On receive:
func handleCanonicalNotify(msg MQTT.Message) {
    var notify struct {
        EventID     string `json:"eventId"`
        WorkspaceID string `json:"workspaceId"`
        Canonical   bool   `json:"canonical"`
    }
    _ = json.Unmarshal(msg.Payload(), &notify)
    // push to connected websocket clients
    // client will call GET /events/{eventId} for full data
}
```

---

## 4. แผนทดสอบ Event จาก AIBOX/PVS

### Pre-requisites

- [ ] gateway-api deploy พร้อม env `DEPLOYMENT_PROFILE=appliance`
- [ ] klynx-api deploy พร้อม `KAFKA_TOPIC_GW_NORMALIZED=gw.events.normalized.v1`
- [ ] MQTT broker (mosquitto) accessible ทั้งสองฝั่ง
- [ ] Org สร้างแล้วใน klynx + workspace provisioned ใน gateway-api

### Test Case 1 — AIBOX event end-to-end (Kafka path)

```bash
curl -X POST https://gw.host/events/{orgId}/AIBOX \
  -H "Content-Type: application/json" \
  -d '{
    "deviceId": "cam-001",
    "eventType": "motion",
    "occurredAt": "2026-04-11T10:00:00Z",
    "payload": { "confidence": 0.92, "zone": "entrance" }
  }'
```

ตรวจสอบ:
1. `raw.events` มี message ใหม่ (Kafkat UI)
2. `event_details` collection ใน MongoDB มี document
3. `gw.events.normalized.v1` มี message (Kafkat UI)
4. klynx lightweight index บันทึก eventId
5. `GET /events/{eventId}` ใน klynx API ส่ง gRPC → gateway-api และ return full data

### Test Case 2 — Fast MQTT path

1. subscribe MQTT topic: `events/ws/{workspaceId}/alert/AIBOX`
2. POST event (เหมือน TC1)
3. ตรวจสอบว่า MQTT message มาก่อน Kafka consumer ประมวลผลเสร็จ
4. payload มี `eventId` ถูกต้อง
5. subscribe `events/ws/{workspaceId}/events/AIBOX` → รับ canonical notify หลัง normalize เสร็จ

### Test Case 3 — PVS event

```bash
curl -X POST https://gw.host/events/{orgId}/PVS \
  -H "Content-Type: application/json" \
  -d '{
    "cameraId": "pvs-cam-002",
    "alarm": "intrusion",
    "timestamp": 1712833200,
    "image": "<base64>"
  }'
```

ตรวจสอบ:
1. template mapping ถูก sourceFamily=PVS
2. binary (image) ถูก extract ไป S3 → `binaryRefs` ใน event_details
3. geo enrichment ทำงาน (ถ้า device มี lat/lng)
4. normalized event flow ครบเหมือน TC1

### Test Case 4 — gRPC RegisterDeliveryTarget จาก klynx

```go
// ทดสอบ WorkspaceService/RegisterDeliveryTarget RPC (ใช้ได้แล้ว — Phase B+)
conn, _ := grpc.NewClient(os.Getenv("GW_GRPC_URI"),
    grpc.WithTransportCredentials(insecure.NewCredentials()),
    grpc.WithDefaultCallOptions(grpc.ForceCodec(jsonCodec{})),
)
var resp struct{ TargetID string `json:"targetId"` }
conn.Invoke(ctx, "/phibek.workspace.v1.WorkspaceService/RegisterDeliveryTarget",
    map[string]string{"workspaceId": "ws-yyy"}, &resp)
// ตรวจ delivery_targets collection ใน gw MongoDB: mode=klynx, name=klynx-platform
```

### Test Case 5 — gRPC GetEvent จาก klynx (Phase C)

```go
// ทดสอบ EventService/GetEvent RPC (ต้องเพิ่ม EventService ใน gateway-api — Phase C)
conn, _ := grpc.NewClient(os.Getenv("GW_GRPC_URI"), grpc.WithTransportCredentials(...))
conn.Invoke(ctx, "/gw.eventservice.v1.EventService/GetEvent",
    &GetEventRequest{EventId: "evt-xxx", WorkspaceId: "ws-yyy"}, &resp)
```

---

## 5. saasPublic Mode — saasKlynx + saasPhibek

### ความแตกต่างจาก Appliance

| | Appliance / Enterprise | saasKlynx + saasPhibek |
|---|---|---|
| Infrastructure | เดียวกัน | แยก network |
| Kafka | shared broker | คนละ cluster |
| gRPC latency | < 1ms (localhost) | 5–30ms (internet) |
| MQTT | shared broker | ต้องเปิด external / bridge |

### Option A — MirrorMaker 2 (Kafka topic replication) ⭐ แนะนำ

```
phibek cluster                    aliza cluster (klynx)
[gw.events.normalized.v1]  →→→  [gw.events.normalized.v1]
          Kafka MirrorMaker 2
```

- klynx consumer code ไม่ต้องเปลี่ยน
- MirrorMaker 2 handle offset mapping, consumer group sync
- ต้อง configure: source cluster, target cluster, topic whitelist
- ข้อควรระวัง: latency เพิ่ม ~50-200ms ขึ้นอยู่กับ network

### Option B — saasPublic Webhook (ใช้อยู่แล้ว)

```
gateway-api deliveryOrchestrator
  ↓ POST KLYNX_DELIVERY_WEBHOOK_URL
klynx-api: POST /gw/events   (phibekwebhook handler)
  ↓ ingestFacade.HandleEvent()
```

- ไม่ต้อง Kafka bridge
- แต่ retry logic, dead-letter, ordering ต้องจัดการเอง
- เหมาะกับ low-volume หรือ event ที่ไม่ต้องการ ordering guarantee

### Option C — MQTT Bridge

```
gateway-api mosquitto broker
  ↓ bridge config → external broker
klynx MQTT broker (aliza)
  ← klynx subscribes
```

- ต้อง expose mosquitto port 8883 (MQTTS) หรือ configure bridge
- เหมาะสำหรับ realtime alert path (Path A/B) เท่านั้น
- ไม่เหมาะกับ full event payload (ใช้ Kafka/webhook แทน)

### gRPC fetch ข้าม network (saasKlynx + saasPhibek)

```
klynx-api  →  gRPC (TLS + service account token)  →  gateway-api:50051
```

- klynx env: `GW_GRPC_URI=gw-svc:50051` (appliance) หรือ `GW_GRPC_URI=gw.host:50051` (saasKlynx/saasPhibek)
- ต้อง expose `GW_GRPC_PORT=50051` ผ่าน LoadBalancer / Ingress TLS
- **Auth: service account token** — ส่งกลับใน `WorkspaceProvisionedEvent.serviceToken`
  - klynx แนบ token ใน gRPC metadata per-call
  - gw validate token ต่อ workspaceId (ไม่ต้อง cert management)
- ควรมี circuit breaker + timeout (default 3s)
- Cache eventId → response ใน Redis (TTL 60s) เพื่อลด cross-network calls

### สรุปแนะนำตาม license tier

| Tier | Kafka | MQTT | gRPC Fetch |
|---|---|---|---|
| **Appliance / Enterprise** | shared broker (direct) | shared broker | localhost gRPC |
| **saasKlynx + saasPhibek** | MirrorMaker 2 | MQTT Bridge / Option C | gRPC over TLS + cache |
| **saasPublic (existing)** | Webhook delivery | - | REST fallback |

---

## 6. Checklist — ขั้นตอนถัดไป

> **Phase dependency:**  
> klynx Phase B (CRUD): ต้องรอ gw Phase B REST API เสร็จ (สำหรับ ingest/binding/template management)  
> klynx delivery target registration: ✅ ไม่ต้องรอ — ใช้ gRPC WorkspaceService/RegisterDeliveryTarget (Phase B+ เสร็จแล้ว)  
> klynx Phase D ต้องรอ gw Phase C (EventService gRPC server) เสร็จก่อน

### gateway-api — Phase A (ทดสอบ Flow ปัจจุบัน)

- [ ] ส่ง test event `POST /events/{orgId}/AIBOX` → ตรวจ Kafbat `gw.events.normalized.v1`
- [ ] ตรวจ klynx log: `gweventscons: received normalized event`
- [ ] subscribe MQTT `events/ws/{ws}/alert/+` → ตรวจ Path A fast alert
- [ ] subscribe MQTT `events/ws/{ws}/events/+` → ตรวจ Path B canonical notify

### gateway-api — Phase B (Delivery Targets + Bindings + Publish Triggers)

> **Model update (C-A + C-B locked):**  
> - `DeliveryTarget` = destination only — ไม่มี `mode=normalize/realtime` และไม่มี `matchFields` อีกต่อไป  
> - `mode=klynx` คงอยู่บน `DeliveryTarget` เป็น **routing marker** เท่านั้น (Kafka EventBridge vs HTTP) — ไม่ใช่ dispatch stage  
> - `dispatchStage` และ `matchFields` ย้ายไปอยู่บน `templateDeliveryBinding`

> **Phase B+ update (delivery target registration):**  
> klynx ลงทะเบียน mode=klynx target ผ่าน **gRPC** (ไม่ใช่ REST) แล้ว:  
> `WorkspaceService/RegisterDeliveryTarget` — RPC ใหม่บน gRPC server เดิม (port 50051)  
> ✅ gw: `targetsvc.RegisterKlynxTarget` + `workspacegrpc.registerDeliveryTargetHandler`  
> ✅ klynx: `phibekgw.Client.RegisterKlynxDeliveryTarget` — ใช้ connection เดิม (GW_GRPC_URI)  
> ✅ `gwgw.DeliveryTargetClient` (REST) ถูกลบแล้ว — ไม่ต้องใช้ `GW_API_URL` สำหรับ delivery target อีกต่อไป

**DeliveryTarget model (updated):**
```json
{ "id": "...", "name": "...", "type": "webhook|line|telegram|discord", "enabled": true, "config": {} }
// system target only (สร้างผ่าน gRPC RegisterDeliveryTarget — ไม่ใช่ REST):
{ "type": "webhook", "mode": "klynx", "name": "klynx-platform" }
```

**templateDeliveryBinding model:**
```json
{
  "id": "...", "templateId": "...", "targetId": "...",
  "dispatchStage": "normalize | realtime",
  "matchFields": { "eventType": "intrusion" },
  "messageTemplateId": "...",
  "enabled": true
}
```

**WorkspaceService gRPC (port 50051) — methods:**
```
ProvisionFromOrg(KlynxOrgID, TenantID, Name, CreatedBy) → (WorkspaceID, EventIngestURI)
RegisterDeliveryTarget(WorkspaceID) → (TargetID)   ← NEW Phase B+
```

- [ ] `DeliveryTarget` CRUD: `POST/GET/PATCH/DELETE /workspaces/{workspaceId}/delivery-targets`
  - Request (normal): `{ "type": "webhook|line|telegram|discord", "name": string, "config": {...} }`
  - Response `201`: `{ "code": "SUCCESS", "details": { "id": "...", ... } }`
  - Response `409`: ถ้า `name` ซ้ำใน workspace
- [ ] `templateDeliveryBinding` CRUD: `POST/GET/PATCH/DELETE /workspaces/{workspaceId}/delivery-bindings`
  - Request: `{ "templateId": "...", "targetId": "...", "dispatchStage": "normalize|realtime", "matchFields"?: {...}, "messageTemplateId"?: "..." }`
- [ ] `ingestTemplate` CRUD: `POST/GET/PATCH/DELETE /workspaces/{workspaceId}/ingest-templates`
  - fields: `sourceFamily`, `matchRules`, `fieldMapping`, `enabled`
- [ ] `messageTemplate` CRUD: `POST/GET/PATCH/DELETE /workspaces/{workspaceId}/message-templates`
  - fields: `channel` (line|webhook|telegram|discord), `body` (template string), `locale`
- [ ] Redis cache `realtime_bindings:{workspaceId}` — bindings where `dispatchStage=realtime`, warm on create/update, invalidate on delete/workspace delete
- [ ] `IngestController`: ก่อน Kafka check Redis realtime bindings → ยิง provisional ถ้ามี (ตาม matchFields + topology)
- [ ] `normalizedcons`: lookup bindings ของ template ที่ match → dispatch ตาม `dispatchStage=normalize` + `matchFields`
- [ ] `normalizedcons`: publish EventBridge เฉพาะ workspace ที่มี `mode=klynx` target (ไม่ผ่าน binding)
- [ ] กำหนด publish trigger 3 topics:
  - `gw.assets.changed.v1` — publish ใน `devicesvc` / `assetsvc` เมื่อ `Update`, `Create`, `Delete` camera/device
  - `gw.sources.changed.v1` — publish ใน `sourcesvc` เมื่อ source config เปลี่ยน (update/delete)
  - `gw.delivery.status.v1` — publish ใน `deliveryOrchestrator` หลัง delivery attempt ทุกครั้ง (success + failure)

### gateway-api — Phase C (gRPC EventService)

- [ ] เพิ่ม `EventService` gRPC server (`internal/grpc/eventservice/`)
  - `GetEvent(eventId, workspaceId) → NormalizedEvent` — klynx ใช้: single fetch
  - `BatchGetEvents(eventIds[], workspaceId) → []NormalizedEvent` — klynx ใช้: merge full data หลัง query event_refs
  - `ListEvents(workspaceId, filter) → []NormalizedEvent` — gw-internal / direct caller เท่านั้น; klynx ไม่ใช้ path นี้
- [ ] expose gRPC server บน `GW_GRPC_PORT` (default 50051) พร้อม auth middleware:
  - appliance: validate `x-gw-token` header ตรงกับ `GRPC_SHARED_SECRET`
  - saasKlynx + saasPhibek: validate per-workspace `serviceToken` จาก gRPC metadata
  - saasPublic: ไม่ผ่าน gRPC — ใช้ REST `GET /ingest/details/{eventId}` แทน

### gateway-api — Phase E (saasKlynx + saasPhibek)

- [ ] เพิ่ม `serviceToken` field ใน `WorkspaceProvisionedEvent` — generate ตอน workspace created, ส่งกลับ klynx
  - ⚠️ klynx `workspaceprovcons` ต้องอัปเดต handler รองรับ field ใหม่นี้พร้อมกัน
- [ ] gw gRPC server: validate `serviceToken` ต่อ workspaceId per-call (gRPC metadata)
- [ ] ตั้งค่า MirrorMaker 2 config สำหรับ `gw.events.normalized.v1`
- [ ] ทดสอบ gRPC fetch ข้าม network + TLS
- [ ] ประเมิน MQTT bridge vs direct webhook สำหรับ realtime path

### klynx-api (สรุปอ้างอิง — รายละเอียดใน klynx repo)

- [ ] Phase B: สร้าง `event_refs` collection + indexes + `eventrefsrepo`; แก้ `ingestsvc.HandleNormalized` upsert event_refs; ✅ `phibekgw.Client.RegisterKlynxDeliveryTarget` (gRPC) — เรียกหลัง workspace provisioned แทน REST
- [ ] Phase C: รอ gw Phase C — ไม่มี action จาก klynx ในส่วนนี้
- [ ] Phase D: สร้าง `gwgw/event.go` EventGateway + gRPC client; controllers `GET /events` + `GET /events/:eventId`
- [ ] Phase E: รองรับ `serviceToken` ใน `workspaceprovcons`; `entitlementpub`

### Testing

- [ ] TC1: AIBOX event end-to-end (Kafka path)
- [ ] TC2: MQTT fast path (provisional + canonical)
- [ ] TC3: PVS event + binary + geo
- [ ] TC4: gRPC fetch จาก klynx

### ⚠️ Shared Contract

- **NormalizedEvent** — ⚡ **time bomb ถ้าไม่จัดการ**:  
  ปัจจุบันทั้งสอง repo มีสำเนาแยก — ใช้ **copy-by-convention**:
  - เมื่อจะ add / rename / remove field ต้อง PR ทั้งสอง repo พร้อมกัน
  - deploy พร้อมกัน — ห้าม deploy ฝั่งใดฝั่งหนึ่งก่อน
  - ถ้า struct ไม่ตรงกัน → consumer deserialize ผิดแบบ silent (ไม่ crash แต่ข้อมูลผิด)
  - **Roadmap**: ย้ายไป shared proto / schema registry เพื่อ eliminate manual sync

- **mode: realtime** delivery targets — สร้างโดย workspace admin ผ่าน gw API โดยตรง, klynx ไม่ได้ create/manage targets ประเภทนี้
