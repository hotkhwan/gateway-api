Subscription.md
1) Goals & Non-goals
Goals

มี subscription packet 3 ระดับ: Freemium / Pro / Enterprise

enforce limits ที่ ingest hot-path:

maxPayloadBytes

perOrgPerSec (+ burst)

perIpPerMin

storageQuotaBytes (ใช้ตอน persist object/event ไม่ใช่ตอนรับเข้า)

maxOrganizationsPerTenant (tenancy)

รองรับการเปิดใช้:

Freemium: active ทันที (live time)

Pro: เตรียมไว้ “activate ภายหลัง” (billing/credit card)

Enterprise: activate ด้วย licenseKey

ใช้ DI style เดิม: repo -> service -> controller/router, ไม่ static call

มี mongoBootstrap สำหรับ index + uniqueness กัน data พังใน prod

cache/ratelimit fail strategy ชัดเจน (ไม่ใช่สุ่ม fail-open ทุกอย่าง)

Non-goals (ตอนนี้)

ไม่ทำ billing จริงในเฟสนี้ (Stripe/Omise ฯลฯ)

ไม่ทำ deny policy ซับซ้อน (ยังคง additive/monotonic style)

ไม่ทำ distributed quota reconciliation (ทำ baseline ที่ปลอดภัยก่อน)

2) Subscription Packets (Source of Truth)
Plan limits (ค่าตั้งต้น)

ใช้ bytes สำหรับ quota/storage เพื่อคำนวณง่าย

Freemium

orgCacheTTL: 30s

maxPayloadBytes: 1 * 1024 * 1024 (1MB) ✅ (คุณเขียน 10MB แต่ตัวเลขคือ 1MB — เลือกให้ชัด)

perOrgPerSec: 10

perIpPerMin: 300

storageQuotaBytes: 10GB

maxOrganizationsPerTenant: 2

billingCycle: none (live)

Pro (เตรียมไว้)

orgCacheTTL: 90s

maxPayloadBytes: 40MB (แนะนำ 40 * 1024 * 1024) ✅ (อย่าใช้ 20242024 มันไม่ใช่ 40MB*)

perOrgPerSec: 100

perIpPerMin: 3000

storageQuotaBytes: 100GB

maxOrganizationsPerTenant: 5

billingCycle: monthly / quarterly / yearly

Enterprise

custom limits (per contract)

maxOrganizationsPerTenant: 10 (default)

activate ด้วย licenseKey

billingCycle: monthly / quarterly / yearly

3) Data Model (Mongo)
3.1 collections
subscriptionPlans (optional แต่แนะนำ)

เก็บ template ของ plan (freemium/pro/enterprise) เพื่อไม่ hardcode กระจายหลายไฟล์

{
  "id": "freemium",
  "name": "Freemium",
  "limits": {
    "orgCacheTtlSec": 30,
    "maxPayloadBytes": 1048576,
    "perOrgPerSec": 10,
    "perOrgBurst": 20,
    "perIpPerMin": 300,
    "storageQuotaBytes": 10737418240,
    "maxOrganizationsPerTenant": 2
  },
  "isPublic": true,
  "createdAt": "2026-02-27T00:00:00Z",
  "updatedAt": "2026-02-27T00:00:00Z"
}

ถ้ายังไม่อยากมี collection นี้ เฟสแรก “hardcode defaults” ใน service ได้ แต่ให้ทำเป็น planCatalog เดียวจบ อย่า hardcode กระจาย

subscriptions

ผูกกับ tenant/account (ในระบบคุณ tenant = realm/tenantId ของ Keycloak/Permify)
1 tenant มี 1 subscription active (ง่ายสุด, production-grade)

{
  "id": "sub_...",
  "tenantId": "aisom",
  "planId": "freemium",
  "status": "active",
  "billingCycle": "none",
  "currentPeriodStart": "2026-02-27T00:00:00Z",
  "currentPeriodEnd": null,
  "licenseKeyHash": null,

  "overrides": {
    "maxPayloadBytes": null,
    "perOrgPerSec": null,
    "perIpPerMin": null,
    "storageQuotaBytes": null,
    "maxOrganizationsPerTenant": null
  },

  "createdAt": "...",
  "updatedAt": "..."
}
organizations

เพิ่ม field สำหรับ ingest/subscription override เฉพาะ org (ถ้าจำเป็น)

{
  "ingestConfig": {
    "rateLimitPerSec": 10,
    "rateLimitBurst": 20
  }
}

จุดนี้สำคัญ: อย่าให้ org มีสิทธิ override “เกิน plan” โดยไม่ตั้งใจ
วิธีที่ปลอดภัย: orgConfig ใช้เป็น “request” แล้ว service จะ min(orgConfig, planLimit) เป็น effective limit

4) Indexes (mongoBootstrap)
models/subscripmod/subscription.go
เพิ่ม bootstrap ใน internal/repo/subscriprepo/mongoBootstrap.go หรือแยกไฟล์ใหม่ subscriptionBootstrap.go (แนะนำแยกเพื่อไม่รก)

Required indexes

subscriptions

unique: { tenantId: 1 } (กัน tenant มีหลาย subscription active แบบมั่ว)

index: { status: 1, updatedAt: -1 } (ops/debug)

subscriptionPlans

unique: { id: 1 }

แนว DI / bootstrap style เดิม:

// internal/repo/subscriprepo/subscriptionBootstrap.go
package subscripepo

import (
  "context"
  "github.com/hotkhwan/gateway-api/config"
)

func init() {
  config.RegisterMongoBootstrap(func(ctx context.Context) error {
    if err := NewSubscriptionRepo().EnsureIndexes(ctx); err != nil {
      return err
    }
    // optional
    if err := NewSubscriptionPlanRepo().EnsureIndexes(ctx); err != nil {
      return err
    }
    return nil
  })
}
5) Repo Layer (subscriprepo)

เพิ่ม repo ใหม่ (อย่ายัดใน orgRepo ให้ปน)

internal/repo/subscriprepo/subscriptionRepo.go

internal/repo/subscriprepo/subscriptionPlanRepo.go (optional)

subscriptionRepo responsibilities

FindByTenantId(ctx, tenantId) (*Subscription, error)

UpsertDefaultIfMissing(ctx, tenantId) (สร้าง freemium auto)

ActivateEnterpriseByLicense(ctx, tenantId, licenseKey) (hash license)

UpdatePlan(ctx, tenantId, planId, billingCycle) (สำหรับ admin)

Repo = CRUD/queries only. ห้าม logic enforce limit อยู่ใน repo

6) Service Layer
6.1 subscriptionSvc (source of truth + effective limits)

ไฟล์: internal/services/subscriptionsvc/subscription.go

Responsibilities:

resolve effectiveLimits จาก:

planCatalog (หรือ subscriptionPlans)

subscription overrides

provide API สำหรับ ingestsvc: GetTenantLimitsCached(ctx, tenantId)

Key decision: ingest รับ orgId แต่ subscription ผูกกับ tenantId
ดังนั้น ingestsvc ต้องรู้ tenantId:

ทางเลือก A (แนะนำ): ให้ ingress endpoint ใส่ header X-Tenant-Id หรือ derive จาก host/subdomain

ทางเลือก B: lookup org->tenantId จาก Mongo (แพงกว่า) แต่ cache ได้

คุณทำ multi-tenant อยู่แล้ว ผมแนะนำ “X-Tenant-Id” เป็น mandatory ใน ingest path (หรือ map จาก API key)

6.2 ingestsvc (hot path) เปลี่ยนจาก “org config” เป็น “effective limits”

แกนที่ต้องเปลี่ยน:

MaxPayloadBytes ต้องมาจาก effectiveLimits ไม่ใช่ const global

rate limit per-org/per-ip ต้องใช้ limit จาก effectiveLimits

cache TTL ต้องใช้ orgCacheTtlSec จาก plan

สรุปโครงสร้างใหม่:

ingestsvc จะ cache “effective ingest policy” ต่อ org:

includes exists, maxPayloadBytes, perOrgPerSec, perOrgBurst, perIpPerMin, orgCacheTtl

Mongo fallback จะอ่าน:

org exists + org.IngestConfig (optional)

tenant subscription limits (ผ่าน subscriptionSvc หรือ subscriptionRepo)

Redis keys:

orgcache:ingest:{tenantId}:{orgId} -> policy JSON + TTL

counters: rl:org:{tenantId}:{orgId}:{unixSec} and rl:ip:{tenantId}:{ip}:{unixMin}

สำคัญ: per-ip ต้อง scope ด้วย tenant ไม่งั้น tenant หนึ่งยิงแล้วไปกระทบอีก tenant

7) Cache & Failure Strategy (Production-grade)
7.1 Cache policy (Redis)

policy cache TTL: ตาม plan (orgCacheTtlSec)

negative cache (org not found): 10s เหมือนเดิม (ดี)

subscription/plan เปลี่ยน: ให้ invalidate ด้วย version bump ใน key

key: orgcache:ingest:v1:{tenantId}:{orgId}

เวลาเปลี่ยน schema/format bump เป็น v2

7.2 Fail-open vs Fail-closed (อย่าทำมั่ว)

Redis down:

rate limit counters ใช้ Redis เป็นหลัก → ถ้า Redis ล่ม แล้วคุณ fail-open = เปิดให้ยิงถล่ม Kafka ได้

แต่ fail-closed = ลูกค้าดีโดนยิงทิ้งหมด

ข้อเสนอ production-grade:

Rate limit: fail-open แต่ใส่ “local emergency limiter” (in-memory token bucket แบบหยาบ) กัน meltdown

Plan/payload validation: fail-closed เมื่อ resolve limits ไม่ได้ (เพราะถ้าไม่มี limits = เปิดช่อง abuse payload ใหญ่)

สรุป:

Resolve effectiveLimits ไม่ได้ → reject ด้วย 503/429 (“subscription unavailable”)

Redis incr fail → ใช้ local limiter (per-process) แล้วผ่าน/กันตาม threshold (หยาบแต่ช่วย)

8) Enforce Points (Where to check what)
8.1 Ingest request validation (ก่อน marshal / ก่อน Kafka)

Order ที่ถูก:

basic validate: body not empty

resolve org policy (cache/mongo)

payload size check using policy.maxPayloadBytes

rate limit check:

per-org per-sec (use policy.perOrgPerSec)

per-ip per-min (use policy.perIpPerMin)

produce Kafka

คุณทำ payload size check ก่อน lookupOrg ตอนนี้ไม่ถูกถ้า maxPayloadBytes เป็น per-plan เพราะต้องรู้ plan ก่อน

8.2 Organization creation limit (tenancy)

ตอน create org endpoint (orgService.CreateOrg):

count orgs by tenantId

compare to subscription maxOrganizationsPerTenant

ถ้าเกิน: 409 หรือ 403 (ผมแนะนำ 409 “limit reached”)

ต้องมี index/support query:

organizations index { tenantId: 1 }

9) Repo Structure (ตาม style คุณ)

เพิ่มโฟลเดอร์/ไฟล์แบบนี้:

internal/
  repo/
    authzrepo/
      mongoBootstrap.go
      subscriptionBootstrap.go
      subscriptionRepo.go
      subscriptionPlanRepo.go        (optional)
      org.go
  services/
    subscriptionsvc/
      subscription.go
      catalog.go                    (ถ้าไม่ใช้ collection plans)
    ingestsvc/
      ingest.go                     (ปรับให้ใช้ subscriptionSvc)
      errors.go

DI wiring (ตัวอย่าง):

subRepo := authzrepo.NewSubscriptionRepo()
subSvc := subscriptionsvc.NewSubscriptionService(subRepo, redis)

orgRepo := authzrepo.NewOrgRepo(config.DB)
ingestSvc := ingestsvc.NewIngestService(orgRepo, subSvc, redis)

จุดนี้ “ต้องยอม” เพิ่ม dependency ของ ingestSvc ไปที่ subSvc ไม่งั้นคุณจะไปกระจาย subscription logic ใน ingestSvc แล้วเละ

10) API/Operations (ขั้นต่ำที่ควรมี)
10.1 Internal admin endpoints (หลังบ้าน)

POST /api/v1/subscriptions/bootstrap
สร้าง freemium ให้ tenant ถ้ายังไม่มี (idempotent)

PATCH /api/v1/subscriptions/plan
เปลี่ยน plan (admin only)

POST /api/v1/subscriptions/enterprise/activate
activate ด้วย licenseKey (hash ก่อนเก็บ)

GET /api/v1/subscriptions/me
ดู plan+limits (debug/support)

10.2 Observability

log fields ที่ควรมีทุก ingest:

tenantId, orgId, planId

maxPayloadBytes, perOrgPerSec, perIpPerMin

cacheHit true/false

rateLimited true/false + reason

metrics (ขั้นต่ำ):

ingest accepted/rejected counters by planId

cache hit ratio

rate limit rejects by reason

11) Rollout Plan (ไม่ทำให้ prod พัง)
Phase 0: Schema + indexes

add collections/indexes + bootstrap

deploy แล้ว verify indexes created

Phase 1: subscriptionSvc + defaults

implement UpsertDefaultIfMissing(tenantId) ให้ทุก tenant มี freemium

เพิ่ม endpoint bootstrap หรือทำตอน middleware init

Phase 2: enforce org count (tenancy)

บังคับ maxOrganizationsPerTenant ตอน create org

นี่ safe และ impact ต่ำ

Phase 3: enforce ingest by plan

ปรับ ingestsvc ให้ resolve policy จาก subscriptionSvc

เปิด feature flag:

SUBSCRIPTION_ENFORCE_INGEST=true

monitor rate rejects + payload rejects

Phase 4: enterprise activation

implement license activation

add overrides support

12) Hard Truths / Risks (ที่ต้องพูดตรงๆ)

ถ้า Redis ล่มแล้วคุณ fail-open 100% = เปิดประตูให้ยิงถล่ม Kafka
ต้องมี local emergency limiter อย่างน้อย

ตัวเลข maxPayloadBytes ที่คุณเขียน “1MB แต่คอมเมนต์ 10MB” และ “2024*2024 ไม่ใช่ 40MB”
ถ้าไม่แก้ตอนนี้ วันหนึ่งลูกค้าจะด่า เพราะ behavior ไม่ตรงแพ็กเกจ

per-IP limit ต้อง scope ด้วย tenant ไม่งั้น multi-tenant จะ cross-impact กันเอง

ingest ต้องรู้ tenantId ให้ชัด (header / api key / host mapping) ไม่งั้นจะต้อง lookup org->tenant ทุกครั้งแล้วเสีย performance