# Plan: Rename `phibek.*` Kafka Topics → `gw.*` + Cleanup

**Date:** 2026-04-11  
**Repos involved:** `gateway-api` (phibek), `klynx-api-feature` (klynx)

---

## Background

Topic naming ชุด `phibek.*` สะท้อนชื่อ internal project ที่ไม่ควรโผล่ใน contract ระหว่าง service
เป้าหมายคือเปลี่ยนเป็น `gw.*` (gateway) ซึ่งสื่อความหมายของ role ได้ชัดกว่า

### Cross-service topic flow (appliance profile) — หลัง rename ✅

```
gateway-api (producer)                     klynx-api (consumer)
───────────────────────────────────────────────────────────────────
gw.events.normalized.v1        ←→   KAFKA_TOPIC_GW_NORMALIZED            (default: gw.events.normalized.v1)
gw.workspace.provisioned.v1    ←→   KAFKA_TOPIC_GW_WORKSPACE_PROVISIONED  (default: gw.workspace.provisioned.v1)
gw.assets.changed.v1           ←→   KAFKA_TOPIC_GW_ASSETS                 (default: gw.assets.changed.v1)
gw.sources.changed.v1          ←→   KAFKA_TOPIC_GW_SOURCES                (default: gw.sources.changed.v1)
gw.delivery.status.v1          ←→   KAFKA_TOPIC_GW_DELIVERY_STATUS        (default: gw.delivery.status.v1)
```

> Phase 1 code changes เสร็จแล้ว — topics ตรงกันทั้งสองฝั่ง พร้อม deploy

klynx-api → gateway-api (ทิศทางกลับ — ชื่อ ok อยู่แล้ว ไม่ต้อง rename):
```
klynx.org.created.v1            →  gateway-api orglifecyclecons
klynx.org.deleted.v1            →  gateway-api orglifecyclecons
klynx.entitlement.snapshot.v1  →  gateway-api entitlementcons
```

---

## Target topic names

| Old name | New name |
|---|---|
| `phibek.events.normalized.v1` | `gw.events.normalized.v1` |
| `phibek.assets.changed.v1` | `gw.assets.changed.v1` |
| `phibek.sources.changed.v1` | `gw.sources.changed.v1` |
| `phibek.delivery.status.v1` | `gw.delivery.status.v1` |
| `phibek.workspace.provisioned.v1` | `gw.workspace.provisioned.v1` |
| `events.workspace.provisioned.v1` | `gw.workspace.provisioned.v1` (รวมกัน) |

---

## Phase 1 — Code changes

### 1A. gateway-api

#### `internal/eventbridge/publisher.go`
- เปลี่ยน default topic: `events.normalized.v1` → `gw.events.normalized.v1`

```go
// เดิม
topic = "events.normalized.v1"
// ใหม่
topic = "gw.events.normalized.v1"
```

#### `internal/services/workspacesvc/service.go` (line ~404)
- เปลี่ยน default topic: `events.workspace.provisioned.v1` → `gw.workspace.provisioned.v1`

```go
// เดิม
topic = "events.workspace.provisioned.v1"
// ใหม่
topic = "gw.workspace.provisioned.v1"
```

#### `config/kafka.go` (ensure list)
- อัปเดต defaults ใน `topicsWithDefaults`:

```go
// เดิม
{"KAFKA_TOPIC_EVENTS_NORMALIZED", "events.normalized.v1"},
{"KAFKA_TOPIC_WORKSPACE_PROVISIONED", "events.workspace.provisioned.v1"},

// ใหม่
{"KAFKA_TOPIC_EVENTS_NORMALIZED", "gw.events.normalized.v1"},
{"KAFKA_TOPIC_WORKSPACE_PROVISIONED", "gw.workspace.provisioned.v1"},
// เพิ่ม gw topics ให้ ensure ด้วย
{"KAFKA_TOPIC_GW_ASSETS", "gw.assets.changed.v1"},
{"KAFKA_TOPIC_GW_SOURCES", "gw.sources.changed.v1"},
{"KAFKA_TOPIC_GW_DELIVERY_STATUS", "gw.delivery.status.v1"},
```

#### `.env.example`
- เพิ่ม entries สำหรับ gw topics:

```env
KAFKA_TOPIC_EVENTS_NORMALIZED=gw.events.normalized.v1
KAFKA_TOPIC_WORKSPACE_PROVISIONED=gw.workspace.provisioned.v1
KAFKA_TOPIC_GW_ASSETS=gw.assets.changed.v1
KAFKA_TOPIC_GW_SOURCES=gw.sources.changed.v1
KAFKA_TOPIC_GW_DELIVERY_STATUS=gw.delivery.status.v1
```

---

### 1B. klynx-api-feature

#### `config/kafka.go` (ensure list)
- เปลี่ยน env var names + defaults ทั้งหมด:

```go
// เดิม
{"KAFKA_TOPIC_PHIBEK_NORMALIZED",              "phibek.events.normalized.v1"},
{"KAFKA_TOPIC_PHIBEK_ASSETS",                  "phibek.assets.changed.v1"},
{"KAFKA_TOPIC_PHIBEK_SOURCES",                 "phibek.sources.changed.v1"},
{"KAFKA_TOPIC_PHIBEK_DELIVERY_STATUS",         "phibek.delivery.status.v1"},
{"KAFKA_TOPIC_PHIBEK_WORKSPACE_PROVISIONED",   "events.workspace.provisioned.v1"},

// ใหม่
{"KAFKA_TOPIC_GW_NORMALIZED",              "gw.events.normalized.v1"},
{"KAFKA_TOPIC_GW_ASSETS",                  "gw.assets.changed.v1"},
{"KAFKA_TOPIC_GW_SOURCES",                 "gw.sources.changed.v1"},
{"KAFKA_TOPIC_GW_DELIVERY_STATUS",         "gw.delivery.status.v1"},
{"KAFKA_TOPIC_GW_WORKSPACE_PROVISIONED",   "gw.workspace.provisioned.v1"},
```

#### Rename consumer packages

| เดิม | ใหม่ |
|---|---|
| `internal/kafka/phibekconsumer/` | `internal/kafka/gweventscons/` |
| `internal/kafka/phibekassetcons/` | `internal/kafka/gwassetcons/` |
| `internal/kafka/phibeksourcecons/` | `internal/kafka/gwsourcecons/` |
| `internal/kafka/phibekdeliverycons/` | `internal/kafka/gwdeliverycons/` |
| `internal/kafka/workspaceprovcons/` | (ชื่อ ok — แค่เปลี่ยน env var ใน consumer.go) |

ใน consumer.go แต่ละไฟล์ เปลี่ยน:
- `os.Getenv("KAFKA_TOPIC_PHIBEK_*")` → `os.Getenv("KAFKA_TOPIC_GW_*")`
- default string `"phibek.*"` → `"gw.*"`
- consumer group ID: `"klynx-phibek-*-grp"` → `"klynx-gw-*-grp"`

#### `main.go`
- อัปเดต import paths ที่ใช้ phibek* packages
- อัปเดต function call names ถ้าเปลี่ยน

---

## Phase 2 — Deploy & migrate

1. **Deploy gateway-api** ใหม่พร้อม env vars ชี้ไปที่ `gw.*` topics
2. **Deploy klynx-api** ใหม่พร้อม `KAFKA_TOPIC_GW_*` env vars
3. ตรวจสอบ consumer lag ใน Kafkat UI — `gw.*` topics ควรมี consumer active
4. ตรวจสอบ `normalized.events` → `gw.events.normalized.v1` flow ว่า event ไหลถึง klynx
5. หลังยืนยัน 24h → ดำเนินการ Phase 3

---

## Phase 3 — Topic cleanup

### phibek cluster — ลบได้ทันที (0 messages, no active consumers)

| Topic | เหตุผล |
|---|---|
| `phibek.events.normalized.v1` | renamed → `gw.events.normalized.v1` |
| `phibek.assets.changed.v1` | renamed → `gw.assets.changed.v1` |
| `phibek.sources.changed.v1` | renamed → `gw.sources.changed.v1` |
| `phibek.delivery.status.v1` | renamed → `gw.delivery.status.v1` |
| `phibek.workspace.provisioned.v1` | renamed → `gw.workspace.provisioned.v1` |
| `events.workspace.provisioned.v1` | รวมเข้า `gw.workspace.provisioned.v1` |

### aliza cluster — ต้องตรวจสอบก่อนลบ

| Topic | สถานะ | Action |
|---|---|---|
| `ata.events-aisom` | stale — `KAFKA_TOPIC_ATA=ata.events-aisom` ใน `.env.dev` เป็นค่าเก่า ย้ายไปใช้ `gw.events` แล้ว แก้ค่าเป็น `ata.events` แล้ว | ลบ topic ออกจาก cluster ได้ |
| `events.workspace.provisioned.v1` | มีใน aliza ด้วย | ลบหลังยืนยัน migrate แล้ว |
| `events.normalized.v1` | อาจมี service อื่นใน aliza ใช้งาน | **ตรวจสอบก่อนลบ** — อย่า assume |

---

## Topics ที่ไม่แตะ (active / internal)

### phibek cluster
`raw.events`, `normalized.events`, `kwatch4g.iwown`, `result`, `tp.detection`

### aliza cluster
`ata.events`, `ata.events-feature`, `authz.relationship.updated`, `events.delivery.v1`,
`gw.workspace.provisioned.v1`, `iboc`, `kalert`, `kcontrol.*`, `kdetect`, `klive.player`,
`ksearch.*`, `kwatch.*`, `klynx.org.created.v1`, `klynx.org.deleted.v1`, `klynx.entitlement.snapshot.v1`

---

## Checklist

### gateway-api
- [x] `internal/eventbridge/publisher.go` — เปลี่ยน default topic
- [x] `internal/services/workspacesvc/service.go` — เปลี่ยน default topic + comment
- [x] `internal/eventschema/workspace.go` — อัปเดต comment
- [x] `config/kafka.go` — อัปเดต ensure list
- [x] `.env.example` — เพิ่ม gw topic env vars

### klynx-api-feature
- [x] `config/kafka.go` — เปลี่ยน env var names + defaults
- [x] `internal/kafka/phibekconsumer/` → renamed → `internal/kafka/gweventscons/`
- [x] `internal/kafka/phibekassetcons/` → renamed → `internal/kafka/gwassetcons/`
- [x] `internal/kafka/phibeksourcecons/` → renamed → `internal/kafka/gwsourcecons/`
- [x] `internal/kafka/phibekdeliverycons/` → renamed → `internal/kafka/gwdeliverycons/`
- [x] `internal/kafka/workspaceprovcons/consumer.go` — อัปเดต env var + consumer group ID
- [x] `main.go` — อัปเดต imports + references
- [x] `internal/services/authzsvc/org.go` — อัปเดต comment
- [x] `.env.dev` — อัปเดต env var overrides

### Deployment
- [ ] Deploy gateway-api → ตรวจสอบ `gw.*` topics มี messages
- [ ] Deploy klynx-api → ตรวจสอบ consumer lag = 0
- [ ] Monitor 24h
- [ ] ลบ `phibek.*` topics จาก phibek cluster
- [ ] ตรวจสอบ + ลบ stale topics ใน aliza cluster
