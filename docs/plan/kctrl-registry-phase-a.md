# KControl Registry — Phase A (gateway-api side)

**Date:** 2026-05-18
**Status:** Implementation (klynx-api Phase B already shipped at 4.60.0)
**Feature Owner Backend:** `klynx-api` (contract authority); `gateway-api` (this plan — implementation of the receiving side)
**Canonical Contract:** [`klynx-api/docs/contracts/kcontrol-gw-managed-registry.md`](../../../klynx/klynx-api/docs/contracts/kcontrol-gw-managed-registry.md) v0.1
**Companion plan:** [`klynx-api/docs/plan/kcontrol-gw-managed-registry.md`](../../../klynx/klynx-api/docs/plan/kcontrol-gw-managed-registry.md) (rev 2)
**Related Repos:** `gateway-api` (implements); `klynx-api` (caller — Phase B shipped at 4.60.0 commit `b5d6724`)

---

## 1. Scope

Implements the gateway-api side of the kctrl-registry contract. klynx-api Phase B (4.60.0) is already shipped and ready: its outbox + worker will start flushing PATCH/DELETE calls to gateway-api as soon as this endpoint is live.

### In Scope

- New collection `kctrl_registry` (Mongo, unique index on `hwId`)
- `PATCH /admin/kctrl-registry/{hwId}` — upsert per contract §4.1; whitelist body fields, return updated doc
- `DELETE /admin/kctrl-registry/{hwId}` — idempotent delete per §4.2
- `GET /admin/system/kctrlRegistryDrift` — operator drift query per §4.3
- `POST /admin/system/kctrlRegistryRetry/{hwId}` — per contract §7.3 (operator manual retry trigger — gateway side just acknowledges; klynx-api owns the outbox reset)
- In-binary LRU cache (1k entries, 5s TTL) with explicit invalidation on PATCH/DELETE per §6
- `kctrlsubmsg.MessageHandler` 3-branch routing per §5 — ENRICH approved / DROP revoked / FORWARD-as-is on miss (**compat mode** — strict-mode flag added in a follow-up Phase A.1)
- Version bump 3.12.0 → 3.13.0
- Tests: service + cache + handler + kctrlsubmsg branching

### Out of Scope

- **Strict mode** — Phase A.1, separate PR after backfill stable ≥1 wk per contract §5.2
- klynx-api outbound adapter — already shipped Phase B 4.60.0
- Cross-replica cache invalidation via Redis pub/sub — v2 per contract §6
- FE drift dashboard — UI decision per contract §10

### Success Criteria

- `kubectl exec` curl `PATCH /admin/kctrl-registry/{hwId}` returns 200 with the upserted doc; row visible in Mongo
- Repeat PATCH (same body) → 200 (idempotent no-op via hash short-circuit)
- DELETE on nonexistent hwId → 204 (per §4.2)
- MQTT `kcontrol.alarms` from approved device → Kafka envelope has `orgId` + `workspaceId` populated
- MQTT from `approved=false` row → no Kafka write; log `kctrlsubmsg: hwId revoked — dropping`
- MQTT from unknown hwId → forwarded as-is (compat mode); log `kctrlsubmsg: unknown hwId — forwarding for discovery`
- `GET /admin/system/kctrlRegistryDrift` returns rows with `lastSyncFromKlynxAt > 1h` ago tagged `cause: stale`

---

## 2. Architecture

Per contract §2 authority direction:

```
klynx-api (write authority) ──PATCH/DELETE──▶ gateway-api/kctrl_registry  (projection)
                                                           │
                                                           ▼
                                              gateway-api/kctrlsubmsg (read)
```

### Component Layout

```
controllers/adminapi/kctrlRegistry.go            (handlers — PATCH/DELETE/GET/POST)
router/adminKctrlRegistry.go                     (route registration)
internal/services/kctrlregistrysvc/service.go    (orchestration)
internal/services/kctrlregistrysvc/cache.go      (LRU + 5s TTL)
internal/repo/kctrlregistryrepo/repo.go          (Mongo: upsert/delete/find/listDrift)
models/kctrlmod/registry.go                      (KctrlRegistry struct)
internal/mqtt/kctrlsubmsg/handler.go             (modify — 3-branch routing using registry)
```

### Field Whitelist (per contract §4.1)

| Accepted in PATCH body | Type | Notes |
|---|---|---|
| `orgId` | string | klynx Permify org uuid |
| `approved` | bool | mirrors klynx.kcontrol.approved |
| `approvedAt` | RFC3339 string | wall-clock from klynx admin action |
| `approvedBy` | string | klynx user uuid |

Path param `hwId` is the cross-ref key — never in body.

### Cache Strategy (contract §6)

- LRU 1000 entries × 5s TTL
- On PATCH/DELETE: evict the `hwId` entry locally before returning response (so originating replica sees fresh state)
- On read (kctrlsubmsg lookup): cache → fall back to Mongo → populate cache
- Other replicas: bounded staleness = 5s TTL

### kctrlsubmsg routing (contract §5)

Per MQTT message, after the existing envelope build:

```
lookup kctrl_registry by hwId
   ├── ROW EXISTS, approved=true   → ENRICH out["orgId"], out["workspaceId"] → forward to Kafka
   ├── ROW EXISTS, approved=false  → DROP (no Kafka write) + log Info
   └── ROW NOT FOUND               → FORWARD as-is (compat mode) + log Info
```

Compat mode preserves bootstrap discovery: klynx-api needs to see the message in order to create the unapproved `klynx.kcontrol` row that the admin will later approve (per contract §5.1).

---

## 3. Auth Model

Per contract §4.1: SA token (Bearer) + `X-Active-Workspace: <gwWorkspaceId>`. For Phase A v1, the existing `middleware.AuthBearer()` + `middleware.ActiveWorkspace()` chain is reused — the SA token validates as a normal bearer JWT. A finer-grained SA-token-only gate is left to a follow-up if abuse is observed.

Operator drift endpoint uses `middleware.RequireRoles([]string{"administrator"})`.

---

## 4. Tests

- Service: upsert idempotency (same body → no-op hash short-circuit); cache invalidation on PATCH and DELETE; cache TTL expiry; 3-branch decision (`Decide(hwId)` returns `{action, row}`)
- Handler: 200 success, 400 FIELD_NOT_ACCEPTED, 401, 404 / 204 (DELETE idempotency), drift listing
- kctrlsubmsg: ENRICH branch sets orgId+workspaceId; DROP branch skips Kafka publish; FORWARD branch leaves orgId empty

---

## 5. Rollout

1. Ship gateway-api 3.13.0 with this endpoint LIVE (handler accepts traffic; kctrlsubmsg routing in **compat mode**)
2. klynx-api 4.60.0 outbox worker (already shipped) flushes pending PATCH/DELETE to gateway-api
3. Operator runs klynx-api backfill (`ENABLE_GW_KCTRL_REGISTRY_BACKFILL=true`) — populates registry with currently-approved devices
4. Monitor `GET /admin/system/kctrlRegistryDrift` for at least 1 week
5. Then ship Phase A.1 (separate chore PR) flipping `KCTRL_REGISTRY_STRICT_MODE=true`

Rollback: revert gateway-api commit → kctrlsubmsg returns to "forward verbatim, no filter, no enrich"; klynx-api 4.59.2 Layer-1 resolver still handles orgId on consumer side.
