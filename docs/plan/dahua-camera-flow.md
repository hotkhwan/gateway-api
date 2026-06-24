# Plan — Dahua camera identity + ANPR field mapping (per-camera ingest)

**Date:** 2026-06-24
**Owner Backend:** `gateway-api` (ingest + normalize + device_management SoR) — spoke; `klynx-api` (camera registration SoR + provisioning) — hub side
**Repos touched (4):** `gateway-api`, `klynx-api` (kapi), `klynx` (Nuxt FE), `gateway-portal` (SvelteKit FE)
**Status:** Reviewed 2026-06-24 — **REQUEST CHANGES applied** (cross-repo decomposition corrected against actual code; v2). Pending re-review, then implement. Do NOT implement before review (AGENTS.md).

---

## ⚠️ Review findings (v1 → v2 corrections, verified in code)

The v1 plan was gateway-api-heavy and assumed klynx work that **already exists**:
- **Camera registration with RTSP/ONVIF creds ALREADY EXISTS** — klynx-api `POST /resources/camera` (`models/devmod/camera.go` stores `URL`/`User`/`Password` AES-encrypted) + klynx FE `app/pages/systemDevices/cameras/add`. Do NOT rebuild — **leverage it**.
- **"Enable AI" ALREADY EXISTS** — klynx-api `POST /resources/camera/{camId}/ai-analyze` (`camera.aiAnalyze`) + klynx FE toggle. Klynx-owned, never pushed to gw. Reference, don't rebuild.
- **gateway-portal owns the Dahua mapping-template + ingest device_management UI** (`ingest/deviceManagement`, mapping templates) — v1 omitted it; the ANPR field-mapping editor belongs here.
- **Identity is split:** klynx-api = SoR for locally-registered cameras (have RTSP); gateway-api = SoR for `device_management` (born from events / sync). Today klynx **does not push camera creation to gw** — only overlay edits (name/desc/lat/lng) via `cameraoverlay` gRPC, plus inbound gw→klynx `DeviceChangedEvent` Kafka sync. → The real new backend gap is **proactive gw device-identity provisioning keyed by camID** (so the camID on the path resolves at ingest), not "build camera registration".
- `eventIngestUri` is **org-level** (`/events/{orgId}/`) today — there is no per-camera EventAutoUpload provisioning yet.

> Follow-up to the Dahua ingest enablement (gw-api 3.18.0 BodyRaw fix, 3.19.0 multipart parser, 3.20.0 comingSoon→active migration). Those made Dahua events flow end-to-end (`/events/{orgId}/dahua` → parser → catch-all template → `raw.events` → `gw.events.normalized.v1`). This plan adds **per-camera attribution** and **real ANPR field mapping**.

---

## Problem (verified on production 2026-06-24)

1. **No reliable per-camera identity.** Multiple Dahua cameras (192.168.1.11, .12, …) all POST to the same `/events/{orgId}/dahua`. The payload carries **no hardware unique id** — verified against a live `raw.events` ANPR (`TrafficJunction`) message:
   - `Channel = 0` (single-channel IPC → always 0)
   - `Events[0].Data.TrafficCar.MachineName = "Dahua001"` (operator-configurable, not hardware-bound, not guaranteed unique)
   - `Events[0].Data.TrafficCar.DeviceAddress / MachineAddress / MachineGroup = " "` (blank)
   - `Events[0].Data.Vehicle.SerialUUID = ""` (vehicle serial, not the camera)
   - **No SN / MAC / DeviceID field anywhere.**
   - Only discriminator is the HTTP `sourceIp` — gateway-side only, not in the event, and unstable (DHCP / NAT collision).
   → Auto-register-camera cannot attribute an event to a specific camera. **Decision: move the camera id into the ingest path.**

2. **ANPR fields not mapped.** The normalized `payload` is `{}` because the catch-all template has no `mappings`. The real fields are nested under the `Events[]` **array** (`Events[0].Data.TrafficCar.*`), and the current mapper [`getNestedValue`](../../internal/services/ingestsvc/templateMatch.go) only walks **maps, not array indices** — so `Events.0.Data.TrafficCar.VehicleColor` cannot be mapped today.

## Real Dahua ANPR payload shape (reference)
```
{ Ack, Channel, EOF, Events: [ {
    Action: "Pulse", Code: "TrafficJunction",
    Data: {
      RealUTC: 1771424063,                      // unix sec → occurredAt
      Object: { ObjectType: "Plate", BoundingBox, Image:{Offset,Length} },
      TrafficCar: { PlateNumber, PlateColor, VehicleType|Category, VehicleColor:"Black",
                    VehicleBrand, MachineName:"Dahua001", Direction, Event:"TrafficJunction" },
      SceneImage: { Offset, Length, Width, Height }   // JPEG (stripped by parser → _binaries)
    } } ] }
```
(Plate/VehicleType are `null` when ANPR doesn't read a plate that frame; populated on a clean read.)

---

## Scope

### A) gateway-api — per-camera ingest path
1. **Route:** add `/events/:orgId/:sourceFamily/:camID` alongside the existing `/events/:orgId/:sourceFamily` (camID optional, backward-compatible — existing un-cammed cameras keep working).
2. **Controller/service:** thread `camID` into `IngestService.Ingest(...)`. When present:
   - resolve the camera (device-management record) by `camID` instead of auto-registering by best-effort payload fields;
   - set `source.deviceId = camID` (and device-mgmt enrichment: name/site/zone/lat-lng) so every event attributes deterministically;
   - when absent, keep today's auto-register/best-effort path (no regression).
3. **Contract:** document the `{camID}` segment + attribution semantics.

### B) gateway-api — ANPR field mapping
1. **Mapper array support:** extend [`getNestedValue`](../../internal/services/ingestsvc/templateMatch.go) / `setNestedValue` to resolve numeric segments as array indices (`Events.0.Data.…`). General fix; benefits all sources; same mapper runs again in the normalizer so it works end-to-end. (Alt: parser flattens Dahua `Events[0]` — rejected: Dahua-specific, lossy on multi-event arrays.)
2. **Dahua default template (replaces catch-all, higher `priority`):**
   - `occurredAt` ← `Events.0.Data.RealUTC` (transform `timestamp`)
   - `payload.plate` ← `Events.0.Data.TrafficCar.PlateNumber`
   - `payload.plateColor` ← `Events.0.Data.TrafficCar.PlateColor`
   - `payload.vehicleType` ← `Events.0.Data.TrafficCar.VehicleType` (fallback `Category`)
   - `payload.vehicleColor` ← `Events.0.Data.TrafficCar.VehicleColor`
   - `payload.vehicleBrand` ← `Events.0.Data.TrafficCar.VehicleBrand`
   - `payload.direction` ← `Events.0.Data.TrafficCar.Direction`
   - `payload.eventCode` ← `Events.0.Code`
   - `classificationRules`: e.g. `eventClass=traffic`; `severity` by plate/blacklist match (define vocab with hub).
3. **Multi-event arrays:** Dahua may batch `Events: [...]`. Decide: map `Events[0]` only (v1) vs. fan-out one canonical event per array element (needs ingest loop). Recommend v1 = first element, log when `len>1`.

### C) klynx-api (kapi) — provisioning glue (mostly NEW; register/AI already exist)
**Reuse (exists):** camera registration (`POST /resources/camera`, RTSP/ONVIF creds), "Enable AI" (`POST /resources/camera/{camId}/ai-analyze`).
**New:**
1. On camera register (or a new "enable event ingest" action), **proactively provision a gateway-api device identity** keyed by the klynx `camId` — a NEW gw call beyond the existing overlay (today klynx only overlays existing gw cameras). This pre-creates the gw `device_management` record with `DeviceId = camID` so the camID-path resolves at ingest.
2. **Generate the per-camera EventAutoUpload target** `https://{host}/events/{orgId}/dahua/{camID}` (HTTPS, mode HTTP) — derived from the org `eventIngestUri` + the camID — returned to FE for the operator to paste into the Dahua camera (Event → EventAutoUpload).
3. Keep the existing gw→klynx `DeviceChangedEvent` sync + `cameraoverlay` consistent with the new pre-provisioned identity (avoid duplicate records vs. auto-register).

### D) gateway-portal (FE) — Dahua mapping-template editor + device_management alignment
1. Mapping-template UI for `dahua` (gw-portal already owns ingest templates + `ingest/deviceManagement`): expose the new array-path mappings (plate/vehicleType/vehicleColor/occurredAt) + classificationRules, with the Dahua field catalog from this plan.
2. Align the abstract `device_management` view (`sourceFamily/entityType/entityId`) with the camID attribution so `entityId`/`deviceId` reflects the camID.

### E) klynx (FE) — per-camera EventAutoUpload surfacing
Camera register + Enable-AI screens **exist**. New: on the camera page, surface/"copy" the EventAutoUpload URL (`/events/{org}/dahua/{camID}`) so the operator can paste it into the Dahua camera. (No new register/AI UI.)

### F) Snapshots (Phase 2, deferred)
Parser currently drops JPEG bytes (keeps `_binaries` descriptors). Phase 2: upload `SceneImage`/plate crop to S3, populate `binaryRefs` on the normalized event.

## Out of scope
Hikvision (stays comingSoon), multi-event fan-out (decision pending), rebuilding camera-register/Enable-AI (exist), S3 snapshot upload (Phase 2).

## Files to change (gateway-api)
- `router/ingest.go` — add `/events/:orgId/:sourceFamily/:camID` route.
- `controllers/ingestapi/ingest.go` — read `camID` param, pass to service.
- `internal/services/ingestsvc/ingest.go` — `Ingest(...camID...)`; when camID present resolve `device_management` by it and set `source.deviceId`; skip weak auto-register.
- `internal/services/ingestsvc/templateMatch.go` — array-index support in `getNestedValue`/`setNestedValue` (+ tests).
- `internal/grpc/...` + `internal/services/devicemgmtsvc` — NEW inbound "provision device identity by camID" surface so klynx-api can pre-create the `device_management` record (mirrors the existing `cameraoverlay` gRPC registration pattern). Must dedupe against auto-register.
- Dahua `mapping_templates` doc (seed/admin) — real field mappings + classificationRules, `priority` above the catch-all.
- Tests beside code.

## Cross-repo decomposition (4 repos) + contract
| Repo | Work |
|---|---|
| **gateway-api** | camID route; camID→device_management resolution; NEW inbound device-identity provisioning (by camID); mapper array support; Dahua template. SoR for device_management. |
| **klynx-api (kapi)** | NEW: proactive gw device-identity provisioning on register; per-camera EventAutoUpload URL gen. REUSE: camera register (rtsp/onvif) + Enable-AI (exist). |
| **klynx (FE)** | NEW: surface/copy EventAutoUpload URL on camera page. REUSE: register + AI toggle (exist). |
| **gateway-portal (FE)** | NEW: Dahua mapping-template editor (array paths + classification); align `device_management` view with camID. |

- **Contract authority:** author a hub contract in `klynx-api/docs/contracts/<name>.md` for the full flow (camera register → proactive gw device-identity → per-camera EventAutoUpload → camID-path ingest → device attribution), per CLAUDE.md hub-and-spoke. gateway-api remains SoR for `device_management` identity.
- **Cross-repo order:** (1) klynx-api provisions gw device-identity (camID) + returns EventAutoUpload URL; (2) operator sets it on the Dahua camera; (3) camera POSTs `/events/{org}/dahua/{camID}`; (4) gateway-api resolves camID → attributes + maps; (5) gw→klynx `DeviceChangedEvent` sync stays consistent.

## Risks
- **Backward compat:** the no-camID path must keep working (existing cameras) — camID optional.
- **Mapper array change** touches a hot path used by both ingest match + normalizer; cover with tests (index in range / out of range / non-array / nested).
- **Plate often null** (no read that frame) — `required:false` mappings; don't drop the event.
- **MachineName as fallback id** is unreliable — do NOT use it for attribution; camID path is the source of truth.
- **Multi-event batches** — undercount if only `Events[0]` mapped; log + decide fan-out.
- **Identity split / duplicate records** — klynx-local cameras vs gw `device_management` (auto-register + gw→klynx sync). Proactive provisioning (DeviceId=camID) must dedupe against the reactive auto-register key `(sourceFamily,entityType,entityId)` so one camera ≠ two records. Define the canonical key with the hub contract.
- **Migration of in-flight cameras** — cameras already auto-registered (pre-camID) need back-fill of `DeviceId=camID` when the operator switches their EventAutoUpload to the camID path.

## Test plan
- Route: `/events/{org}/dahua/{camID}` attributes `source.deviceId=camID`; `/events/{org}/dahua` still works.
- Mapper: `Events.0.Data.TrafficCar.VehicleColor` resolves; out-of-range/non-array → not-found (no panic).
- Template: live ANPR sample → normalized `payload` has plate/vehicleType/vehicleColor + `occurredAt` from RealUTC.
- `gofmt` / `go vet` / `go build ./...` / `go test ./...`.

## Review gate
Review before implementing — focus: backward-compat of the optional camID segment, mapper array-index correctness on the shared hot path, camID↔device-management ownership (gateway-api SoR), multi-event decision, cross-repo order (klynx registers first). Verdict line required.
