# Gateway (SaaS Tenancy) — Step 2 Implementation Plan (Monolith + DI Style)

> เป้าหมายของ Step 2: ทำ “Control Plane” ให้ครบ (Org/OU/Targets/Permission Profile) และเริ่ม “Data Plane” (Ingest → Kafka → Normalize → Store → Distribute) แบบที่ user เห็นของไว โดยสลับทำ BE/FE เป็นช่วงๆ

## Scope & Non-goals

### In scope (ต้องทำใน Step 2)
- Auth (มีแล้ว): Keycloak (Local/Google), JWT verify local
- AuthZ (มีแล้ว): Permify schema + check
- Tenancy:
  1) Create Org (tenant boundary)
  2) Org Events Ingest Endpoint `/events/{orgId}` (hot path, no JWT)
  3) Create OrgUnit (OU tree)
  4) Create Delivery Targets (Webhook / Line OA / Telegram / Discord) = “resource” สำหรับ distribute
  5) Create Permission Profiles เพื่อ assign OU + Targets (และ menu/api access ถ้าพร้อม)
- Data Plane (MVP):
  - Ingest REST → Kafka raw.events
  - Normalization consumer → canonical metadata (Mongo) + binaryRefs → S3
  - Distributor consumer → enqueue delivery tasks → workers send webhook + retry + DLQ
- File Proxy API `/files/*` (JWT + Permify) สำหรับดาวน์โหลด binary

### Out of scope (เลื่อนไปหลังได้)
- resourceServiceAccount (คุณบอกไว้ที่หลัง)
- MQTT adapter (optional) — ทำหลัง REST stable
- Advanced reconciliation tooling (ทำขั้นต่ำก่อน)

---

## Hard truth / Risk Callouts (จำกัด ​rete limit  ใชเ redis ช่วยไหม)
### 1) `/events/{orgId}` ไม่ตรวจ token = “public write endpoint”
คุณยืนยันว่า “ไม่ตรวจ token ใดๆ” เพราะ device/customize จำกัด  
**โอเค แต่ต้องมี guard อย่างน้อย 1 ชั้น** ไม่งั้นโดนยิง spam จน Kafka/Mongo/S3 ตายได้จริง
- + **Rate limit ต่อ org + ต่อ IP** (ต้องมีเสมอ)

> ถ้าคุณ “เอาโล่งจริงๆ” ให้ทำ rate limit + payload size limit + WAF อย่างน้อย ไม่งั้น 10k TPS จะกลายเป็น 0 TPS เพราะโดน flood

---

## Architecture (Monolith แต่แบ่งโมดูลชัด)
- API server (Fiber) = control plane + ingest endpoint
- Background workers (ใน repo เดียวกัน) = normalize + distribute
- Shared libs = kafka/mongo/s3/redis/authz/auth

### Process layout
- `cmd/api` : Fiber server
- `cmd/worker-normalize` : consume raw.events → canonical
- `cmd/worker-delivery` : consume canonical.events → enqueue → send
> ถ้าจะ “monolith process เดียว” ก็ทำ `cmd/gateway` แล้ว run goroutines 3 ตัวได้ แต่ production/debug ยากกว่า (แนะนำแยก cmd)

---

## Data Model (MVP)
### Mongo Collections
- `organizations`
- `orgUnits`
- `deliveryTargets` (webhook/line/telegram/discord)
- `permissionProfiles` (assign OU + targets + relations)
- `canonicalEvents` (metadata)
- `deliveryTasks` (optional ถ้า queue อยู่ใน Redis)
- `auditLogs` (มีแล้ว)

### Redis
- rate limit counters
- hot cache routing (orgId → target list / config)
- delivery queue (ถ้าเลือก Redis queue)

### S3
- binary objects ตาม key pattern:
  - `org/{orgId}/events/{yyyy}/{mm}/{dd}/{eventId}/{objectId}`

---

## Permify Modeling (แนวทางให้ไม่พังภายหลัง)
> หลัก: **entity type ต้องจำแนกชัด** เพื่อไม่ให้ check/lookup วิ่งกว้าง

แนะนำ entity type:
- `organization`
- `orgUnit`
- `target` (delivery target)
- `menu` (ถ้าทำ)
- `api` (ถ้าทำ)
- (event ไม่จำเป็นต้องเป็น entity ถ้า query ผ่าน Mongo แล้ว filter ด้วย orgId + permission)

ความสัมพันธ์หลัก:
- `organization#admin @user`
- `organization#member @user`
- `orgUnit#parent @orgUnit`
- `orgUnit#member @user`
- `orgUnit#admin @user`
- `target#viewer @orgUnit`
- `target#editor @orgUnit`
- `target#deleter @orgUnit`
- `target#parentOrg @organization`

> อย่าทำ “resource:camera, resource:kcontrol …” ถ้าไม่ได้ใช้จริงใน control plane ตอนนี้  
> เอาให้ deliver target + file access + menu access ทำงานก่อน

---

## API Modules & Endpoints (Step 2)

### Conventions
- Base: `/api/v1`
- Headers:
  - `Authorization: Bearer <token>` (control plane)
  - `X-Active-Org: <orgId>` (scoped endpoints)
- Response format: ใช้ pattern เดิมของคุณ
  - `{ "code": "SUCCESS|...", "message": "...", "status": true|false, "details": any }`

---

# 1) Auth (Done)
- `/auth/signin`, `/auth/refreshToken`, `/auth/signout`, `/auth/register`, `/auth/google` etc.

---

# 2) Organization (Control Plane)
## 2.1 Create Org
- `POST /api/v1/orgs`
- Body:
  - `name` (required)
  - `description` (optional)
- Behavior:
  - create org in Mongo
  - bootstrap Permify tuples: org admin/member
  - create root OU automatically (recommended)
  - generate `ingestConfig` (see below)

## 2.2 List Orgs (already)
- `GET /api/v1/orgs`

## 2.3 Org ingest config (new)
> ต้องมี endpoint ให้ FE/owner ดู ingest endpoint + secret (ครั้งเดียว)  
- `POST /api/v1/orgs/:id/ingest/rotateSecret` (admin only)
- `GET /api/v1/orgs/:id/ingest` (admin only)
- Fields:
  - `ingestEndpoint`: `/events/{orgId}` หรือ `/events/{orgId}/{ingestKey}`
  - `signatureRequired`: bool
  - `ingestSecretMasked`: string (แสดงบางส่วน)
  - `rateLimit`: `{ perSecond, burst }`

> ถ้าคุณไม่เอา secret เลย: ก็ยังควรมี `rateLimit` per org และ `payloadMaxBytes`

---

# 3) OrgUnit (OU Tree)
## 3.1 Create OU
- `POST /api/v1/orgs/units`
- Header: `X-Active-Org`
- Body:
  - `name`
  - `parentUnitId` (optional; null = root child)
- Permission:
  - org.manage หรือ unit.manage ที่ parent

## 3.2 Tree
- `GET /api/v1/orgs/units/tree`
- Response: nested tree

## 3.3 Update / Delete
- `PATCH /api/v1/orgs/units/:id`
- `DELETE /api/v1/orgs/units/:id`
- Guard: delete ได้เมื่อไม่มี child + ไม่มี bindings (members/targets)

## 3.4 Assign members to OU
- `POST /api/v1/orgs/units/:id/members`
- `PATCH /api/v1/orgs/units/:id/members` (remove)
- `GET /api/v1/orgs/units/:id/members`

---

# 4) Delivery Targets (Webhook / Line / Telegram / Discord)
> นี่คือ “resource” ของการส่งต่อ event

## 4.1 CRUD Targets
- `POST /api/v1/orgs/targets`
- `GET /api/v1/orgs/targets`
- `GET /api/v1/orgs/targets/:id`
- `PATCH /api/v1/orgs/targets/:id`
- `DELETE /api/v1/orgs/targets/:id`

### Target schema (MVP)
- `targetId`
- `orgId`
- `type`: `webhook|line|telegram|discord`
- `name`
- `enabled`
- `config`:
  - webhook: `{ url, signingEnabled, signingSecret, headers, timeoutMs }`
  - line: `{ channelAccessTokenRef, to }` (อย่าเก็บ token plain ถ้าเลี่ยงได้)
  - telegram: `{ botTokenRef, chatId }`
  - discord: `{ webhookUrl, signingEnabled? }`

> แนะนำ: เก็บ secret เป็น reference ไป secret store ภายหลัง ตอนนี้ถ้าจำเป็นเก็บใน Mongo ให้ encrypt-at-rest (KMS) ไม่งั้นเป็นหนี้ความปลอดภัยทันที

## 4.2 Bind targets to OU (access control)
- `GET /api/v1/orgs/units/:unitId/targets` (list accessible/assigned)
- `POST /api/v1/orgs/units/:unitId/targets` (bulk assign)
- `PATCH /api/v1/orgs/units/:unitId/targets` (bulk remove)
- Body:
  - `targetIds: []`
  - `relations: ["viewer"|"editor"|"deleter"]` (default viewer)

---

# 5) Permission Profiles (OU + Targets + Menu/API)
คุณมีของ `menuPermissionProfile` และ `resourcePermissionProfile` แล้ว  
Step 2 ทำให้ “ใช้ได้จริง” โดย FE เอาไป assign แล้ว permission check ผ่าน Permify ได้

## 5.1 TargetPermissionProfile (ใหม่หรือใช้ resourcePermissionProfile เดิม)
- `POST /api/v1/orgs/target/permissions`
- `GET /api/v1/orgs/target/permissions`
- `GET /api/v1/orgs/target/permissions/:id`
- `PATCH /api/v1/orgs/target/permissions/:id`
- `DELETE /api/v1/orgs/target/permissions/:id`

Fields:
- `name`, `description`, `status`
- `orgUnitIds: []`
- `targetIds: []`
- `relations: ["viewer","editor","deleter"]`

Behavior when Create/Update:
- write Mongo profile
- write Permify tuples:
  - for each (targetId, unitId, relation): `target:targetId#relation@orgUnit:unitId`

## 5.2 Member Access endpoints (already pattern)
- `GET /api/v1/orgs/target/access` → list targets caller can view/edit/delete
- `GET /api/v1/orgs/menu/access` → list menus caller can see
> FE sidebar ใช้ endpoint นี้ได้เลย

---

# 6) Data Plane — Ingest / Normalize / Distribute

## 6.1 Ingest REST (NO JWT hot path)
- `POST /events/{orgId}`  (outside `/api/v1` เพื่อไม่โดน middleware auth/audit หนัก)
- Request:
  - headers: optional signature / timestamp / contentType
  - body: vendor-specific JSON (ไม่ต้อง canonical)
- Response:
  - `202 Accepted` + `{ eventId, receivedAt }`

**Ingest responsibilities (ต้องเบาที่สุด):**
- validate org exists + enabled (cache in Redis)
- apply rate limit (Redis)
- assign `eventId` + `receivedAt`
- produce Kafka `raw.events`:
  - key: `orgId`
  - value: `{ orgId, eventId, receivedAt, rawBody, headersSubset, sourceIp }`
- DONE (no Mongo, no S3, no Permify)

> อย่าใส่ “normalize” ใน handler เด็ดขาด ไม่งั้น latency กระโดดทันที

## 6.2 Normalize Worker
Consume `raw.events` → produce `canonical.events` + persist:
- parse raw
- map vendor → canonical schema v1
- extract binary if exists:
  - ถ้า payload ส่ง base64: upload to S3 แล้วแทนเป็น `binaryRefs`
- save metadata to Mongo `canonicalEvents`
- publish `canonical.events` Kafka (small message only)

## 6.3 Distributor Worker
Consume `canonical.events`:
- resolve routing targets by orgId (Redis cached)
- enqueue tasks:
  - Redis stream/list OR Kafka `delivery.tasks` (เลือก 1)
- workers send:
  - POST webhook
  - optional HMAC signing
  - retry exponential backoff
  - DLQ after max retries
- audit delivery outcome (minimal)

---

# 7) Event Query & Recovery API (MVP)
> เพื่อให้ partner “recovery” ได้โดยไม่ต้อง replay Kafka

- `GET /api/v1/orgs/events`
  - filters: `from`, `to`, `eventType`, `deviceId`, `ownerService`
- permission: org.view หรือ OU mapping ตาม design (ถ้ายังไม่พร้อม ใช้ org scope ก่อน)

---

# 8) File Proxy API
- `GET /api/v1/files/:objectId`
- JWT required
- Permify check: user has view on target/resource หรือ org/view (MVP)
- stream from S3

---

## DI Style (Monolith wiring)
คุณทำถูกแล้ว: router รับ `*app.Container` ไม่ new เอง

### Container (ตัวอย่าง)
- controllers:
  - OrgController, OrgUnitController, TargetController
  - TargetPermissionProfileController, MenuPermissionProfileController
  - MemberAccessController
  - EventQueryController, FileProxyController
- services:
  - OrganizationService, OrgUnitService
  - TargetService, PermissionProfileService
  - IngestService (thin), EventQueryService, FileService
- gateways:
  - auth (keycloak jwks verifier), authz (permify hybrid)
  - kafka producer/consumer, redis, s3
- repos:
  - orgRepo, orgUnitRepo, targetRepo, profileRepo, eventRepo, auditRepo

---

## FE Plan (Svelte HUD) — ทำให้ user “เห็นของ” เร็ว
### Pages (ลำดับทำ)
1) Org Switcher + Create Org modal
2) OU Tree (CRUD + assign members)
3) Targets (CRUD) + Bind to OU
4) Permission Profiles (Targets + Menus)
5) “My Access” debug view (โชว์ menus/targets ที่ user เห็น)

### FE API dependencies (ต้องมีเพื่อ unlock UI)
- Org list/create
- OU tree endpoints
- Target list/create/update/delete
- Assign targets to OU
- Access endpoints (my menu/target access)

---

## Delivery Strategy (สลับทำ BE/FE แบบที่คุณต้องการ)
### Sprint A (BE core) — 1-2 วันทำงานจริง
- Target CRUD + OU bind
- TargetPermissionProfile CRUD + apply Permify tuples
- `/events/{orgId}` ingest MVP → Kafka raw.events

### Sprint B (FE visible)
- Org switcher + OU tree UI
- Targets UI + bind UI
- My Access UI (อ่านจาก `/access`)

### Sprint C (Workers)
- normalize worker MVP → Mongo + S3 + canonical.events
- distributor worker MVP → webhook only + retry + DLQ

### Sprint D (polish)
- event query + file proxy
- audit coverage และ metrics (ingest latency, kafka lag, delivery success rate)

---

## Performance & Reliability Must-haves (อย่าข้าม)
- Ingest:
  - max body size (เช่น 256KB/1MB)
  - timeout ต่ำ
  - rate limit ต่อ org + ip
  - redis cache org config (TTL 30-60s)
- Kafka:
  - key partition by orgId
  - async producer with backpressure handling
- Mongo:
  - index: `orgId + occurredAt`, `orgId + eventType`, `orgId + source.deviceId`
- S3:
  - upload concurrency control
- Delivery:
  - idempotency key per (eventId,targetId)
  - retry policy config per target
  - DLQ viewer/admin endpoint (ภายหลัง)

---

## “Definition of Done” (Step 2)
- User สร้าง Org → สร้าง OU → สร้าง Target → สร้าง Profile → assign → user เห็นเมนู/targets ตามสิทธิ์
- Third party POST `/events/{orgId}` แล้วเห็น event เข้า pipeline:
  - raw.events มี message
  - canonicalEvents มี record
  - webhook ส่งออกสำเร็จอย่างน้อย 1 target
- ไม่มี auth ใน hot path แต่มี guard (rate limit + payload limit อย่างน้อย)

---

## Next concrete actions (เริ่มโค้ดต่อทันที)
1) Implement module: `targets` (repo/service/controller/router)
2) Implement module: `targetPermissionProfiles` (write tuples to Permify)
3) Implement ingest endpoint + redis rate limit + kafka producer
4) FE: Targets page + bind to OU + Access page

> ถ้าคุณอยาก “เริ่ม Step 2” แบบไม่สะดุด: เริ่มจาก Targets + Profiles ก่อน เพราะมัน unlock UI และ validate Permify schema ได้เร็วสุด