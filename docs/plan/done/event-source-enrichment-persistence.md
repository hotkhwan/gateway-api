# Event Source Enrichment Persistence — `deviceMgmtId` + `sn` in event_refs / event_details (revised)

**Date:** 2026-05-18
**Status:** Draft (rev 3 — post-Codex-round-3, architecturally ready per Codex; minor wording cleanup applied)
**Review Status:** Pending Codex Review
**Feature Owner Backend:** `gateway-api` (override — default `klynx-api`; see §2A)
**Related Repos:** `gateway-api`, `klynx-api`
**Canonical Contract:** [`klynx-api/docs/contracts/klynx-kafka-consumer.md`](../../../klynx/klynx-api/docs/contracts/klynx-kafka-consumer.md) §6.1 — projection persistence declaration patched in this session (Codex round-2 decision: hub contract for `event_refs` schema lives at klynx-kafka-consumer, not `delivery-topic-unification.md`)
**Supersedes:** rev 1 (rejected — scope contradictions, wrong code-path target, stale "add model field" task), rev 2 (rejected — klynx-api hub contract update was still pending)
**Depends on:** none — schema-additive storage-only

---

## 1. Executive Summary

**Storage-only plan.** Goal: persist `deviceMgmtId` and `sn` from the normalized event into the two consumer-side documents that already exist (`gateway-api.event_details` and `klynx-api.event_refs`). Both documents currently drop these fields because the consumer code paths do not propagate the enriched values into the persisted struct — even though the underlying model types already have the field slots.

This plan is intentionally narrow: no API surface, no DTO change, no index, no read-path. Klynx-side queries against `event_refs` (the user's stated use case — "klynx ใช้ sync edge device") are direct MongoDB reads in klynx-api — they do not require new gateway-api endpoints. Adding the fields to gRPC/REST DTOs (`EventSourceInfo`, `EventRefView`, `EventDetailDTO`) is **explicitly deferred** to a follow-up plan if a future feature actually consumes them.

**Producer side is already correct:** `gw.events.normalized.v1` payload carries `source.deviceMgmtId` + `source.sn` + `source.edgeName` (verified in `internal/kafka/normalizedcons/consumer.go:516-533`). What is broken is the **other** consumer write path (the `event_details` upsert at line 205), which writes a different document built from `canonical.Source` — and `canonical.Source` is not populated with the enriched values before that upsert.

---

## 2. Scope

### In Scope

- `gateway-api`: backfill `canonical.Source.DeviceMgmtId` and `canonical.Source.SN` in the `raw.events` consumer (`internal/kafka/normalizedcons/consumer.go`) before the `event_details.Upsert` call so the persisted `event_details` document includes both fields under `source`
- `klynx-api`: add `DeviceMgmtID` + `SN` to the `EventRef` struct, populate them from `ev.Source.DeviceMgmtID` + `ev.Source.SN` in `handleNormalized.go`
- klynx-api hub contract update: declare `event_refs.deviceMgmtId` and `event_refs.sn` as projection-persisted fields — **already authored** in `klynx-api/docs/contracts/klynx-kafka-consumer.md` §6.1 (Codex round-2 picked klynx-kafka-consumer as canonical owner; commit `121e3e2` on klynx-api branch `chore/cross-repo-contract-symmetry`)

### Out of Scope

- Any new query / filter / index — `deviceMgmtId` and `sn` will be present in documents but not searchable by API until a separate plan declares the read-path
- DTO / response shape changes — `EventSourceInfo` (`internal/grpc/eventservice/server.go:53`), `EventRefView`, `EventDetailDTO` are **not** modified. Codex called this out; we are honoring it by deferring entirely
- Backfill of historical documents — additive `omitempty` only; old rows stay sparse
- Adding `edgeName` persistence — already on the wire but not requested by the user; not in this plan
- Producer-side change at `internal/services/ingestsvc/ingest.go:629-646` — the producer publishes `CanonicalEvent` without `DeviceMgmtId`/`SN` on `Source`, but the consumer is the cheaper fix point because the resolver already runs there. Producer enrichment is left as a possible future cleanup, not required for this plan

### Success Criteria

- New `event_details` documents include `source.deviceMgmtId` and `source.sn` whenever the resolver finds a `device_management` record and the raw payload carries `sn`
- New `event_refs` documents include `deviceMgmtId` and `sn` whenever the normalized event carries them in `source`
- Old documents remain readable (fields absent → empty / `omitempty` keeps them out of the bson)
- klynx-side admin user can run a direct mongo query `db.event_refs.find({deviceMgmtId:"23395e49-..."})` and get matching rows for events ingested after this rollout
- Codex review approves both this revised plan and the klynx-api hub contract update

---

## 2A. Standard Cross-Repo Context

| Context | Default Value | Applies? | Notes |
|---|---|---|---|
| Feature owner backend | `klynx-api` | **no** | Override: `gateway-api` owns the producer topic; this change ensures both consumer projections persist what the producer already emits |
| Events system of record | `gateway-api` | yes | `event_details` is gateway-api's canonical store |
| Klynx normalized event consumer | `klynx-api` via `gw.events.normalized.v1` | yes | `event_refs` is klynx-api's projection |
| Device/camera identity and sync state source of truth | `gateway-api/device_management` | yes | `deviceMgmtId` is the FK into this SoR |
| Klynx camera model | projection / consumer model | yes | klynx will be able to resolve `event_refs.deviceMgmtId` back to `klynx.cameras.externalSource.gwDeviceMgmtId` via mongo query |
| Frontend contract rule | FE must not guess schema | yes | this plan does not add any FE-visible schema; FE remains unaffected |

---

## 3. Current State

### Current Backend Flow

```text
ingest → gateway-api ingest.go publishes CanonicalEvent to raw.events
  → CanonicalEvent.Source contains only {DeviceId, DeviceType, DeviceName, DeviceDescription, WorkspaceId}
  → DeviceMgmtId + SN are NOT set on canonical.Source at publish time
    (verified: internal/services/ingestsvc/ingest.go:635-641)

gateway-api normalizedcons consumer (raw.events):
  → unmarshal CanonicalEvent — canonical.Source.DeviceMgmtId="" canonical.Source.SN=""
  → device_management resolver runs at line 113-117 → returns dm.DeviceMgmtId into LOCAL VAR
  → LOCAL VAR is never written back to canonical.Source
  → builds final NormalizedEvent: event.Source = canonical.Source  (line 170)
  → event_details upsert at line 205 — persists Source with empty deviceMgmtId/sn
  ❌ persisted: source={deviceId, deviceType, workspaceId} — deviceMgmtId+sn DROPPED

gateway-api normalizedcons consumer also publishes gw.events.normalized.v1:
  → builds SEPARATE bridge struct at lines 515-533 with src.DeviceMgmtID = deviceMgmtId AND src.SN from canonical.Payload["sn"]
  → publishes enriched event to klynx
  ✅ wire has the fields

klynx-api gwdeliverycons / handleNormalized.go:
  → reads gw.events.normalized.v1
  → ev.Source.DeviceMgmtID is set (line 51 reads it for auto-sync)
  → builds EventRef at line 57-69 — DeviceMgmtID/SN NOT included in the upsert payload
  ❌ persisted: event_refs row has eventType/deviceId/orgId/workspaceId/sourceFamily etc but no deviceMgmtId/sn
```

### Current Constraints

- `models/ingestmod/normalization.go:36-37` (`SourceInfo`) already has `DeviceMgmtId` and `SN` fields — **schema is correct; the bug is purely the assignment gap in the consumer flow**
- `internal/kafka/normalizedcons/consumer.go:111-117` resolves `deviceMgmtId` to a local variable but does not assign back to `canonical.Source.DeviceMgmtId`; `sn` is not resolved at all in this code path
- `internal/repo/eventrefsrepo/repo.go:21-37` (klynx side) `EventRef` struct has no `DeviceMgmtID` / `SN` field — needs additive struct extension
- `internal/services/ingestsvc/handleNormalized.go:48-69` (klynx side) builds the upsert payload without `DeviceMgmtID` / `SN` fields

### Current Risks

- Today: klynx cannot uniquely identify which gateway-api camera produced an event by `deviceMgmtId` (only by `deviceId` which is the edge-local channel string — not unique across edges)
- Today: klynx cannot disambiguate events from two edge boxes that emit overlapping `entityId` in the same workspace (the user's exact concern — `sn` is the discriminator)
- After fix: only new documents have the fields. Operator must accept that historical documents stay sparse (no backfill in this plan)

---

## 4. Ownership Model

### Feature Owner Backend

- `gateway-api` owns the topic schema. klynx-api consumes and projects. Both must be amended to persist what the topic already carries.

### System of Record by Domain

| Domain | System of Record | Notes |
|---|---|---|
| Event canonical detail | `gateway-api/event_details` | adds `source.deviceMgmtId`, `source.sn` to persisted struct |
| Event projection (klynx) | `klynx-api/event_refs` | adds `deviceMgmtId`, `sn` (flat — matches existing `event_refs` shape) |
| Device identity (resolves `deviceMgmtId`) | `gateway-api/device_management` | unchanged |
| Edge serial origin (`sn`) | edge device raw payload | unchanged; the resolver already extracts via `canonical.Payload["sn"]` in the bridge path |

### Producer / Consumer Mapping

| Asset | Producer | Consumer | Notes |
|---|---|---|---|
| `gw.events.normalized.v1` `source.deviceMgmtId` | gateway-api normalizedcons (`buildBridgeEvent`) | klynx-api `handleNormalized` | already emitted; **klynx persist gap** |
| `gw.events.normalized.v1` `source.sn` | gateway-api normalizedcons | klynx-api `handleNormalized` | same |
| `event_details.source.deviceMgmtId` | gateway-api normalizedcons `Upsert` | none (mongo-only) | **gateway persist gap** — `canonical.Source` not enriched before upsert |
| `event_details.source.sn` | same | same | same |

### Canonical Store / Projection Store

| Data | Canonical Store | Projection Store | Notes |
|---|---|---|---|
| `source.deviceMgmtId` | `gateway-api/event_details.source.deviceMgmtId` | `klynx-api/event_refs.deviceMgmtId` | additive both sides |
| `source.sn` | `gateway-api/event_details.source.sn` | `klynx-api/event_refs.sn` | additive both sides |

---

## 5. Proposed Architecture

### Target Flow

```text
gateway-api normalizedcons consumer:
  → unmarshal CanonicalEvent (canonical.Source.DeviceMgmtId / SN still empty)
  → device_management resolver runs
  → IF resolver returns a record:
      canonical.Source.DeviceMgmtId = dm.DeviceMgmtId   (NEW backfill at ~line 117)
  → extract sn from canonical.Payload["sn"] → canonical.Source.SN  (NEW at same location)
  → event.Source = canonical.Source                                 (existing — line 170 — now carries enriched values)
  → event_details.Upsert(event)                                     (existing — line 205)
  ✅ persisted document includes source.deviceMgmtId + source.sn

gateway-api normalizedcons publish to gw.events.normalized.v1:
  → buildBridgeEvent — unchanged (already includes deviceMgmtId/sn at lines 515-533)
  ✅ wire unchanged

klynx-api handleNormalized.go:
  → unmarshal NormalizedEvent — ev.Source.DeviceMgmtID and ev.Source.SN are set
  → build EventRef:
      ref.DeviceMgmtID = ev.Source.DeviceMgmtID   (NEW)
      ref.SN = ev.Source.SN                       (NEW)
  → UpsertRef(ctx, ref)                            (existing)
  ✅ event_refs row has both fields
```

### API / Event Surface

- Added REST endpoints: **none**
- Updated REST endpoints: **none**
- Added Kafka topics: **none**
- Updated Kafka topics: **none** (`gw.events.normalized.v1` schema unchanged — fields already present on the wire)
- Added MQTT topics: **none**
- Added Redis keys: **none**
- Added sync rules: **none**

### Artifact Output

- Plan: `docs/plan/event-source-enrichment-persistence.md` (this revision)
- Canonical contract patch: `klynx-api/docs/contracts/klynx-kafka-consumer.md` §6.1 — **DONE in this session** (added "Projection persistence — `event_refs`" subsection that declares `deviceMgmtId` + `sn` and lists all projection fields with BSON tags). Codex round-2 picked klynx-kafka-consumer as the canonical owner (over delivery-topic-unification)

---

## 6. Contract Summary

| Surface | Method / Topic | Auth | Request | Success Response | Error Contract |
|---|---|---|---|---|---|
| `event_details.source` (mongo doc) | n/a | n/a | n/a | adds `deviceMgmtId: string?`, `sn: string?` (`omitempty`) | n/a |
| `event_refs` (mongo doc) | n/a | n/a | n/a | adds `deviceMgmtId: string?`, `sn: string?` (`omitempty`) | n/a |

### Frontend Impact Summary

- FE is unaffected. `EventSourceInfo` gRPC and any REST event DTOs are explicitly **not modified** by this plan. If a future FE feature needs to display or filter by `deviceMgmtId` / `sn`, that requires a separate plan with DTO/index/handler work.

---

## 7. Field Ownership and Sync Rules

### Field Ownership Matrix

| Field | Authoritative Writer | Allowed Initiator | Replicated To | Conflict Rule |
|---|---|---|---|---|
| `event_details.source.deviceMgmtId` | gateway-api normalizedcons | only consumer write path | n/a (event_details is canonical leaf) | events are immutable per `eventId`; no real-world overwrite |
| `event_details.source.sn` | same | same | n/a | same |
| `event_refs.deviceMgmtId` | klynx-api `handleNormalized` | only consumer write path | n/a | `UpsertRef` is idempotent by `eventId`; same value on repeat |
| `event_refs.sn` | same | same | n/a | same |

### Write Authority Policy

- Each side writes independently from the same Kafka source. No dual-write conflict.
- Both writes are downstream consumers of `gw.events.normalized.v1`. Producer (gateway-api ingest) is unchanged.

### Conflict Resolution / Idempotency

- `event_details.Upsert` is keyed by `eventId` — re-delivery overwrites with same values
- `event_refs.UpsertRef` is keyed by `eventId` — same

---

## 8. Cross-Repo Impact

### Backend Repos

| Repo | Change Type | Required Work |
|---|---|---|
| `gateway-api` | consumer code fix (no model change — fields already exist) | edit `internal/kafka/normalizedcons/consumer.go` to assign enriched values back to `canonical.Source` before line 170 (see §10 for exact lines) |
| `klynx-api` | repo struct field + handler map + contract update | add `DeviceMgmtID` + `SN` `bson:"omitempty"` to `EventRef`; populate in `handleNormalized.go`; amend klynx-api hub contract for projection field declaration |

### Frontend Repos

| Repo | Change Type | Required Work |
|---|---|---|
| `klynx` (FE1) | none | reads `event_refs` via klynx-api APIs; no DTO change today |
| `gateway-portal` (FE2) | none | reads `event_details` via gateway-api APIs; no DTO change today |

### Breaking Change Assessment

- Backward compatible? **yes** — additive `omitempty` BSON fields on documents
- Compatibility window? n/a — old rows stay valid, no migration needed
- Which repo ships first? gateway-api (canonical store gets fields first) then klynx-api (projection catches up) — order is operationally optional since each side is independent

---

## 9. Rollout Plan

### Phase Sequencing

1. Codex review on this plan
2. `gateway-api` PR — consumer.go fix + tests → deploy → observe new `event_details` rows
3. `klynx-api` PR — `EventRef` struct + handler map + hub contract update → deploy → observe new `event_refs` rows
4. Operator may opt into a one-off replay of recent events if a query path needs the new fields on historical rows (out of scope here)

### Cross-Repo Rollout Order

1. `gateway-api` PR merges first (small, isolated fix)
2. `klynx-api` PR follows (struct + contract update)

### Review Gate

- Codex review status: rev 2 architecturally approved with cleanup ask; rev 3 applies the cleanup (this revision)
- Blocking issues from rev 1 — addressed in rev 2:
  - ✅ Scope tightened to storage-only — DTO/API surface explicitly deferred
  - ✅ Success criterion no longer says "query event_refs by deviceMgmtId via API" — only direct mongo query
  - ✅ Implementation checklist points at the real bug (`consumer.go:117` local-var assignment) — not "add model field" (already exist at `normalization.go:36-37`)
  - ✅ gRPC `EventSourceInfo` and REST event DTOs intentionally left untouched
- Blocking issue from rev 2 — addressed in rev 3:
  - ✅ klynx-api hub contract patch authored in this session — `klynx-kafka-consumer.md` §6.1 now contains the canonical projection persistence declaration. The plan's success criterion that "Codex review approves the hub contract" is no longer a deferred dependency — the patch is in the working tree, co-reviewed with this plan
- High-risk assumptions: producer side at `ingest.go:635-641` does not set `Source.DeviceMgmtId`/`SN` on the canonical event published to `raw.events`, so the consumer-side fix is what actually moves the value into `canonical.Source` (via the resolver). If a future plan moves the resolve upstream into the producer, this fix becomes a no-op but is still safe

### Deployment / Migration Notes

- No migration; `omitempty` keeps old documents valid
- No new indexes

---

## 10. Implementation Checklist

### Backend Owner Repo (`gateway-api`)

- [ ] **Fix consumer write path** — `internal/kafka/normalizedcons/consumer.go` around lines 113-131:
  - After line 117 `deviceMgmtId = dm.DeviceMgmtId`, add:
    ```go
    if canonical.Source.DeviceMgmtId == "" {
        canonical.Source.DeviceMgmtId = dm.DeviceMgmtId
    }
    ```
  - Add SN extraction near the same block, gated on empty source-level value:
    ```go
    if canonical.Source.SN == "" {
        if v, ok := canonical.Payload["sn"].(string); ok && v != "" {
            canonical.Source.SN = v
        }
    }
    ```
  - This must execute **before** line 170 `event.Source = canonical.Source` so the persisted `event_details` document carries the enriched values.
- [ ] **No model change required** — `models/ingestmod/normalization.go:36-37` already has `DeviceMgmtId` and `SN` on `SourceInfo`
- [ ] **Test**: extend `internal/kafka/normalizedcons/consumer_test.go` with two cases:
  - Resolver returns a `device_management` record → persisted `event_details` doc has `source.deviceMgmtId` populated
  - Raw payload has `sn` key → persisted doc has `source.sn` populated
- [ ] **Test**: payload without `sn` and resolver returns nil → persisted doc omits both fields (no `null`/empty-string overwrite)

### Upstream / Peer Backends (`klynx-api`)

- [ ] Add fields to `internal/repo/eventrefsrepo/repo.go` `EventRef` struct (after line 33):
  ```go
  DeviceMgmtID  string `bson:"deviceMgmtId,omitempty"`
  SN            string `bson:"sn,omitempty"`
  ```
- [ ] Populate in `internal/services/ingestsvc/handleNormalized.go` around lines 57-69 — add inside the `ref := &eventrefsrepo.EventRef{...}` literal:
  ```go
  DeviceMgmtID: ev.Source.DeviceMgmtID,
  SN:           ev.Source.SN,
  ```
- [ ] Test: `eventrefsrepo` round-trip with non-empty `DeviceMgmtID` + `SN`
- [ ] Test: `handleNormalized` with `ev.Source.DeviceMgmtID="..."` and `ev.Source.SN="..."` → persisted row has both
- [x] Amend klynx-api hub contract — **DONE in this session**: `klynx-api/docs/contracts/klynx-kafka-consumer.md` §6.1 now contains canonical projection persistence declaration with full field list, BSON tags, and cross-link back to this plan. Codex round-2 picked klynx-kafka-consumer (not delivery-topic-unification) as the canonical owner

### Frontend

- [ ] None
