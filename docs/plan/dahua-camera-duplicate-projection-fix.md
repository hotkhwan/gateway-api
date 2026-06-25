# Plan — Fix duplicate klynx camera projection on Dahua per-camera ingest

**Date:** 2026-06-25
**Owner Backend:** `klynx-api` (camera SoR + projection — primary fix); `gateway-api` (`device_management` SoR + `gw.devices.changed.v1` producer — secondary fix)
**Repos touched:** `klynx-api` (kapi), `gateway-api`
**Status:** A + B IMPLEMENTED 2026-06-25 (user-approved, review-gate waived); migration script ready; C already implemented upstream (camID Phase 2/3.2 — no work). Tests + `go build ./...` green in both repos.
**Follow-up to:** [`dahua-camera-flow.md`](./dahua-camera-flow.md) · Contract: [`klynx-api/docs/contracts/dahua-camera-event-ingest.md`](../../../../home/klynx/klynx-api/docs/contracts/dahua-camera-event-ingest.md)

---

## Problem (verified on production 2026-06-25)

A Dahua camera registered in klynx as **LRP** (`camId = bab573c0-…`) uploads events to its
per-camera URL `…/events/{org}/dahua/bab573c0-…`. Instead of the events attaching to LRP, a
**second, empty-named camera** `8186551a-…` auto-registered with
`externalSource.gwCamId = bab573c0-…`. Both rows' `eventAutoUploadUrl` point at `/dahua/bab573c0-…`
— i.e. **two klynx rows for one physical camera**.

This directly violates the contract's join-key guarantee:
> §3 — "The camID is the immutable **join key**: `klynx.camera.camId == gateway.device_management.DeviceId`."
> §6.1 / §13 — "klynx camera projection **joins by camId (no dup)**."

### Root causes (3 — all verified in code)

**RC-1 (klynx, PRIMARY) — projection matches only `externalSource.gwCamId`, never `camId`.**
The hot path is:
`HandleNormalized` → `GetDeviceSummaryForIngest(orgId, deviceId)` →
`GetDeviceSummary` → `cameraRepo.FindByGWCamId` (filter `{orgId, externalSource.gwCamId: deviceId}`)
— [`gwdevicesync/service.go:319-358`](../../../../home/klynx/klynx-api/internal/services/gwdevicesync/service.go#L319-L358),
[`devicerepo/gwSync.go:27-40`](../../../../home/klynx/klynx-api/internal/repo/devicerepo/gwSync.go#L27-L40).
LRP is a **native** klynx camera: `camId = bab573c0` but **no `externalSource`**. The gwCamId
filter misses → `GetDeviceSummaryForIngest` returns `!ok` →
[`handleNormalized.go:148-155`](../../../../home/klynx/klynx-api/internal/services/ingestsvc/handleNormalized.go#L148-L155)
calls `deviceAutoSyncer.SyncFromIngestEvent` → `SyncFromGW` creates a **fresh `uuid.New()` camId**
([`service.go:276-278`](../../../../home/klynx/klynx-api/internal/services/gwdevicesync/service.go#L276-L278))
with `externalSource.gwCamId = bab573c0`. **That is the duplicate `8186551a`.**
The contract join key (`camId == deviceId`) is never consulted.

**RC-2 (gateway-api, SECONDARY) — `gw.devices.changed.v1` producer schema is wire-incompatible with the klynx consumer.**
[`devicemgmtsvc/service.go:88-105`](../../internal/services/devicemgmtsvc/service.go#L88-L105) emits
`{deviceMgmtId, workspaceId, tenantId, sourceFamily, entityType, entityId, action}`.
The klynx consumer decodes into `eventbridge.DeviceChangedEvent`
([`types.go:169-198`](../../../../home/klynx/klynx-api/internal/eventbridge/types.go#L169-L198)) which reads
`remoteDeviceId`, `changeType`, `orgId`, `gwWorkspaceId`, `deviceId`, `name`, `lat`, `lng`, `status`.

| klynx reads (json) | gw emits | result |
|---|---|---|
| `remoteDeviceId` | `entityId` | `RemoteDeviceID=""` → `SyncFromGW` **skips** ([service.go:96-99](../../../../home/klynx/klynx-api/internal/services/gwdevicesync/service.go#L96-L99)) |
| `changeType` (created/updated/deleted) | `action` (create/update) | `ChangeType=""` |
| `orgId` + `gwWorkspaceId` | `workspaceId` | both `""` |
| `deviceId` (= camId join key) | *(absent)* | missing |
| `name`/`lat`/`lng`/`status` | *(absent)* | projection never enriched from gw |

→ The Kafka `Provision → gw.devices.changed.v1 → SyncFromGW` path (contract §6.1) is **effectively dead**;
the projection only ever gets created by the ingest auto-sync (RC-1), which is why `8186551a` has an empty name.

**RC-3 (context, NOT a root cause) — register→Provision IS wired; its result just can't land.**
`gwgw.ProvisionClient.ProvisionDeviceIdentity` is called from the camera controller on **create** and **edit**
(`controllers/deviceapi/camera.go:160,283,748` — camID Phase 2 / 3.2, klynx ≥ 4.13x). So gw DOES receive
`POST /admin/device-management/identities` → `ProvisionByCamID` creates `device_management(deviceId=camId)` and
emits `gw.devices.changed.v1`. The reason LRP still has no `externalSource` is **RC-2**: the producer schema is
unreadable by the consumer, so klynx never backfills the native row from that event. → Fixing RC-2 (B) is the
unlock that makes the proactive provision path actually populate the projection; no new wiring is needed.
*(Earlier draft wrongly called this "never wired" — the grep was scoped to `internal/` and missed `controllers/`.)*

> gateway-api's own ingest side is **correct**: on the camID path it sets `source.deviceId = camID` and skips
> the weak auto-upsert ([`ingest.go:484-517`](../../internal/services/ingestsvc/ingest.go#L484-L517)); the
> normalized event carries `source.deviceId = bab573c0` as required. The join key is on the wire — klynx just
> doesn't use it.

---

## Fix decomposition

### A) klynx-api — adopt-by-camId (PRIMARY, fixes the duplicate)

Make the projection lookup honor the contract join key: **match `externalSource.gwCamId == id` OR `camId == id`**,
and on a `camId`-only match, **adopt** the native camera (backfill `externalSource`) instead of creating a new row.

1. **Repo** — add to `CameraRepo` (`devicerepo/gwSync.go`):
   - `FindByGWCamIdOrCamId(ctx, orgId, id) (*CameraMongo, error)` — filter
     `{orgId, isDeleted≠true, $or:[{externalSource.gwCamId:id},{camId:id}]}`. (Reuse the existing
     `FindByCamIDAndOrg` + `FindByGWCamId` if a single `$or` is undesirable; one query preferred.)
   - `AdoptAsGWManaged(ctx, camId, ext ExternalSource) error` — set
     `externalSource.{provider:"gw", gwCamId:id, sourceFamily, syncState:"active"}` on the row whose `camId==id`
     **only when `externalSource.gwCamId` is currently empty** (idempotent; never clobbers an already-managed row).
2. **Service** (`gwdevicesync/service.go`):
   - `GetDeviceSummary` ([:319](../../../../home/klynx/klynx-api/internal/services/gwdevicesync/service.go#L319)):
     look up via `FindByGWCamIdOrCamId`. If matched by `camId` with empty `gwCamId`, call `AdoptAsGWManaged`
     before returning the summary. Result: the Dahua event enriches from **LRP** and **never** falls through to
     `SyncFromIngestEvent` → no duplicate is created.
   - `SyncFromGW` step [2] ([:101-105](../../../../home/klynx/klynx-api/internal/services/gwdevicesync/service.go#L101-L105)):
     load `existing` via the same OR-lookup, so the Kafka path (once RC-2 is fixed) also adopts rather than inserts.
3. **Guard / safety:** `camId`-match only fires when a row with that exact `camId` exists. AIBOX channel-keyed
   deviceIds (e.g. `"56"`) are not UUIDs and won't collide with a `camId`, so legacy auto-register is unaffected.
   Keep `uuid.New()` creation only for genuinely-unknown devices (no gwCamId AND no camId match).

### B) gateway-api — fix `gw.devices.changed.v1` producer schema (SECONDARY)

Rewrite `publishDevicesChanged` ([`service.go:88-105`](../../internal/services/devicemgmtsvc/service.go#L88-L105))
to emit the schema the klynx consumer actually decodes:

```go
payload := map[string]any{
    "eventId":       uuid.NewString(),
    "syncOrigin":    "gw",
    "orgId":         d.WorkspaceId,   // klynx camera.orgId == gw workspaceId
    "gwWorkspaceId": d.WorkspaceId,
    "remoteDeviceId": d.DeviceId,     // = camId (join key) — was wrongly `entityId`
    "deviceMgmtId":  d.DeviceMgmtId,
    "sourceFamily":  d.SourceFamily,
    "changeType":    mapChangeType(action), // create→created, update→updated, delete→deleted
    "name":          d.Name,
    "lat":           d.Lat,
    "lng":           d.Lng,
    "status":        true,
    "occurredAt":    time.Now().UTC(),
}
```
- Add `mapChangeType` (create→created / update→updated / delete→deleted).
- `remoteDeviceId = d.DeviceId` (the canonical camId alias), not `entityId` — for a provisioned camera they are
  equal, but `deviceId` is the contractually-correct join field. Carry `deviceId` too for forward-compat.
- Keep the publish non-blocking (goroutine) and best-effort.

### C) klynx-api — register→Provision (ALREADY IMPLEMENTED — no work)

Provisioning on create/edit is already wired (`controllers/deviceapi/camera.go`, camID Phase 2/3.2). No change
needed. Once B ships, this path's emitted `gw.devices.changed.v1` becomes consumable and backfills the projection
proactively (before the first ingested event). Nothing to implement here.

---

## Migration — existing duplicate (`8186551a` + LRP `bab573c0`)

One-off, idempotent (run after A ships):
1. **Backfill** LRP: set `externalSource.{provider:"gw", gwCamId:"bab573c0-…", sourceFamily:"dahua", syncState:"active"}`
   on `camId == bab573c0-…` (exactly what `AdoptAsGWManaged` does — first event after deploy self-heals it).
2. **Retire** `8186551a-…`: soft-delete (`isDeleted:true`). `event_refs.deviceId == bab573c0-…` already equals
   LRP's camId, so history joins to LRP after the backfill; nothing references `8186551a`'s uuid camId.
3. Verify camera list shows **one** Dahua row (LRP) with `monitorState` intact.

---

## Rollout order

1. **klynx-api A** (adopt-by-camId) — stops new duplicates immediately on the live ingest path. Deploy first.
2. **Migration** — backfill LRP + retire `8186551a`.
3. **gateway-api B** (producer schema) — revives gw→klynx metadata enrichment (name/lat/lng/status) AND makes the
   already-wired register→Provision path (C) actually populate the projection. Independent of A.

A is safe without B; B is safe without A (no consumer behavior change beyond "events now decode"). C needs no work.

---

## Test plan

- **klynx unit:** `FindByGWCamIdOrCamId` matches by gwCamId, by camId, and `$or` precedence; `AdoptAsGWManaged`
  is idempotent + no-clobber when gwCamId already set.
- **klynx flow:** native camera (camId=X, no externalSource) + normalized event deviceId=X →
  `GetDeviceSummaryForIngest` returns LRP's summary, backfills externalSource, **does NOT** call
  `SyncFromIngestEvent`; assert exactly one camera row.
- **klynx regression:** AIBOX channel-keyed deviceId (`"56"`, no camId match) still auto-creates (no behavior change).
- **gateway-api unit:** `publishDevicesChanged` emits `remoteDeviceId/changeType/orgId/gwWorkspaceId/deviceId`;
  round-trip JSON → `eventbridge.DeviceChangedEvent` populates `RemoteDeviceID/ChangeType/OrgID` non-empty.
- **klynx consumer:** decode a real gw payload → `SyncFromGW` no longer early-returns on empty `RemoteDeviceID`.
- `gofmt` / `go vet` / `go build ./...` / `go test ./...` in both repos.

---

## Risks

- **Adopt clobber:** must NOT overwrite an existing `externalSource.gwCamId` (different gw device colliding on a
  camId). Guard = adopt only when current `gwCamId` is empty.
- **`$or` index:** `{orgId, externalSource.gwCamId}` is unique-sparse; the camId leg uses the existing unique
  `camId` index. Confirm the `$or` plan uses both indexes (no collscan on the hot path) — else split into two finds.
- **B field semantics:** klynx `OrgID` vs `gwWorkspaceId` — both set to `d.WorkspaceId` (platform invariant
  klynx.org == gw.workspace). Validate against a real `HandleNormalized` orgId before shipping.
- **Replay:** `SyncFromGW` freshness guard (sourceVersion/OccurredAt) still applies after B; verify adopted rows
  get `LastSyncedAt` so later real events aren't dropped as stale.

---

## Review gate

Review before implementing — focus: (1) adopt-by-camId honoring contract §3 join key without clobbering managed
rows; (2) `$or` lookup index safety on the ingest hot path; (3) producer-schema field mapping correctness
(`remoteDeviceId=deviceId`, `changeType` enum, `orgId`/`gwWorkspaceId`); (4) migration safety for in-flight
duplicates; (5) rollout independence (A without B). Verdict line required.
