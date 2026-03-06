# Gateway API Baseline Architecture

This document defines the **baseline architecture and development
standards** for the Gateway API platform.

------------------------------------------------------------------------

# Core Principles
Baseline Rule File Path Comment
All file .go Must be comment path of first line 
Plattern
// controllers/grpapi/create.go
package grpapi
Or
// internal/services/eventsvc/create.go
package eventsvc


Architecture follows strict dependency flow:

repo → service → controller → router

Rules:

  Layer        Responsibility
  ------------ -------------------------------
  router       route definition + middleware
  controller   HTTP layer
  service      business logic
  repo         database access

Forbidden:

controller → repo\
router → repo\
repo → service

------------------------------------------------------------------------

# API Response Standard

``` json
{
  "code": "SUCCESS",
  "message": "events fetched successfully",
  "status": true,
  "details": [],
  "pagination": {
    "page": 1,
    "perPages": 10,
    "totalRecords": 0,
    "totalPages": 0,
    "sortField": "createdAt",
    "sortOrder": "desc"
  }
}
```

Rules:

-   Always use `details`
-   Never use `detail`
-   pagination optional

------------------------------------------------------------------------

# Query Parameters

Standard parameters:

page=1\
perPages=10\
sortField=createdAt\
sortOrder=desc\
search=`<regex by name>`{=html}\
dateTime=from,to

Example:

/api/v3/events?page=1&perPages=10&search=camera

Mongo query:

``` js
{ name: { $regex: search, $options: "i" } }
```

------------------------------------------------------------------------

# DateTime Standard

RFC3339 UTC

Example

2026-01-20T16:59:59Z

------------------------------------------------------------------------

# MongoDB Naming

Collection naming: snake_case

Examples

event_management\
canonical_events\
subscriptions\
audit_logs

Fields use camelCase

Deprecated fields:

dateTimeCreate\
dateTimeUpdate\
long

Replace with

createdAt\
updatedAt\
lng

------------------------------------------------------------------------

# Headers

Required headers

Authorization: Bearer `<jwt>`{=html}\
X-Active-Org: `<orgId>`{=html}

------------------------------------------------------------------------

# Logging Strategy

  Level   Usage
  ------- ----------------------
  TRACE   deep debugging
  DEBUG   development
  INFO    lifecycle milestones
  WARN    recoverable errors
  ERROR   runtime failures

Development workflow

development → TRACE\
stabilized → DEBUG\
production → INFO

------------------------------------------------------------------------

# Audit Logging

Example document

``` json
{
  "actorType": "user",
  "actorId": "unknown",
  "actorName": "anonymous",
  "actorIp": "172.16.1.108",
  "operation": "authSignin",
  "resource": "/auth/signin",
  "method": "POST",
  "status": 401,
  "payload": {
    "username": "admin",
    "password": "***"
  },
  "response": {
    "message": "AUTH_FAILED",
    "details": "unauthorized",
    "status": false,
    "code": "UNAUTHORIZED"
  },
  "latencyMs": 98,
  "traceId": "da075a44ae199f022b02c1d6b1ffa6b5",
  "createdAt": "2026-03-04T08:34:14Z"
}
```

Sensitive fields must be masked.

Audit only for

POST\
PUT\
PATCH\
DELETE

------------------------------------------------------------------------

# Storage Architecture

  Storage   Purpose
  --------- -----------------------
  MongoDB   metadata / raw events
  Redis     cache
  S3        binary storage
  Kafka     event streaming

------------------------------------------------------------------------

# Event Pipeline

external device\
→ POST /events/:orgId\
→ event_management (pending)\
→ approve device\
→ Kafka raw.events\
→ normalizer\
→ canonical_events (MongoDB)\
→ S3 binary storage\
→ delivery workers\
→ webhook / retry / DLQ

------------------------------------------------------------------------

# C4 Container Diagram

``` mermaid
flowchart LR
    Device -->|HTTP Events| GatewayAPI
    GatewayAPI --> MongoDB
    GatewayAPI --> Redis
    GatewayAPI --> Kafka

    Kafka --> Normalizer
    Normalizer --> MongoDB
    Normalizer --> S3

    MongoDB --> Distributor
    Redis --> Distributor

    Distributor --> WebhookWorker
    WebhookWorker --> ExternalSystems
```

------------------------------------------------------------------------

# Kafka Event Pipeline

``` mermaid
flowchart TD
    Device --> IngestAPI
    IngestAPI --> PendingEvents
    PendingEvents --> ApproveDevice
    ApproveDevice --> KafkaRaw

    KafkaRaw --> Normalizer
    Normalizer --> CanonicalEvents
    Normalizer --> BinaryExtractor
    BinaryExtractor --> S3

    CanonicalEvents --> DeliveryQueue
    DeliveryQueue --> WebhookWorker
    WebhookWorker --> ExternalSystem
    WebhookWorker --> RetryQueue
    RetryQueue --> WebhookWorker
    RetryQueue --> DLQ
```

------------------------------------------------------------------------

# Security Flow (Keycloak + Permify)

``` mermaid
flowchart LR
    User --> Frontend
    Frontend --> Keycloak
    Keycloak -->|Auth Code| Gateway
    Gateway -->|Token Exchange| Keycloak
    Gateway --> JWTVerify

    JWTVerify --> Permify
    Permify --> Gateway

    Gateway --> ProtectedAPI
```

## MongoDB Index Bootstrap (มาตรฐานบังคับ)

ถ้าต้องทำ index / unique index / TTL index **ห้าม** ไปเรียก `EnsureIndexes()` แบบกระจาย ๆ ใน router/controller/service  
ให้ทำเป็น **bootstrap** ตอน start app เท่านั้น

### หลักการ
- ✅ Source of truth: Repo เป็นเจ้าของ index ของ collection ตัวเอง (`EnsureIndexes(ctx)`)
- ✅ Bootstrap register: `config.RegisterMongoBootstrap(fn)` เก็บรายการงาน bootstrap แล้ว `config.InitMongo()` เป็นคนเรียก `runMongoBootstraps()`
- ✅ One-time only: เรียกครั้งเดียวตอน start (idempotent)
- ❌ ห้าม `init()` ใน router (router ไม่ควรทำ side-effect ที่แตะ infra)
- ❌ ห้ามสร้าง repo ใหม่ใน router เพื่อทำ index

### โครงสร้างไฟล์ที่แนะนำ
- `internal/repo/<domain>repo/mongoBootstrap.go`  
  register bootstrap ของ repo ใน package เดียวกัน (ใกล้ index ที่สุด)
- หรือถ้าอยาก “ชัด” กว่า: รวมไว้ที่ `internal/app/bootstrap/mongo.go` แล้วเรียก `EnsureIndexes()` ทั้งหมดจากที่เดียว ** ย้ายมาทำแบบนี้ได้เลย

### ตัวอย่าง (แนะนำ) — bootstrap อยู่ใน repo package

```go
// internal/repo/authzrepo/mongoBootstrap.go
package authzrepo

import (
  "context"

  "github.com/hotkhwan/gateway-api/config"
)

// NOTE: bootstrap นี้ถูกเรียกจาก config.InitMongo() ผ่าน runMongoBootstraps()
func init() {
  config.RegisterMongoBootstrap(func(ctx context.Context) error {
    if err := NewOrgRepo(config.DB).EnsureIndexes(ctx); err != nil {
      return err
    }
    if err := NewOrgUnitRepo().EnsureIndexes(ctx); err != nil {
      return err
    }
    // เพิ่ม repo อื่น ๆ ได้ที่นี่
    return nil
  })
}
```

### ป้องกัน “init order” พัง
- ให้ `config.InitMongo()` ถูกเรียกก่อน `InitHTTP()` เสมอใน `main.go`
- ให้ bootstrap อยู่ใน package ที่ถูก import แน่นอนตอน start  
  (เช่น import repo packages ผ่าน container wiring หรือ import แบบ side-effect ใน bootstrap layer)

### TTL / retention index (ถ้าต้องใช้)
- TTL index เหมาะกับ `raw.events`/`event_management` ถ้าจะล้าง auto
- TTL ควรถูกประกาศใน `EnsureIndexes()` เท่านั้น และต้องระบุ UTC



## Shared Models & Helpers (Response + Errors)

### `models/gmod` response helpers
- **Canonical API response**: prefer `gmod.SendPaginationOK` (or equivalent) to keep consistent envelope:
  ```json
  { "code": "SUCCESS", "message": "ok", "status": true, "details": [], "pagination": { } }
  ```
- `SendPagination` (generic `PaginationDbResponse`) is OK internally แต่ถ้าภายนอก/FE consume แล้ว ให้ lock เป็น pattern เดียว (`code/message/status/details/pagination`) เพื่อหลีกเลี่ยง breaking change

### `models/gmod` coded errors
- ใช้ `gmod.CodedError` / `gmod.Errorf(code, msg)` เพื่อให้ service layer ส่ง “code” กลับมาที่ controller ได้ตรง
- `gmod.ErrCodeOf(err)` ใช้ map error → response code ได้โดยไม่ต้อง `switch err.Error()` (ลด string-compare anti-pattern)

### `utils/httputil` error helpers
- `httputil.FailReason(...)` คือรูปแบบที่แนะนำ: **code กลาง** + **reason เฉพาะ** ใน `details.reason`
- ไม่ log ซ้ำในทุกชั้น: ให้ log ที่ boundary (controller/service) แล้ว return error/response ให้ตรง

---

## Pagination Contract

### Query param: `perPages`
- **Default**: 10 (ถ้าไม่ส่ง หรือส่ง <= 0)
- **Clamp**: จำกัดบนด้วย `PERPAGE_MAX` (default 250) ผ่าน `utils.PerPage(perPages)`
- เหตุผล: ลด DoS-by-pagination + คุม query cost

> Appendix A: “perPages vs perPage”  
> ตอนนี้คุณใช้ `perPages` แล้ว → ใช้ต่อได้ แต่ต้อง enforce ทั้ง BE+FE ให้ตรงกัน

---

## Search Contract (Name Regex)

- Query param `search` ให้ default behavior เป็น **regex by name** (case-insensitive)
- Base rule: `name` คือ field หลักที่ใช้ค้นหา (ถ้าจำเป็นค่อยเพิ่ม field อื่นภายหลัง เช่น `email`, `deviceKey`)
- Mongo example (safe-ish): ใช้ `primitive.Regex{Pattern: escaped, Options: "i"}` และ escape input ก่อนเสมอ  
  *อย่าปล่อย raw regex ให้ client ใส่เอง ถ้าไม่จำเป็น (ReDoS risk)*

---

## Repo Adapters (Mongo / S3)

### `internal/adapters/repo/stomongo`
แนวคิด: เป็น “thin wrapper” ที่:
- ลด boilerplate ของ `Find/Update/Bulk/Tx`
- ทำ default behavior ให้ consistent เช่น `updatedAt` ต้องเป็น UTC (`nowSet`)
- รวม pagination helper (`FindWithPagination`/`FindPaginated`)

ข้อควรระวัง:
- `UpdateOneOps` ถ้ามีแค่ `$unset` อย่างเดียว จะไม่ touch `updatedAt` → ใช้ `UpdateOneUnsetOnlyTouch` หรือใส่ `$set.updatedAt` เอง
- อย่าให้ wrapper กลายเป็น “ORM” ที่ซ่อน query cost / index hint มากเกินไป

### `internal/repo/stos3minio`
แนวคิด: รวม behavior มาตรฐานสำหรับ:
- `Upload/UploadWithMeta`
- `PresignOnce/PresignMany` (expiry จาก ENV `S3_EXPIRY`, default 1m)
- `DeleteByKey` (normalize key + stat ก่อน/หลังเพื่อ debug)

Security note:
- presigned URL สำหรับ private file ต้อง enforce org ownership ก่อนสร้าง URL เสมอ
- อย่า expose `bucket/key` แบบเดาง่ายให้ข้าม org ได้

---

## Mongo Index Bootstrap

### Rule
- **สร้าง index ผ่าน bootstrap เท่านั้น** (ตอน start service)
- Repo แต่ละตัวมี `EnsureIndexes(ctx)` แล้ว register ที่เดียวผ่าน `config.RegisterMongoBootstrap(...)`

### Pattern
```go
func init() {
  config.RegisterMongoBootstrap(func(ctx context.Context) error {
    if err := authzrepo.NewOrgRepo(config.DB).EnsureIndexes(ctx); err != nil { return err }
    if err := authzrepo.NewOrgUnitRepo().EnsureIndexes(ctx); err != nil { return err }
    return nil
  })
}
```

ข้อดี:
- ไม่กระจาย index creation ไปทั่ว (ลด surprise)
- run once, deterministic, test ง่าย
