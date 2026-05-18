# Klynx Camera Overlay — Inbound PATCH Endpoint Plan (rev 3)

**Date:** 2026-05-18
**Status:** Draft (rev 3 — post-Codex-2)
**Review Status:** Pending Codex Review
**Feature Owner Backend:** `gateway-api` for **implementation**; `klynx-api` for **contract authority** (per [`docs/contracts/README.md`](../contracts/README.md): klynx-api is the caller and a klynx hub contract exists, so the canonical contract lives at the klynx hub)
**Related Repos:** `gateway-api` (implements), `klynx-api` (caller + contract owner)
**Canonical Contract:** [`klynx-api/docs/contracts/camera-gw-managed-overlay.md`](../../../klynx/klynx-api/docs/contracts/camera-gw-managed-overlay.md) §8 — finalized rev 2 jointly authored 2026-05-18
**Supersedes:** rev 1 (rejected by Codex for missing contract + wrong topic + schema mismatch + missing klynx-api scope) and rev 2 (rejected by Codex for: spoke contract in wrong repo; If-Match overpromise; event-reconcile claim; auth/null still open)
**Depends on:** `camera-gw-managed-overlay` Phase A shipped (klynx-side `gwSyncStatus="localOnly"` exists)

---

## 1. Executive Summary

This plan describes the **gateway-api implementation work** for the `PATCH /admin/device-management/cameras/{gwDeviceMgmtId}` endpoint defined canonically in the klynx-api hub contract `camera-gw-managed-overlay.md` §8 (rev 2, finalized 2026-05-18).

The contract was previously a STUB in the klynx-api hub plus a parallel spoke-companion file in this repo (`gateway-api/docs/contracts/klynx-camera-overlay-inbound.md`). Codex round-2 review correctly pointed out that `gateway-api/docs/contracts/README.md` reserves this repo's contract directory for flows whose consumer is **not** klynx-api. Since klynx-api **is** the caller here, the contract belongs in the klynx hub. The gateway-api companion file has been **deleted** in this rev; klynx hub §8 is the single source of truth for endpoint shape, auth, error codes, and idempotency semantics. This plan covers only the implementation, tests, and gateway-side rollout against that contract.

**Three substantive changes from rev 2** (driven by Codex round-2):

1. **Contract moved to klynx hub** — no gateway-api/docs/contracts/ companion (per README rule)
2. **If-Match weakened to replay-only** — no `409 PRECONDITION_FAILED` in v1, because the existing `devicemgmtsvc.Update` path ([`service.go:58-73`](../../internal/services/devicemgmtsvc/service.go)) does not bump `lastOutboundHash`, so a true concurrent-edit gate would fire false positives
3. **`gw.devices.changed.v1` redefined as notification-only** — not "projection reconciles from event body", because the slim 7-field payload cannot carry updated values

Codex auth/null decisions are locked: forwarded operator JWT only (no s2s in v1); absent field = no change; `""` = set empty; `null` = `400 VALIDATION_FAILED`.

---

## 2. Scope

### In Scope

- Implement endpoint `PATCH /admin/device-management/cameras/{gwDeviceMgmtId}` per klynx hub §8 (rev 2)
- New repo method on `devicemgmtrepo` for whitelisted partial update + `lastOutboundHash` recompute (called only from this new service path; existing `Update` keeps its current shape)
- Add `LastOutboundHash` field to `models/ingestmod/DeviceManagement` (additive, `omitempty`)
- Auth integration: `middleware.AuthBearer()` (existing) + `X-Active-Workspace` validation (use existing if present; otherwise add a thin `ActiveWorkspace` middleware)
- Reuse existing `internal/services/devicemgmtsvc/service.go:90` `publishDevicesChanged` after successful persist (no schema change to the event)
- `swag` annotations on handler; regenerate `docs/swagger.yaml`
- Tests: handler 200 / 400 (each rejection code) / 401 / 403 / 404 / 5xx; service-layer round-trip; replay-only If-Match observability

### Out of Scope

- Concurrent-edit gating (full `If-Match` `409` semantics) — requires all gateway write paths to bump `lastOutboundHash`; tracked as klynx hub §8.8 v2 work
- Enrichment of `gw.devices.changed.v1` payload to carry updated state — separate klynx hub contract amendment
- Service-to-service token auth — separate amendment
- Stream-credentials sync (`url`/`user`/`password`/`brand`/`ip`/`district`/`type`/`location`) — klynx canonical, not synced (klynx hub §8.6 amendment)
- klynx-api Phase B push worker implementation — separate klynx-api PR (still required for end-to-end success; tracked in §8 below)

### Success Criteria

- Endpoint deployed to gateway-api dev
- klynx-api Phase B PR deployed to dev with `ENABLE_GW_CAMERA_OVERLAY_PUSH=true`
- klynx-side rename of a `provider="gw"` camera → klynx push fires → gateway-api `device_management` row updates `name` and `lastOutboundHash` within 10s
- klynx-side push of `url`/`user`/`password` (manually crafted negative test) → gateway returns `400 FIELD_NOT_ACCEPTED` with the offending field
- klynx-side push with `null` value on a string field → `400 VALIDATION_FAILED`
- klynx-side push with `If-Match` mismatch → `200 OK` with `X-If-Match-Status: mismatched` response header (no `409`)
- `gw.devices.changed.v1` consumer count increments by 1 after each successful PATCH
- Codex round-3 approves both this plan and the rev-2 klynx hub contract update

---

## 2A. Standard Cross-Repo Context

| Context | Default Value | Applies? | Notes |
|---|---|---|---|
| Feature owner backend | `klynx-api` | yes for **contract authority** | klynx hub owns §8 |
| Implementation backend | `klynx-api` | **no** | Override: endpoint implementation lives in `gateway-api` because `device_management` is the gateway-api SoR |
| Events system of record | `gateway-api` | yes | unchanged |
| Klynx normalized event consumer | `klynx-api` via `gw.events.normalized.v1` | yes | unaffected |
| Device/camera identity and sync state source of truth | `gateway-api/device_management` | yes | this is why implementation is here |
| Klynx camera model | projection / consumer model | yes | klynx still owns stream creds; projection ≠ subset |
| Frontend contract rule | FE must not guess schema | yes | no direct FE impact in this plan |

---

## 3. Current State

### Current Backend Flow

```text
klynx operator edits camera on klynx UI
  → klynx-api PATCH /kapi/resources/camera/:id (existing)
  → cameras.externalSource.{name,location,...}Overridden = true
  → cameras.externalSource.gwSyncStatus = "localOnly"
  → STOPS — no push to gateway-api today (no worker, no flag)
```

### Current Constraints

- klynx hub contract §8 finalized in this rev (no longer STUB)
- gateway-api has no admin write endpoint for `device_management` today; existing write paths are ingest-driven via `internal/services/devicemgmtsvc.Update` (called from `internal/kafka/normalizedcons/consumer.go` and bulk import paths)
- `device_management` schema is identity+geo only ([`models/ingestmod/deviceManagement.go:8-25`](../../models/ingestmod/deviceManagement.go)); no `url`/`user`/`password`/`brand`/`ip`/`type` — confirms klynx hub §8.3 accepted-fields whitelist
- klynx push worker not shipped (per `klynx-api/docs/plan/platform-roadmap.md:188`); klynx-api Phase B PR is a co-merge prerequisite for end-to-end success
- Existing `gw.devices.changed.v1` publisher emits slim 7-field payload from [`service.go:90-105`](../../internal/services/devicemgmtsvc/service.go) — this plan does not change the publisher

### Current Risks

- If gateway-api ships endpoint but klynx-api Phase B PR is delayed: endpoint is caller-less. No write happens. No regression. Operator confusion possible if monitoring fires "endpoint deployed but no traffic" — call out in §9 Rollout Notes
- If a future plan changes `gw.devices.changed.v1` payload to carry updated state, downstream consumers must reconcile — klynx hub §8.5 documents this is a separate amendment

---

## 4. Ownership Model

### Feature Owner Backend / Contract Authority

- `klynx-api` — owns the contract at `docs/contracts/camera-gw-managed-overlay.md` §8
- `gateway-api` — owns this plan and the implementation

### System of Record by Domain

| Domain | System of Record | Notes |
|---|---|---|
| Camera identity + geo (`name`, `description`, `lat`, `lng`, `site`, `zone`, `serialNo`) | `gateway-api/device_management` | this PATCH accepts klynx-initiated writes |
| Stream creds + `brand`/`ip`/`district`/`type`/`location` | `klynx-api.cameras` | klynx-canonical; not synced — klynx hub §8.6 amendment |
| Klynx overlay flags (`*Overridden`) | `klynx-api.cameras.externalSource` | gateway-api unaware |
| `lastOutboundHash` (gateway side) | `gateway-api/device_management.lastOutboundHash` | new field; only set by this endpoint |
| `lastOutboundHash` (klynx side) | `klynx-api.cameras.externalSource.lastOutboundHash` | klynx stores returned hash for replay tracking |

### Producer / Consumer Mapping

| Asset | Producer | Consumer | Notes |
|---|---|---|---|
| `PATCH /admin/device-management/cameras/{gwDeviceMgmtId}` | klynx-api push worker (Phase B, separate PR) | gateway-api (this plan) | per klynx hub §8 |
| `gw.devices.changed.v1` | gateway-api (`devicemgmtsvc.publishDevicesChanged`) | klynx-api `gwdevicecons` | existing; this plan adds a new emission point; payload unchanged; **notification-only semantics** per klynx hub §8.5 |

---

## 5. Proposed Architecture

### Target Flow

```text
klynx operator edits camera
  → klynx-api Phase A (existing): set override flags, gwSyncStatus="localOnly"
  → if ENABLE_GW_CAMERA_OVERLAY_PUSH=true AND edit changed at least one synced field:
     → klynx push worker (Phase B, separate PR) calls gateway-api:
        PATCH /admin/device-management/cameras/{gwDeviceMgmtId}
          headers: Authorization, X-Active-Workspace, (optional) If-Match, Content-Type
          body: whitelisted accepted fields per klynx hub §8.3
        → gateway-api controller validates body (whitelist + null/type checks)
        → service.ApplyKlynxOverlay:
            load existing doc
            apply accepted fields
            compute new lastOutboundHash = sha256(canonical accepted-fields payload)
            persist
            publish gw.devices.changed.v1 (existing slim payload — notification only)
        → return 200 with updated doc + X-If-Match-Status header
        → klynx stores returned lastOutboundHash, marks gwSyncStatus="synced"
  → klynx gwdevicecons receives gw.devices.changed.v1 invalidation
        respects *Overridden flags → does not overwrite klynx-side fields
```

### API / Event Surface

- Added REST endpoint: `PATCH /admin/device-management/cameras/{gwDeviceMgmtId}` (per klynx hub §8)
- Updated REST endpoints: none
- Added Kafka topics: none
- Updated Kafka topics: `gw.devices.changed.v1` — emits from a new code path; payload schema unchanged
- Added MQTT topics: none
- Added Redis keys: none
- Added sync rules: none beyond what klynx hub §8 declares

### Artifact Output

- Plan: `docs/plan/klynx-camera-overlay-inbound.md` (this revision)
- Canonical contract: `klynx-api/docs/contracts/camera-gw-managed-overlay.md` §8 (already authored in this rev)
- Regenerated `docs/swagger.yaml` from `swag` annotations
- **No** `gateway-api/docs/contracts/` file (deleted per Codex round-2 ruling)

---

## 6. Contract Summary

The authoritative endpoint contract lives in the klynx hub at `camera-gw-managed-overlay.md` §8 (rev 2). Quick reference:

| Aspect | Value |
|---|---|
| Endpoint | `PATCH /admin/device-management/cameras/{gwDeviceMgmtId}` |
| Auth | Forwarded operator JWT (no s2s in v1) + `X-Active-Workspace: <gwWorkspaceId>` |
| Accepted fields | `name`, `description`, `lat`, `lng`, `site`, `zone`, `serialNo` |
| Rejected fields | stream creds + `brand`/`ip`/`district`/`type`/`location` (`FIELD_NOT_ACCEPTED`); system/path-param fields (`FIELD_READONLY`); unknown fields (`FIELD_UNKNOWN`) |
| Null semantics | absent = no change; `""` = set empty; `null` = `400 VALIDATION_FAILED` |
| Success | `200 OK` with full updated doc + new `lastOutboundHash`; response header `X-If-Match-Status: matched\|mismatched\|absent` |
| Errors | `400 FIELD_NOT_ACCEPTED / FIELD_READONLY / FIELD_UNKNOWN / VALIDATION_FAILED`; `401 AUTH_INVALID`; `403 AUTHZ_FORBIDDEN`; `404 DEVICE_NOT_FOUND`; `5xx INTERNAL_ERROR`. **No `409` in v1.** |
| Idempotency | `If-Match` replay-only — never gates the write; surfaces as `X-If-Match-Status` only |

### Frontend Impact Summary

- No FE change. klynx-api is the caller. klynx FE continues to consume `camera-gw-managed-overlay` for the `gwSyncStatus` badge.

---

## 7. Field Ownership and Sync Rules

See klynx hub `camera-gw-managed-overlay.md` §6 (with §8.6 amendment) and §8. Summary applicable to this implementation:

| Field | Accepted by PATCH? | Authoritative Writer | Notes |
|---|---|---|---|
| `name`, `description`, `lat`, `lng`, `site`, `zone`, `serialNo` | ✅ | gateway-api/device_management | klynx propagation via `gw.devices.changed.v1` notification + klynx re-fetch path (existing) |
| `url`, `user`, `password`, `brand`, `ip`, `district`, `type`, `location` (free-text), and any `streamUrl`/`streamUser`/`streamPassword` synonyms | ❌ `400 FIELD_NOT_ACCEPTED` | `klynx-api.cameras` | klynx-only |
| `gwCamId`, `deviceMgmtId`, `gwWorkspaceId`, `tenantId`, `orgId`, `sourceFamily`, `entityType`, `entityId`, `deviceId`, `createdAt`, `updatedAt`, `lastOutboundHash` | ❌ `400 FIELD_READONLY` | gateway-api | system / path-param only |

### Write Authority Policy

- `gateway-api/device_management` authoritative for accepted fields
- klynx-api may initiate writes via this endpoint
- After commit, gateway-api emits `gw.devices.changed.v1` as a notification (not a state-bearing event)
- Concurrent-edit story: last-writer-wins; klynx override flags prevent inbound overwrite; `If-Match` is observability-only

### Idempotency / Replay (v1 — replay-only `If-Match`)

- Klynx retry with same body → same canonical payload → same new hash → byte-identical persist (modulo `updatedAt`); klynx end-state = `synced`
- Klynx replay after partial failure → safe
- Concurrent edit by gateway-portal admin between klynx's last sync and klynx's push: gateway-portal admin write is lost on klynx's next push (last-writer-wins). Operator awareness via `X-If-Match-Status: mismatched`. v2 of the hub contract may add real `409` semantics once all gateway write paths bump the hash

---

## 8. Cross-Repo Impact

### Backend Repos

| Repo | Change Type | Required Work |
|---|---|---|
| `gateway-api` (this plan) | new REST endpoint + 1 new field + new emission point | controller `controllers/admindeviceapi/cameraOverlayInbound.go`; service method `internal/services/devicemgmtsvc.ApplyKlynxOverlay`; repo whitelist partial-update method; `LastOutboundHash` field on `models/ingestmod/DeviceManagement`; reuse existing `publishDevicesChanged`; swag annotations + tests |
| `klynx-api` (separate co-merged PR) | hub contract patch + push worker | klynx hub `camera-gw-managed-overlay.md` §8 already updated in this rev (authored from this plan's session); klynx-api Phase B PR still required: extend `internal/gateways/gwgw.UpdateCamera`, implement `internal/services/gwdevicesync/pushWorker.go`, wire `ENABLE_GW_CAMERA_OVERLAY_PUSH` flag |

### Frontend Repos

| Repo | Change Type | Required Work |
|---|---|---|
| `klynx` (FE1) | none | hub contract documents `gwSyncStatus` badge |
| `gateway-portal` (FE2) | none | no FE surface |

### Breaking Change Assessment

- Backward compatible? **yes** — additive endpoint, additive `lastOutboundHash` field, no schema change to existing events, no semantic change to existing `Update` write path
- Compatibility window? n/a
- Roll first? `gateway-api` (endpoint live, caller-less) → klynx-api Phase B PR → flag flip per env

---

## 9. Rollout Plan

### Phase Sequencing

1. Codex round-3 review on this plan + verify klynx hub §8 (rev 2) is consistent
2. After approval: implement endpoint in gateway-api; ship to dev (caller-less)
3. klynx-api Phase B PR ships (worker + adapter + flag)
4. Per-env flag flip: dev → staging → prod
5. Monitor `gwSyncStatus` transitions + `X-If-Match-Status: mismatched` counter

### Cross-Repo Rollout Order

1. gateway-api PR (this plan + implementation)
2. klynx-api PR (already includes hub §8 update in this session; Phase B worker is the remaining code)
3. Per-env `ENABLE_GW_CAMERA_OVERLAY_PUSH=true` rollout

### Rollout Notes

- Endpoint may exist callerless for a window. Monitoring: emit a one-time WARN log on startup if the endpoint route is registered but `KAFKA_TOPIC_GW_DEVICES` traffic has been zero on this path for >24h — out of scope for the v1 endpoint but flagged for ops
- Rollback: flip `ENABLE_GW_CAMERA_OVERLAY_PUSH=false` in klynx — endpoint becomes caller-less again. Data revert not needed (writes are last-writer-wins and respect override flags)

### Review Gate

- Codex review status: pending rev 3
- Blocking issues from rev 2 — addressed:
  - ✅ Contract moved into klynx hub (`camera-gw-managed-overlay.md` §8 rev 2); gateway-api spoke file deleted per README rule
  - ✅ If-Match weakened to replay-only with `X-If-Match-Status` observability header; no `409` in v1; rationale documented in klynx hub §8.7
  - ✅ `gw.devices.changed.v1` redefined as notification-only invalidation in klynx hub §8.5; klynx projection-reconcile-from-event-body claim removed everywhere
  - ✅ Auth locked to forwarded operator JWT only (klynx hub §8.2); null/empty semantics locked (`null` → 400 VALIDATION_FAILED) in klynx hub §8.3
- High-risk assumptions: gateway-portal admin write path still does not bump `lastOutboundHash`; v1 does not enforce concurrent-edit gate; v2 work item documented

### Deployment / Migration Notes

- Add `LastOutboundHash string` field with `bson:"lastOutboundHash,omitempty"` and `json:"lastOutboundHash,omitempty"` to `models/ingestmod/DeviceManagement` — additive, defaults `""`
- No backfill; klynx omits `If-Match` on first call (klynx hub §7.5)
- No new index in v1

---

## 10. Implementation Checklist

### Backend Owner Repo (`gateway-api`)

- [ ] Add `LastOutboundHash` field to [`models/ingestmod/DeviceManagement`](../../models/ingestmod/deviceManagement.go) (additive, `omitempty`)
- [ ] Implement `controllers/admindeviceapi/cameraOverlayInbound.go` with `swag` annotations matching klynx hub §8.3 / §8.4 exactly
- [ ] Implement `internal/services/devicemgmtsvc.ApplyKlynxOverlay(ctx, gwDeviceMgmtId, workspaceId, body map[string]any, ifMatch string) (*DeviceManagement, ifMatchStatus, error)` (load → whitelist validate → reject nulls/unknowns/readonly/notaccepted → apply accepted fields → recompute hash → persist → publish notification)
- [ ] Extend `internal/repo/devicemgmtrepo` with whitelist partial-update method (NEW method — do not alter the generic `Update`)
- [ ] Register route in `router/admindevice.go` with `middleware.AllowMethods("PATCH")` + `middleware.AuthBearer()` + workspace-scoping middleware
- [ ] Reuse `internal/services/devicemgmtsvc.publishDevicesChanged` after successful persist (no new transport adapter)
- [ ] Set response header `X-If-Match-Status` on every 200
- [ ] Unit tests (service): whitelist validation (each rejection code), accepted-field round-trip, hash recompute determinism, replay-idempotency (same body → same hash → no-op persist), `null` rejection, `""` accepted as empty
- [ ] Handler tests: 200 / 400 (FIELD_NOT_ACCEPTED, FIELD_READONLY, FIELD_UNKNOWN, VALIDATION_FAILED) / 401 / 403 / 404 / 5xx
- [ ] Run `swag init` and commit regenerated `docs/swagger.yaml` + `docs/swagger.json` + `docs/docs.go`
- [ ] Verify no existing test exercises the new path with `409` expectation (none should; this is a new endpoint)

### Upstream / Peer Backends (`klynx-api` — separate co-merged PR)

- [ ] Hub contract `camera-gw-managed-overlay.md` §8 update — **DONE in this rev** (authored from this session; co-submitted with gateway-api PR for joint Codex review)
- [ ] Hub contract §6 amendment per §8.6 — mark stream creds + brand/ip/district/location/type as "Push to gw? ❌"
- [ ] Implement `internal/services/gwdevicesync/pushWorker.go` per klynx hub §7
- [ ] Implement outbound HTTP call to this endpoint via `internal/gateways/gwgw.UpdateCamera` per klynx hub §8
- [ ] Wire `ENABLE_GW_CAMERA_OVERLAY_PUSH` flag (default `false`)

### Frontend

- [ ] None

---

## 11. Known Issues / Out-of-Scope

- `gw.devices.changed.v1` slim payload: klynx receives notification without updated state; klynx must re-fetch or rely on its own override flags to keep the projection consistent. Enrichment is a separate klynx hub amendment.
- Concurrent gateway-portal admin edit + klynx push race: last-writer-wins in v1. `X-If-Match-Status: mismatched` is an operator-visible drift signal. Real `409` semantics deferred to v2 once all gateway write paths bump the hash.
- Service-to-service token auth: not in v1.
