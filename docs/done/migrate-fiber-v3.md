# Migration Plan: Fiber v2 → v3

**Branch:** `chore/migrate-fiber-v3`  
**Scope:** 212 files, ~3,000 call sites  
**Principle:** inventory → scoped replace → compile gate → manual verify  
**Not in this PR:** `go get -u` bulk dependency refresh, Go patch version bump

---

## Pre-Migration Baseline

Before touching anything, capture a clean baseline:

```bash
go build ./...
go test ./... -count=1
```

Both must pass. If either fails, fix first — do not start migration on a broken build.

### Inventory counts

Dump to a file so all team members compare against the same baseline:

```bash
mkdir -p .migration
{
  echo "=== *fiber.Ctx (files) ===" && grep -r '\*fiber\.Ctx' . --include='*.go' -l | wc -l
  echo "=== *fiber.Ctx (lines) ===" && grep -r '\*fiber\.Ctx' . --include='*.go' | wc -l
  echo "=== .UserContext() ===" && grep -r '\.UserContext()' . --include='*.go' | wc -l
  echo "=== .BodyParser( ===" && grep -r '\.BodyParser(' . --include='*.go' | wc -l
  echo "=== gofiber/fiber/v2 (files) ===" && grep -r 'gofiber/fiber/v2' . --include='*.go' -l | wc -l
  echo "=== otelfiber/v2 (files) ===" && grep -r 'otelfiber/v2' . --include='*.go' -l | wc -l
  echo "=== fiber-swagger (files) ===" && grep -r 'fiber-swagger' . --include='*.go' -l | wc -l
  echo "=== .Route( ===" && grep -r '\.Route(' . --include='*.go' | wc -l
} > .migration/fiberV3Inventory.before.txt

cat .migration/fiberV3Inventory.before.txt
```

After each phase, re-run the relevant subset and compare against this file to confirm residual = 0.

---

## Phase 1 — Minimal Dependency Change

**Rule:** only change packages directly required by Fiber migration. No bulk upgrades.

### 1.1 Add Fiber v3

```bash
go get github.com/gofiber/fiber/v3@latest
```

### 1.2 Add otelfiber v3

```bash
go get github.com/gofiber/contrib/otelfiber/v3@latest
```

Verify the import path actually exists in the downloaded module:

```bash
cat $(go env GOPATH)/pkg/mod/github.com/gofiber/contrib/*/otelfiber/v3/go.mod 2>/dev/null | head -5
# or
find $(go env GOPATH)/pkg/mod/github.com/gofiber -name 'go.mod' | xargs grep -l 'otelfiber'
```

If v3 does not exist yet under `contrib/otelfiber/v3`, check the actual module path:

```bash
go get github.com/gofiber/contrib@latest
# then inspect what's available
```

**Do not assume the import path** — verify before writing it into any file.

### 1.3 Add swaggerui

```bash
go get github.com/gofiber/contrib/swaggerui@latest
```

Verify the Config struct fields that actually exist:

```bash
cat $(go env GOPATH)/pkg/mod/github.com/gofiber/contrib/*/swaggerui/*.go | grep -A 20 'type Config'
```

The `swaggerui` package serves a pre-generated OpenAPI JSON file.  
`swag` CLI and all `// @Summary` / `// @Router` annotations are unchanged.  
The `_ "...docs"` init-based import is no longer needed — but **verify this works before removing it**.

### 1.4 Drop v2 packages (after imports are migrated in Phase 2)

Do not drop yet. Keep both in go.mod until all imports are updated.

### 1.5 go mod tidy

```bash
go mod tidy
```

Expected: module graph resolves. If there are conflicts, resolve them before proceeding.

**Compile gate — module resolution only:**

```bash
go build ./...
```

Having both v2 and v3 in `go.mod` does not cause a build failure by itself — Go modules allow multiple major versions to coexist. What will cause failures is source code that still uses v2 types alongside v3 imports. At this stage, module graph resolution should succeed cleanly. If you see `cannot find module` or `ambiguous import` errors, fix those before proceeding. Type-mismatch compile errors are expected and will be resolved in later phases.

---

## Phase 2 — Import Migration

**Rule:** change import paths only. No behavior changes in this phase.

### 2.1 Fiber core import

Files to change: all `.go` files that import `github.com/gofiber/fiber/v2`.

Preferred approach — use your IDE's project-wide find-and-replace with literal string (not regex):

```
Find:    github.com/gofiber/fiber/v2
Replace: github.com/gofiber/fiber/v3
```

If using command line, restrict to import blocks only and verify each file:

```bash
# GNU sed only (Linux / CI with GNU coreutils)
# Do NOT run on macOS default sed without verifying syntax first
grep -rl 'gofiber/fiber/v2' . --include='*.go' | \
  xargs sed -i 's|github.com/gofiber/fiber/v2|github.com/gofiber/fiber/v3|g'
```

After replace, verify zero residual:

```bash
grep -r 'gofiber/fiber/v2' . --include='*.go'
# must return empty
```

### 2.2 otelfiber import

```
Find:    github.com/gofiber/contrib/otelfiber/v2
Replace: <actual v3 import path verified in Phase 1>
```

Verify:
```bash
grep -r 'otelfiber/v2' . --include='*.go'
# must return empty
```

### 2.3 recover middleware import

The recover middleware moves to `github.com/gofiber/fiber/v3/middleware/recover` — same relative path, just v3. The import alias `recovermw` in `main.go` stays.

Verify the path exists:
```bash
ls $(go env GOPATH)/pkg/mod/github.com/gofiber/fiber/v3*/middleware/recover/
```

The Phase 2.1 fiber core replace already handles this. Confirm:
```bash
grep 'middleware/recover' main.go
# should show v3 path
```

### 2.4 swagger import (main.go only)

This is a **manual change**, not a global replace.

In `main.go`:
```diff
-fiberSwagger "github.com/swaggo/fiber-swagger"
-_ "github.com/hotkhwan/gateway-api/docs"
+swaggerui "github.com/gofiber/contrib/swaggerui"
```

Do not wire the new route yet — leave the old registration commented out until Phase 6 (swagger verification spike).

**Compile gate:**

```bash
go build ./...
```

Expected: compile errors on type mismatches (still using `*fiber.Ctx` everywhere). That is expected. Verify that the only errors are type errors, not import resolution errors. If there are unresolved imports, fix before proceeding.

---

## Phase 3 — Handler Signature Migration

**Rule:** `*fiber.Ctx` → `fiber.Ctx` across all handler functions and type definitions.

In Fiber v3, `Ctx` is an interface, not a pointer to a struct.

### 3.1 Scope the change

Before replacing, understand what you are touching:

```bash
grep -rn '\*fiber\.Ctx' . --include='*.go'
```

Review the output. The expected patterns are:
- `func SomeHandler(c *fiber.Ctx) error` — function signatures ✅ replace
- `func(c *fiber.Ctx) error` — inline types ✅ replace
- `fiber.Handler` type alias — stays as-is (Fiber defines this internally) ✅ no change needed
- Comment or string with `*fiber.Ctx` — ⚠️ do not replace

### 3.2 Replace

IDE project-wide replace (preferred):
```
Find:    *fiber.Ctx
Replace: fiber.Ctx
```

Scope to `*.go` files. Review each replacement before accepting, especially any that appear in comments or string literals.

After replace:
```bash
grep -rn '\*fiber\.Ctx' . --include='*.go'
# must return empty
```

### 3.3 fiber.Handler type references

```bash
grep -rn 'fiber\.Handler' . --include='*.go'
```

`fiber.Handler` in v3 is defined as `func(Ctx) error` (no pointer). If any file declares a custom type that shadows this or uses `*fiber.Ctx` in a type definition, fix manually.

**Compile gate:**

```bash
go build ./...
```

After this phase, the number of compile errors should drop significantly. All remaining errors should be about `UserContext()` and `BodyParser` — if you see other error categories, investigate before proceeding.

---

## Phase 4 — Context Migration

**Rule:** `c.UserContext()` → `c`, with an async safety audit first.

In Fiber v3, `fiber.Ctx` implements `context.Context` directly. `UserContext()` is removed.

### 4.1 Async context audit — do this before replacing

`c` in Fiber v3 is a request-bound context. It gets cancelled (or recycled) when the request ends. This is different from the behavior if `c.UserContext()` previously returned a context derived from `context.Background()`.

Find every place where the context leaves the request lifecycle:

```bash
# goroutines that may capture ctx from handler
grep -rn 'go func' . --include='*.go' -B5 | grep -A5 'UserContext\|ctx'

# async publish / enqueue (Kafka, MQTT, jobs)
grep -rn 'Publish\|Enqueue\|Produce\|Emit' . --include='*.go' -B3 | grep 'ctx'

# context stored into struct or variable that outlives handler
grep -rn 'ctx:.*UserContext\|\.ctx = ' . --include='*.go'

# context.WithTimeout / context.WithCancel wrapping handler context
grep -rn 'WithTimeout\|WithCancel\|WithDeadline' . --include='*.go'
```

For each match: if the context is used **after the handler returns** (async goroutine, background job, deferred publish), it must be detached using the existing `traceutil.DetachWithParent`:

```go
// before (v2 — may have been safe if UserContext was background-derived)
ctx := c.UserContext()
go someBackgroundTask(ctx)

// after (v3 — must detach to prevent context cancellation)
ctx := traceutil.DetachWithParent(c)
go someBackgroundTask(ctx)
```

Do not replace these mechanically. Each must be reviewed.

### 4.2 Replace synchronous UserContext calls

After the audit, replace the remaining (synchronous, request-scoped) usages:

```bash
grep -rn '\.UserContext()' . --include='*.go'
```

For each call site that is confirmed to be within the request lifecycle (no goroutine, no async), replace:

```
Find:    c.UserContext()
Replace: c
```

Common patterns after replace:
```go
// was: ctx := c.UserContext()
ctx := c  // c satisfies context.Context

// was: traceutil.StartLite(c.UserContext(), ...)
traceutil.StartLite(c, ...)

// was: logger.FromCtx(c.UserContext(), ...)
logger.FromCtx(c, ...)
```

Verify zero residual:
```bash
grep -rn '\.UserContext()' . --include='*.go'
# must return empty
```

**Compile gate:**

```bash
go build ./...
```

---

## Phase 5 — Request Binding Migration

**Rule:** `c.BodyParser(&req)` → `c.Bind().Body(&req)`.

### 5.1 Verify behavior before mass replace

`c.Bind().Body()` is not guaranteed to be identical to `c.BodyParser()`. Before replacing all 124 sites, test the following cases on a representative endpoint:

- JSON body
- Empty body (should return error or zero struct, verify which)
- Unknown fields (strict vs. permissive)
- Content-Type mismatch

Only proceed with mass replace after you have confirmed the binding behavior matches on at least 2-3 real endpoints.

### 5.2 Replace

IDE project-wide replace:
```
Find:    c.BodyParser(
Replace: c.Bind().Body(
```

Scope to `*.go`. The call signature is identical (`&req` stays as-is).

After replace:
```bash
grep -rn '\.BodyParser(' . --include='*.go'
# must return empty
```

### 5.3 QueryParser and other parsers

Check if any other parser calls exist:

```bash
grep -rn 'QueryParser\|ReqHeaderParser\|ParamsParser\|CookieParser' . --include='*.go'
```

**Do not assume the v3 equivalent method names.** Before replacing any hit, verify from source:

```bash
cat $(go env GOPATH)/pkg/mod/github.com/gofiber/fiber/v3*/bind.go | grep 'func.*Bind\|func.*Query\|func.*Header\|func.*URI\|func.*Cookie'
```

Known v3 equivalents (verify these against source before using):
- `c.QueryParser(&s)` → likely `c.Bind().Query(&s)` — verify
- `c.ReqHeaderParser(&s)` → likely `c.Bind().Header(&s)` — verify
- `c.ParamsParser(&s)` → likely `c.Bind().URI(&s)` — verify, struct tag may change from `params` to `uri`
- `c.CookieParser(&s)` → likely `c.Bind().Cookie(&s)` — verify

For each: test the binding behavior on a real call before mass-replacing. The parser family has content-type and zero-value behavior differences that are not guaranteed to be identical across v2 and v3.

**Compile gate:**

```bash
go build ./...
```

After this phase, the build should be clean or very close to clean. Remaining errors should be isolated to manual changes in Phase 6.

---

## Phase 6 — Manual: App Bootstrap

These changes cannot be done with global replace. Each requires reading and understanding the surrounding context.

### 6.1 `main.go` — fiber.Config field renames

```diff
 fiber.Config{
     ReadBufferSize: 16 * 1024,
     BodyLimit:      50 * 1024 * 1024,
     StrictRouting:  false,
     Prefork:        false,
-    ProxyHeader:             fiber.HeaderXForwardedFor,
-    EnableTrustedProxyCheck: true,
-    TrustedProxies: []string{
-        "10.42.0.0/16",
-        "192.168.0.0/16",
-        "127.0.0.1/32",
-    },
+    TrustProxy: true,
+    TrustProxyConfig: fiber.TrustProxyConfig{
+        Proxies: []string{
+            "10.42.0.0/16",
+            "192.168.0.0/16",
+            "127.0.0.1/32",
+        },
+    },
```

Verify the exact field names against the Fiber v3 source before writing:
```bash
cat $(go env GOPATH)/pkg/mod/github.com/gofiber/fiber/v3*/app.go | grep -A5 'TrustProxy'
```

### 6.2 `main.go` — ErrorHandler

Phase 3 already changed `*fiber.Ctx` → `fiber.Ctx`. Verify:
```go
ErrorHandler: func(c fiber.Ctx, err error) error {
```

### 6.3 `main.go` — recover middleware

Verify the v3 recover package is a drop-in:
```bash
cat $(go env GOPATH)/pkg/mod/github.com/gofiber/fiber/v3*/middleware/recover/*.go | grep 'func New'
```

Usage should remain:
```go
app.Use(recovermw.New())
```

### 6.4 `main.go` — otelfiber middleware

Verify the v3 middleware signature:
```bash
cat $(go env GOPATH)/pkg/mod/github.com/gofiber/contrib/*/otelfiber/v3/*.go | grep 'func Middleware'
```

Usage should remain:
```go
app.Use(otelfiber.Middleware())
```

**Compile gate:**

```bash
go build ./...
# must pass with 0 errors before proceeding to Phase 7
```

---

## Phase 7 — Router Verification Spike

**Rule:** do not mass-replace router API until you have a working proof.

### 7.1 Verify router.Route() in v3

Check the actual Fiber v3 router API:

```bash
cat $(go env GOPATH)/pkg/mod/github.com/gofiber/fiber/v3*/router.go | grep -n 'func.*Route\|RouteChain'
# or
grep -r 'RouteChain\|func.*Route(' $(go env GOPATH)/pkg/mod/github.com/gofiber/fiber/v3*/ 2>/dev/null
```

Possible outcomes:
- `router.Route()` still exists with same signature → no change needed in 31 router files
- `router.Route()` was renamed or removed → need to understand the new API before touching any file

Do not assume. Do not write any router file changes until this is confirmed.

### 7.2 If router.Route() is changed: build one router file first

Pick the simplest router file (e.g. `router/system.go`). Migrate it manually. Build. Confirm it compiles and routes register correctly. Only then proceed to migrate the remaining 30 files.

### 7.3 AllowMethods pattern

Verify that the `r.All(...) + r.Get(...)` pattern for method guards still works:
```go
r.All("/path", middleware.AllowMethods("GET", "POST"))
r.Get("/path", handler)
```

Test with a real request that should 405 before confirming this pattern is fine.

---

## Phase 8 — Swagger Verification Spike

**Rule:** verify actual behavior of `swaggerui` in isolation before replacing the existing route.

### 8.1 Confirm Config struct

The exact `swaggerui.Config` fields must be read from source, not assumed from docs:

```bash
cat $(go env GOPATH)/pkg/mod/github.com/gofiber/contrib/*/swaggerui/*.go | grep -A 20 'type Config'
```

### 8.2 Confirm file-based serving behavior

`swaggerui` serves `docs/swagger.json` as a static file. Verify:
- `swag init` already generates `docs/swagger.json` (it does by default)
- The BasePath, Path, and FilePath config behave as expected under `/api/v3` prefix
- The UI loads correctly and points to the right JSON file

### 8.3 Wire the new route

Only after confirming the above, update `main.go`:

```go
// Remove:
// api.Get(swaggerPath+"/*", fiberSwagger.WrapHandler)

// Add (exact config fields from source):
app.Use(swaggerui.New(swaggerui.Config{
    // ... verified fields only
}))
```

Verify the swagger path env var behavior matches the original:
```
SWAGGER_PATH=/docs  →  accessible at /api/v3/docs
```

Do not change the leading-slash convention without a confirmed reason — route 404s from slash mismatch are hard to debug.

### 8.4 Remove old swagger import

Only after the new route is confirmed working:
```diff
-_ "github.com/hotkhwan/gateway-api/docs"
```

**Compile gate:**

```bash
go build ./...
go mod tidy
```

---

## Phase 9 — Final Verification

### 9.1 Residual grep

Run the full inventory grep set from Pre-Migration Baseline. Every count must be 0:

```bash
grep -r '\*fiber\.Ctx' . --include='*.go' | wc -l        # 0
grep -r '\.UserContext()' . --include='*.go' | wc -l     # 0
grep -r '\.BodyParser(' . --include='*.go' | wc -l       # 0
grep -r 'gofiber/fiber/v2' . --include='*.go' | wc -l    # 0
grep -r 'otelfiber/v2' . --include='*.go' | wc -l        # 0
grep -r 'fiber-swagger' . --include='*.go' | wc -l       # 0
```

### 9.2 Smoke matrix

Test each category manually or via integration test:

| Category | Test case | Pass? |
|---|---|---|
| JSON binding | POST with valid JSON body | |
| JSON binding | POST with empty body | |
| JSON binding | POST with invalid Content-Type | |
| Query params | GET with query string | |
| Path params | GET /:id | |
| Multipart | file upload endpoint | |
| Auth middleware | protected route with valid JWT | |
| Auth middleware | protected route with missing JWT → 401 | |
| ActiveOrg middleware | valid org → passes | |
| ActiveOrg middleware | invalid org → 403 | |
| Error handler | route that returns 500 | |
| Error handler | validation error → 400 | |
| Method guard | wrong method → 405 | |
| Swagger | GET /api/v3/docs → UI loads | |
| Swagger | OpenAPI JSON accessible | |
| Tracing | X-Trace-Id in response header | |
| Async context | Kafka publish after handler returns | |
| File download | GET file proxy/download endpoint | |
| File upload | multipart image upload | |
| Webhook | inbound webhook endpoint | |
| Kafka→HTTP | HTTP handler that triggers Kafka produce | |
| Version | GET / → version field present | |

**Swagger — test both access paths:**
- Direct: `http://localhost:3001/api/v3/docs`
- Behind ingress/reverse proxy: test from the actual deployed URL, not just localhost — path prefix stripping behavior may differ

### 9.3 go test

```bash
go test ./... -count=1
```

---

## Commit Strategy

Split into atomic commits so each is independently bisectable:

```
commit 1: go.mod: add fiber/v3, otelfiber/v3, swaggerui (keep v2 — do not drop yet)
commit 2: imports: fiber/v2 → fiber/v3, otelfiber, swagger package swap
commit 3: types: *fiber.Ctx → fiber.Ctx across all handlers
commit 4: context: c.UserContext() → c (synchronous sites only)
commit 5: context: async sites — add traceutil.DetachWithParent where needed
commit 6: binding: c.BodyParser → c.Bind().Body
commit 7: main: fiber.Config fields, error handler, middleware wiring
commit 8: router: verified router API changes
commit 9: swagger: wire swaggerui, remove old route
commit 10: cleanup: go mod tidy, drop fiber/v2, otelfiber/v2, fiber-swagger from go.mod
```

Prefer compile-clean commits. For replace-based phases (commits 2–6), it is acceptable to use a short-lived working branch where intermediate commits may not compile, as long as each **phase** ends at a clean compile gate before merge or before starting the next phase.

**Notes for replace-based commits (commits 2–6):**
- Project-wide literal replace must be reviewed hit-by-hit, not accepted blindly
- Do not replace inside comments, string literals, test data, or example snippets
- Some call sites may have unusual formatting or spacing that the replace misses — verify with residual grep after each replace

---

## Rollback

If a compile gate fails and the root cause is not clear within a reasonable investigation window, discard uncommitted working-tree changes and return to the current checked-out commit state:

```bash
# Stash any local scripts or scratch files you want to keep first
git stash -u

# Then reset working tree to last commit
git restore .
```

Use `git clean -fd` only when you are certain there are no local-only files (generated docs, scratch scripts, local configs) that have not been committed — it removes all untracked files and cannot be undone.

Do not attempt to patch forward from a broken state. Diagnose first, then re-apply changes from scratch for the affected phase. The atomic commit strategy exists precisely to make this safe — each commit is a known-good checkpoint to return to.

---

## Hotspot Files — Migrate These First

Within each phase, prioritize these files. They are the highest-traffic paths and will surface breaking changes earliest:

1. `main.go` — app bootstrap, middleware chain, error handler
2. `internal/middleware/auth.go` — every protected request passes through here
3. `internal/middleware/activeorg.go` — org context used by most controllers
4. `internal/middleware/audit.go` — captures request/response; sensitive to Ctx API changes
5. `utils/httputil/success.go` + `utils/httputil/error.go` — all responses flow through here
6. `router/*.go` (31 files) — all route registrations
7. `controllers/` (189+ files) — last, because they depend on the above being stable

---

## Out of Scope for This PR

- `go get -u` bulk dependency refresh — separate PR after this lands
- Go version patch bump (`1.26.0` → `1.26.1`) — separate one-line PR
- Any feature work on top of migrated code

---

## Checklist

```
[x] Pre-migration: go build ./... passes
[ ] Pre-migration: go test ./... passes
[x] Pre-migration: inventory counts dumped to .migration/fiberV3Inventory.before.txt

[x] Phase 1: Fiber v3 / otelfiber v3 / swaggerui added to go.mod
[x] Phase 1: import paths verified from actual module source
[x] Phase 1: go mod tidy passes

[x] Phase 2: all gofiber/fiber/v2 imports → v3
[x] Phase 2: otelfiber import updated
[x] Phase 2: residual grep = 0
[x] Phase 2: compile gate passed

[x] Phase 3: *fiber.Ctx → fiber.Ctx
[x] Phase 3: fiber.Handler references verified
[x] Phase 3: residual grep = 0
[x] Phase 3: compile gate passed

[x] Phase 4: async context audit completed (kwatapi/sync*.go uses DetachWithParent correctly)
[x] Phase 4: DetachWithParent added where needed
[x] Phase 4: c.UserContext() → c (sync sites)
[x] Phase 4: residual grep = 0
[x] Phase 4: compile gate passed

[x] Phase 5: behavior verified on representative endpoint before mass replace
[x] Phase 5: c.BodyParser → c.Bind().Body
[x] Phase 5: QueryParser / others — v3 method names verified from source before replacing
[x] Phase 5: residual grep = 0
[x] Phase 5: compile gate passed

[x] Phase 6: fiber.Config fields verified from source
[x] Phase 6: ErrorHandler signature correct
[x] Phase 6: recover / otelfiber wiring verified
[x] Phase 6: compile gate passed

[x] Phase 7: router.Route() v3 API verified from source (app.go:995, group.go:215 — unchanged)
[x] Phase 7: no router file changes needed (API identical in v3)
[x] Phase 7: AllowMethods pattern verified (uses fiber.Ctx interface correctly)

[x] Phase 8: internal/swagger adapter built (swag.ReadDoc + swaggerFiles.FS)
[x] Phase 8: swagger title updated "Klynx API" → "Gateway API"
[x] Phase 8: docs import kept (required for swag.ReadDoc registry)
[x] Phase 8: compile gate passed

[x] Phase 9: all residual greps = 0
[ ] Phase 9: smoke matrix completed (including file upload, Kafka→HTTP, swagger behind proxy)
[ ] Phase 9: go test ./... passes
[x] Phase 9: commit history clean and bisectable
[x] Phase 9: go.mod no longer contains fiber/v2, otelfiber/v2, fiber-swagger
```
