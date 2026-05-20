<!-- .claude/CLAUDE.md -->
# Gateway API — Claude Rules

## Overview

Gateway API is the central backend for the platform, built with **Go + Fiber**.

**Module:** `github.com/hotkhwan/gateway-api`

## Stack

- HTTP: Go Fiber
- DB: MongoDB (`internal/infra/mongo`)
- Cache: Redis
- MQ: Kafka
- Realtime: MQTT
- Storage: MinIO / S3 (`internal/infra/s3`)
- Auth: Keycloak
- Authz: Permify
- Tracing: OpenTelemetry
- Logging: zerolog
- Docs: Swagger (`swag`)

## Canonical Architecture

```text
HTTP:
router → middleware → controller → service
service → repo | gateways | messaging | configruntime

Async:
consumer/subscriber → extract trace → start span → service
service → repo | gateways | messaging | configruntime

Cross-cutting:
logger | traceutil | crypto/secretbox
```

All production flows must fit this model.

## Package Direction

```text
internal/
  infra/
    mongo/
    s3/
  repo/
    devicerepo/
    mediarepo/
    orgrepo/
  gateways/
  services/
```

Rules:
- `internal/infra/*` = low-level infra helpers/adapters
- `internal/repo/*` = domain repositories
- service must call repo/gateway/messaging, not infra helpers directly
- do not treat infra helpers as domain repos

## Core Rules

### 1) Dependency direction

Allowed:

```text
router → middleware → controller → service
service → repo
service → gateways
service → messaging
service → configruntime
repo → infra
gateways → infra (only when needed internally)
```

Forbidden:

- controller → repo/gateways/messaging/infra
- router → service/repo/infra
- middleware → service/repo/infra
- service → infra directly
- repo → service/gateways/messaging
- gateways → repo/controller
- messaging → repo/controller
- service imports `fiber` / `net/http`

Only **service** may orchestrate multiple dependencies in one workflow.

### 2) Fiber boundary

`*fiber.Ctx` is allowed only in:
- router
- middleware
- controller

Inner layers must use `context.Context` only.

### 3) Context contract

All public methods in service, repo, gateways, messaging, and request-scoped configruntime must accept `ctx context.Context` as the first argument.

Canonical pattern:

```go
func (s *DeviceService) Update(ctx context.Context, req UpdateDeviceRequest) error
```

### 4) HTTP boundary

#### router
- register routes
- attach middleware
- wire handlers

Router must declare allowed HTTP methods **before** the method-specific handler using `AllowMethods`:

```go
r.All("/path", middleware.AllowMethods("GET", "POST"))
r.Get("/path", controller.List)
r.Post("/path", controller.Create)
```

Do **not** use `r.All(path, middleware.AllowOnly(...), handler)` — separate the method guard from the handler.

Do **not** import or use `utils/traceutil` or `utils/httputil` in router files.

#### middleware
- validate JWT/cookie
- validate org context
- set `c.Locals(...)`
- attach trace/audit metadata
- reject invalid requests early

May call only security/boundary gateways needed for admission.

#### controller
- parse params/query/body
- validate request shape
- map DTO ↔ service input/output
- call service
- map service errors to HTTP response

Must not own workflow/business decisions or call repo/gateway/messaging/infra directly.

Controller must start a span at the top using `traceutil.StartLite` and write responses via `utils/httputil`:

```go
func MyHandler(c *fiber.Ctx) error {
    ctx, end, log := traceutil.StartLite(c.UserContext(), "github.com/hotkhwan/gateway-api/myapi", "myapi.MyHandler", "myapi", "MyHandler")
    defer end()
    // ...
    return httputil.Ok(c, data)
}
```

Use `traceutil.Start` when you need the `span` object directly (e.g. to record attributes). Prefer `StartLite` otherwise.

### 5) Service contract

Service owns:
- business rules
- workflow orchestration
- consistency decisions
- permission decisions
- retry/compensation policy
- mapping infra/integration failures to domain errors

Service must not:
- import Fiber
- write HTTP responses
- do raw DB / raw HTTP / raw Kafka / raw MQTT logic inline
- call `internal/infra/mongo` or `internal/infra/s3` directly

Service tracing pattern:

```go
func DoSomething(ctx context.Context, input Input) (*Result, error) {
    ctx, end, log := traceutil.StartLite(
        ctx,
        "github.com/hotkhwan/gateway-api/mysvc",
        "mysvc.DoSomething",
        "mysvc", "DoSomething",
    )
    defer end()
    // log is already bound to traceId — use it for all logs in this scope
}
```

### 6) Repo contract

Repo is persistence only.

Rules:
- no business workflow
- no calls to service/gateway/messaging
- may use `internal/infra/mongo` internally
- regex queries must use `regexp.QuoteMeta()`
- indexes managed centrally
- timestamps crossing boundaries use RFC3339 UTC
- **MongoDB collection names use `snake_case`** — e.g. `media_stream_sessions`, `device_groups`; never `mediaStreamSessions` or `deviceGroups`

### 7) Gateway contract

`internal/gateways/*` = outbound integration adapters.

Allowed:
- HTTP/gRPC/SDK calls
- payload mapping
- wrapped integration errors
- trace propagation
- context-aware logging

Forbidden:
- domain workflow
- calling repo
- writing API responses

### 8) Messaging contract

Messaging is transport adapter only.

Allowed:
- publish/consume mechanics
- serialization boundary
- topic routing
- trace inject/extract
- transport-level retry/backoff

Forbidden:
- business workflow
- calling repo/controller
- deciding business policy

There must be exactly **one publish path per transport**:
- Kafka publish in Kafka adapter only
- MQTT publish in MQTT adapter only
- no duplicate publish helpers in `config`, `utils`, or services

Kafka consumer trace pattern:

```go
kafka.StartConsumerWithHeaders(broker, topic, groupID, func(msg MyEvent, headers map[string]string) error {
    // 1) restore parent span from producer headers
    parentCtx := traceutil.ExtractHeaders(context.Background(), headers)
    // 2) start child span — do NOT use otel.Tracer() directly
    ctx, end, log := traceutil.StartLite(parentCtx, "gateway.mycons", "topic.consume", "mycons", "handler")
    defer end()
    // log is bound to traceId; pass ctx to service
    return mysvc.Handle(ctx, msg)
})
```

### 8a) Cross-service transport (klynx-api ↔ gateway-api)

Service-to-service calls between `klynx-api` and `gateway-api` SHALL prefer **gRPC over HTTP**, mirroring the pattern already used by:

- `phibek.workspace.v1.WorkspaceService` (provisioning + delivery-target registration)
- `phibek.target.v1.TargetService` (delivery-target admin CRUD)
- `phibek.event.v1.EventService` (event detail fetch)
- `phibek.cameraoverlay.v1.CameraOverlayService` (klynx → gw camera overlay PATCH)

The single gRPC server lives in `internal/grpc/workspacegrpc/server.go`; new services register via `srv.RegisterService(svc.ServiceDesc(), struct{}{})` alongside the existing ones (no new port, no new auth path).

Use the corresponding `phibekgw.Client` on the klynx side — it owns the dial, JSON codec, shared-secret metadata, and the otelgrpc stats handler that propagates W3C trace context.

**Auth model:**
- gRPC service-to-service auth is the shared-secret `x-gw-token` metadata (`GRPC_SHARED_SECRET`), checked by `sharedSecretInterceptor`. There is no operator JWT verification on the gRPC path.
- When per-operator audit is required and both services run against the **same Keycloak realm**, the HTTP variant (`Authorization: Bearer <operator JWT>` + `X-Active-Workspace`) is a valid alternative. The dual-transport dispatcher pattern (see `klynx-api/overlayPatcher.go`) lets the caller pick primary + fallback per env.
- For any new cross-service surface, document the choice in the relevant `docs/contracts/<name>.md` §8 (`Auth`).

**HTTP is allowed (not deprecated) for:**
- Ingest hot-path (`POST /events/:orgId/:source`) — public, no JWT, rate-limit-gated.
- Operator-facing routes mounted under `BASE_PATH` (`/api/v1/...`) hit by the FE or external integrators via the istio gateway.
- Internal "admin" REST endpoints used by ops/curl for direct testing (e.g. the legacy `PATCH /admin/device-management/cameras/{id}` kept as the HTTP variant of `phibek.cameraoverlay.v1`).

**Service-to-service HTTP is discouraged** because it requires Keycloak realm parity between caller and callee. When realms drift (or when the deployment runs without a shared SSO), the forwarded-JWT scheme fails with `401 INVALID_TOKEN` and no audit context can be derived. Prefer gRPC + shared secret; add HTTP as a dual-transport alternate only when per-operator audit is required AND realm parity is guaranteed.

**`GW_API_URL` rule (klynx-side):** the env value MUST include gw-api's `BASE_PATH` (`/api/v1`). All `gwgw.*` clients build URLs as `${GW_API_URL}/<route>` — without the prefix, fiber returns a router-level `404`.

### 9) Pragmatic DI rule

Full DI is **not required everywhere**.

Prioritize DI for important boundaries first:
- service → repo
- service → gateways
- service → messaging
- service → configruntime

Full DI is optional for:
- pure helpers
- mappers
- formatters
- validators without external dependencies

Migration rule:
- do not refactor the entire project at once
- new service code must not call infra helpers directly
- new flows must go through repo/gateway/messaging
- legacy package-style code can be migrated incrementally at hot spots

### 10) Domain ownership example

Choose repo by **data ownership**, not by caller package.

For camera lookup to get `rtspUrl`:
- owner = camera/device domain
- use `devicerepo`
- do **not** create it in `mediarepo`
- do **not** let `mapsvc` call mongo helper directly

Preferred repo methods:

```go
FindCameraForStream(ctx context.Context, orgId, cameraId string) (*devicemod.Camera, error)
```

or, if only a few fields are needed:

```go
GetRtspSource(ctx context.Context, orgId, cameraId string) (*CameraRtspSource, error)
```

Use the narrower method when the use case needs only stream source fields.

## Observability

### Logging

Request/message flow must use:

```go
logger.FromCtx(ctx, component, source)
```

Boot/static logger is for startup and non-request initialization only.

### Tracing

Trace must continue from entrypoint to final dependency call.

#### Tracing utilities (`utils/traceutil`)

| Function | Returns | Use when |
|---|---|---|
| `traceutil.StartLite` | `(ctx, end(), log)` | controller / webhook handler (preferred) |
| `traceutil.Start` | `(ctx, span, log)` | when span attributes/events must be set |
| `traceutil.StartScope` | `*Scope{Ctx,Span,Log}` | service methods that need timer or sub-scope |
| `traceutil.InjectHeaders` | — | inject trace into outbound Kafka/HTTP headers |
| `traceutil.ExtractHeaders` | `ctx` | extract trace from incoming Kafka/HTTP headers |
| `traceutil.DetachWithParent` | `ctx` | fire-and-forget goroutines (detach from request cancel) |

#### Full tracing loop

```text
controller  →  traceutil.StartLite / Start   →  span starts, log bound
    ↓ ctx passed down
service     →  logger.FromCtx(ctx, ...)       →  log carries traceId
    ↓ ctx passed down
repo        →  logger.FromCtx(ctx, ...)       →  log carries traceId
gateway     →  logger.FromCtx(ctx, ...)  +  traceutil.InjectHeaders   →  outbound trace
kafka pub   →  traceutil.InjectHeaders(ctx, headers)                  →  trace in header
kafka sub   →  traceutil.ExtractHeaders(parent, headers)              →  span restored
mqtt sub    →  traceutil.ExtractHeaders(parent, headers)              →  span restored
```

Rules:
- controller starts inbound span with `traceutil.StartLite` (or `Start` when span attributes needed)
- service starts its own child span with `traceutil.StartLite` — receives `ctx` from controller
- async consumer/subscriber: extract trace with `traceutil.ExtractHeaders`, then `traceutil.StartLite`
- pass same `ctx` downward — never create `context.Background()` inside a live request or service call
- Kafka/MQTT outbound must propagate trace via `traceutil.InjectHeaders`
- outbound HTTP gateways must inject trace headers via `traceutil.InjectHeaders`
- do **not** use `otel.Tracer(...).Start(...)` directly anywhere — always use `traceutil.StartLite` or `traceutil.Start`

### Correlation

`traceId` is mandatory.

Add domain IDs when useful:
- `eventId`
- `tenantId`
- `orgId`
- `camId`
- `deviceId`
- `topic`
- `sourceType` / `sourceFamily`

## Runtime / Security Contracts

### configruntime

`configruntime` is a read-only runtime dependency for service.

Rules:
- initialized once in container/bootstrap
- injected, not global
- may use TTL cache
- fault-tolerant on DB/config read failure
- not for secrets
- not a business layer
- not a persistence layer

### crypto/secretbox

All sensitive encryption/decryption must go through `internal/crypto/secretbox`.

Mandatory for:
- passwords
- API keys
- credentials
- secret tokens at rest

Failure to load crypto material at startup is fatal.

## API Contract

### Swagger documentation contract

Every controller handler **must** have a `swag` godoc block directly above the function.

Required fields:
- `@Summary` — short single-line label (≤ 10 words)
- `@Tags` — group name matching the router section (e.g. `Maps`, `Media`, `Devices`)
- `@Produce json`
- `@Success` — HTTP status matching the actual `httputil.*` call
- `@Failure 400 {object} gmod.ErrorResponse` — for validation errors
- `@Failure 401 {object} gmod.ErrorResponse` — for protected routes
- `@Failure 500 {object} gmod.ErrorResponse` — for server errors
- `@Router /path [method]`
- `@Security BearerAuth` — **only** on protected routes; omit for public routes

Optional fields:
- `@Description` — longer description when summary alone is not enough
- `@Accept json` / `@Accept multipart/form-data` — when body is expected
- `@Param` — one line per input param (path, query, body, formData)

#### `@Success` ↔ `httputil` mapping

| httputil call | Swagger annotation |
|---|---|
| `httputil.Ok(c, data)` | `@Success 200 {object} gmod.SuccessDataResponse` |
| `httputil.Ok(c, data, "msg")` | `@Success 200 {object} gmod.SuccessDataResponse` |
| `httputil.OkPaginated(c, details, pg)` | `@Success 200 {object} gmod.PaginationResponse` |
| `httputil.MessageOK(c, "msg")` | `@Success 200 {object} gmod.SuccessMessageResponse` |
| `httputil.Created(c, data, "msg")` | `@Success 201 {object} gmod.SuccessDataResponse` |
| `httputil.Accepted(c, data)` | `@Success 202 {object} gmod.SuccessDataResponse` |
| `httputil.NoContent(c)` | `@Success 204` |

#### Example godoc block

```go
// CreateFoo godoc
// @Summary      Create a new foo
// @Description  Creates a foo resource and returns the created document.
// @Tags         Foos
// @Accept       json
// @Produce      json
// @Param        body  body  CreateFooRequest  true  "Foo input"
// @Success      201   {object}  gmod.SuccessDataResponse
// @Failure      400   {object}  gmod.ErrorResponse
// @Failure      401   {object}  gmod.ErrorResponse
// @Failure      500   {object}  gmod.ErrorResponse
// @Router       /foos [post]
// @Security     BearerAuth
func CreateFoo(c *fiber.Ctx) error {
```

Rules:
- HTTP status in `@Success` must match the actual `httputil.*` function used — do not write `@Success 200` when the code returns `httputil.Created`
- Public routes (no auth middleware) must **not** have `@Security BearerAuth`
- `@Tags` must match the router section name exactly (consistent casing)
- Do not annotate middleware-only functions or helper functions

### Protected route headers

```text
Authorization: Bearer <jwt>
X-Active-Org: <orgId>
```

### Common locals

Set by middleware:
- `userId`
- `tenantId`
- `activeOrg`
- `traceId`

Read locals only in middleware/controller boundary code.

### Standard success envelopes

**Single / detail response** (`httputil.Ok` with scalar or struct):
```json
{
  "code": "SUCCESS",
  "message": "...",
  "status": true,
  "details": {}
}
```

**Non-paginated collection response** (`httputil.Ok` with `fiber.Map{"items": ...}`):
```json
{
  "code": "SUCCESS",
  "message": "...",
  "status": true,
  "details": {
    "items": []
  }
}
```

**Paginated list response** (`httputil.OkPaginated`):
```json
{
  "code": "SUCCESS",
  "message": "...",
  "status": true,
  "details": {
    "items": [],
    "summary": { "totalOnline": 10, "totalOffline": 1 }
  },
  "pagination": {
    "page": 1,
    "perPage": 10,
    "totalRecords": 11,
    "totalPages": 2,
    "sortField": "createAt",
    "sortOrder": "desc"
  }
}
```

Rules:
- use `details`, not `detail`
- keep envelope stable
- **never return a bare array** as `details` — always wrap in `{ "items": [] }`
- paginated lists: `pagination` is top-level (not nested inside `details`)
- `summary` counts must reflect the full org/filter scope — not just the current page
- use `perPage` (singular) in `gmod.PageMeta` — not `perPages`
- do **not** use `gmod.SendPagination` or `gmod.SendPaginationOK` — use `httputil.OkPaginated`

### ID field convention

Public-facing response models must use `json:"id"` for the primary identifier.

- **Never** use `json:"camId"`, `json:"deviceId"`, or other domain-specific names as the ID field in a response DTO
- The internal `camId` UUID is the value; the JSON key exposed to clients is always `"id"`

Example:
```go
type PublicCameraItem struct {
    CamID string `json:"id"` // ✅ exposed as "id"
    ...
}
```

### Optional-auth route pattern

For endpoints that serve both anonymous and authenticated users:

**Router:**
```go
router.Get("/live/map", middleware.TryAuthBearer(), middleware.TryActiveOrg(), mapapi.PublicCameraMap)
```

**Controller:**
```go
orgId, _ := c.Locals("activeOrg").(string)
if orgId != "" {
    // authenticated — full org view
    items, err := svc.GetOrgMap(ctx, orgId)
    ...
    return httputil.Ok(c, fiber.Map{"items": items})
}
// anonymous — public-only view
items, err := svc.GetPublicMap(ctx)
...
return httputil.Ok(c, fiber.Map{"items": items})
```

- `TryAuthBearer()` — validates JWT if present, silently continues on failure (sets `userId`, `tenantId` locals)
- `TryActiveOrg()` — verifies Permify org membership if auth locals are set, silently continues on failure (sets `activeOrg` local only on success)
- Use `AuthBearer()` + `ActiveOrg()` (hard versions) for fully protected routes

### HTTP status contract

Default mapping:
- `200 OK` — GET, PATCH, PUT, DELETE success with body
- `201 Created` — POST create success
- `202 Accepted` — async job accepted
- `204 NoContent` — success with no response body
- `400 BadRequest` — validation / malformed input
- `401 Unauthorized` — missing or invalid auth
- `403 Forbidden` — authenticated but not allowed
- `404 NotFound` — resource not found
- `409 Conflict` — duplicate / state conflict
- `422 UnprocessableEntity` — syntactically valid but semantically invalid input, only if the API explicitly uses it
- `500 InternalServerError` — unexpected server error

Rule:
- do not return `200` for failures
- do not hide errors only in response body
- HTTP status and response body must agree

### Error contract

- service → sentinel/domain errors
- repo → wrapped storage errors
- gateways → wrapped integration errors
- messaging → wrapped transport errors
- controller → maps service errors to HTTP status + response code

Never leak raw internal/driver/SDK errors to API clients.

### Bulk CRUD contract

See `.claude/rule/code-style.md` → Bulk CRUD Contract for full rules.

Route structure per resource:
```text
CRUD:  GET / GET /:id / PATCH /:id / DELETE /:id
Bulk:  DELETE /bulk / PATCH /bulk / POST /bulk/approve
```

Rules:
- `DELETE/PATCH /bulk` for bulk CRUD
- body-based IDs (`ids`), batch max 100, partial success response
- `:id` param accepts both domain UUID and alternative key (e.g. hwId) — service resolves
- per-resource explicit patch payload with pointer fields
- delete = remove from org (soft), not hard delete for important resources

### Time rules

- use RFC3339 UTC across boundaries
- keep date/time field names consistent

## Package / File Rules

- every `.go` file must start with a path comment on line 1
- **file names use camelCase** — e.g. `publicMap.go`, `authStream.go`, `kmlUpload.go`; never `public_map.go` or `auth_stream.go`
- keep packages cohesive
- avoid dumping unrelated helpers into vague `utils` / `common`
- choose one transport package layout and keep it consistent

Example:

```go
// controllers/mapapi/publicMap.go
package mapapi
```

## Startup Order

```text
logger.Init()
→ InitMongo()
→ InitRedis()
→ InitKafka()
→ InitOtel()
→ InitSecretboxKeyring()
→ app.NewContainer()
→ start consumers/subscribers
→ router.Init()
→ app.Listen()
```

Infra must be ready before container wiring and before serving/consuming starts.

## Cross-Repo Planning & Contracts

For any feature, bug, flow change, event change, sync change, deploy change, or cross-repo work, follow the plan-review-implement workflow defined in `AGENTS.md`. Do not implement before the plan and contract are reviewed.

### Contract Authority Model

The platform runs hub-and-spoke for cross-repo contracts:

- **Hub (cross-repo / cross-service flow contracts):** `klynx-api/docs/contracts/<name>.md` is canonical. It groups every surface that shares one lifecycle — REST + Kafka + MQTT + Redis + sync + cache + rollout — into one contract per domain or per flow. See `klynx-api/docs/contracts/README.md` for the grouping rule before authoring or consuming.
- **`gateway-api` SoR (this repo):** events canonical detail and `device_management` identity / sync state are owned here. Hub contracts that touch these domains reference `gateway-api` as the canonical writer; do not let projection / consumer state be treated as canonical.
- **`gateway-api/docs/swagger.yaml`:** is the REST schema *subset* of a contract — generated from the `swag` annotations. It is not the full contract. Async (Kafka / MQTT) and Redis-visible behavior, write authority, field ownership, and rollout / replay rules live in the contract `.md` only.
- **`gateway-api/docs/contracts/<name>.md`:** reserved for `gateway-api`-owned cross-repo behavior that has no klynx-api hub equivalent. Use `klynx-api/docs/contracts/TEMPLATE.md` as the skeleton and keep one contract per domain or per flow. See `docs/contracts/README.md` (this repo).

### Frontend Contract Rule

Frontend repos (`klynx`, `gateway-portal`, 3rd-party integrators) consume the contracts above and must not invent any schema across REST / Kafka / MQTT / Redis-visible / cache / sync / auth / permission surfaces. Network traces, screenshots, and source code are not contracts. If FE needs behavior not in the contract, the BE contract must be updated first.

## References

Read before implementing:
- `.claude/rule/code-style.md`
- `.claude/rule/security.md`
- `.claude/rule/test.md`
- `AGENTS.md` (cross-repo plan-review-implement workflow)
- `klynx-api/docs/contracts/README.md` (cross-repo contract grouping rules)
- `klynx-api/docs/contracts/TEMPLATE.md` (canonical contract skeleton — also used when authoring `gateway-api/docs/contracts/<name>.md`)

## Final Rule

If code works but violates these contracts, refactor it before building more on top.
