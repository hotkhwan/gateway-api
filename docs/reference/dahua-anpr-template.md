# Reference — Dahua ANPR (TrafficJunction) MappingTemplate

Concrete field mappings for the `dahua` source family, derived from a **verified
full `TrafficJunction` / `VehicleDetect` payload** (2026-06-24, camera `Dahua001`).
Requires the array-index mapper (gateway-api ≥ 3.21.0) — source paths reach into
the `Events[]` array and into array elements (e.g. `DrivingDirection.0`).
See `docs/plan/dahua-camera-flow.md`.

The mapped output becomes the normalized event `payload`. The taxonomy
(`sourceType`/`sourceCategory`/`sourceAction`) is derived separately in code
(`internal/sourcemapping/dahua`, gw-api ≥ 3.25.0) — NOT from these mappings.

## ⚠️ Root-cause correction (2026-06-24)

The original template read the rich vehicle attributes from `TrafficCar.*`, but
in this firmware those keys do **not** exist there — they live under `Vehicle.*`.
That is why `vehicleType` / `vehicleBrand` were always null and the payload looked
sparse. Corrected below:

| field | OLD path (null) | NEW path (verified value) |
|---|---|---|
| `vehicleType` | `TrafficCar.VehicleType` | `Vehicle.Category` → `"Microbus"` |
| `vehicleBrand` | `TrafficCar.VehicleBrand` | `Vehicle.Text` → `"Mitsubishi"` |

## Field mappings (`mappings[]`) — 25 fields

| targetPath (→ payload) | sourcePath | transform | verified sample |
|---|---|---|---|
| `occurredAt` | `Events.0.Data.RealUTC` | `timestamp` | unix sec → time (guarded vs device clock skew) |
| `eventCode` | `Events.0.Code` | — | `TrafficJunction` |
| `eventName` | `Events.0.Data.Name` | — | `VehicleDetect` |
| `objectClass` | `Events.0.Data.Class` | — | `ObjectDetect` |
| `objectType` | `Events.0.Data.Object.ObjectType` | — | `Plate` |
| `plate` | `Events.0.Data.TrafficCar.PlateNumber` | — | null when ANPR reads no plate |
| `plateColor` | `Events.0.Data.TrafficCar.PlateColor` | — | null this frame |
| `plateType` | `Events.0.Data.TrafficCar.PlateType` | — | `Unknown` |
| `vehicleType` | `Events.0.Data.Vehicle.Category` | — | `Microbus` |
| `vehicleBrand` | `Events.0.Data.Vehicle.Text` | — | `Mitsubishi` (car-logo text) |
| `vehicleColor` | `Events.0.Data.TrafficCar.VehicleColor` | — | `Black` |
| `vehicleSize` | `Events.0.Data.TrafficCar.VehicleSize` | — | `Light-duty` |
| `vehicleAction` | `Events.0.Data.Vehicle.Action` | — | `Appear` |
| `direction` | `Events.0.Data.TrafficCar.Direction` | — | `6` (numeric code) |
| `drivingDirection` | `Events.0.Data.TrafficCar.DrivingDirection.0` | — | `Approach` (human label) |
| `speed` | `Events.0.Data.TrafficCar.Speed` | — | `0` |
| `speedLimit` | `Events.0.Data.TrafficCar.UpperSpeedLimit` | — | `60` |
| `lane` | `Events.0.Data.TrafficCar.PhysicalLane` | — | `0` |
| `driverCalling` | `Events.0.Data.Vehicle.MainSeat.DriverCalling` | — | `CallingUnknown` |
| `driverSmoking` | `Events.0.Data.Vehicle.MainSeat.DriverSmoking` | — | `SmokingUnknown` |
| `safeBelt` | `Events.0.Data.Vehicle.MainSeat.SafeBelt` | — | `unknow` (sic) |
| `detectConfidence` | `Events.0.Data.CategoryConfidence` | — | `63` |
| `machineName` | `Events.0.Data.TrafficCar.MachineName` | — | `Dahua001` (configurable, NOT a unique id) |
| `violationCode` | `Events.0.Data.TrafficCar.ViolationCode` | — | blank when no violation |
| `violationDesc` | `Events.0.Data.TrafficCar.ViolationDesc` | — | blank when no violation |

All `required: false` — most fields are routinely null/blank per frame; never drop the event.

## Images / `binaryRefs` — IMPLEMENTED in code (gw-api ≥ 3.26.0)

The JPEGs are embedded in the multipart binary part(s), indexed by byte offset.
`internal/services/ingestsvc/dahuaImages.go` reassembles the binary blob and picks
**two** images — the full scene (`pictureList_0`) + the detected body crop
(`pictureList_1`). Dahua firmwares describe the images in **three different
shapes**, all handled (verified against real `HumanTrait` / `TrafficJunction`
payloads, 2026-06-24):

| shape | scene | crop |
|---|---|---|
| **direct keys** (HumanTrait/FaceTrait) | `Data.SceneImage` | `Data.HumanImage` / `FaceImage` / `VehicleBodyImage` |
| **Type-tagged `Data.Image[]`** | entry `Type:"SceneImage"` | entry `Type:"VehicleBody"` / `HumanImage` / … |
| **nested legacy** (older ANPR) | `Data.SceneImage` | `Data.Vehicle.Image` / `Object.Image` |

Verified `HumanTrait` layout (contiguous in the blob): `SceneImage [0:194706]`,
`HumanImage [194706:289951]`, `FaceSceneImage [289951:484774]`, `FaceImage
[484774:538674]` — we take `SceneImage` + `HumanImage` (`HumanImage` preferred
over `FaceImage` for a person event).

Each JPEG is SOI-validated, then attached base64 under `pictureBase64List` — the
**same field AIBOX uses**. The normalizer's existing `extractBinaries`
(normalizedcons) uploads each to S3 (`{ws}/events/{eventId}/pictureList_N.jpg`,
bucket = `S3_BUCKET`) and emits `binaryRefs[]` (role `capture`) — identical shape
to AIBOX, zero new S3 code.

- Cumulative cap `maxDahuaPicBytes` (700 KB raw) keeps the raw.events message
  under the Kafka limit (scene added first, kept; crop drops if a frame is huge).
- Reuses the S3 bucket already configured for AIBOX — no extra deploy config.

## `pictureCoordinates` — IMPLEMENTED in code (gw-api ≥ 3.28.0)

AIBOX-style detection boxes are derived in the normalizer (`sourcemapping/dahua.
PictureCoordinates`) from the raw `Vehicle.BoundingBox` / `Object.BoundingBox`.
Dahua boxes are on a 0..8191 grid → normalized to `[0,1]`; `width`/`height` come
from `SceneImage`. Emitted on the normalized payload as:

```json
"pictureCoordinates": [
  { "width": 1920, "height": 1080, "x1": 0.028, "y1": 0.080, "x2": 0.238, "y2": 0.434 }
]
```

Vehicle box first, then object/plate box. Presence-gated; never overrides a
template-produced value.

## Notes / limits
- **Single-event:** maps `Events[0]` only. Dahua may batch `Events: [...]`; multi-event fan-out is a separate decision (plan §B.3).
- **Camera identity:** payload has no hardware unique id — attribution comes from the camID path (`/events/{org}/dahua/{camID}`); `machineName` is informational only.
- **`location`:** comes from the per-camera `device_management` record (operator-set at the camera), NOT from the payload.
- **`direction` code:** `6` is a Dahua direction enum; `drivingDirection` (`Approach`) is the human-readable companion. A code→label table can be added in `internal/sourcemapping/dahua` later if needed.

## Apply to the production workspace template
Workspace `02f05fa8-…` template `Dahua default (catch-all)`
(`mapping_templates._id = ObjectId("6a3b689f8a8562b5cbd32dd9")`,
`templateId = 77cc6e2f-43d3-4a6c-96fc-cfba5ea5819f`). The ready-to-run mongosh
`updateOne` + cache-clear is in `docs/reference/dahua-template-apply.js`.
Template mappings are re-read per ingest; clear `source:profile:dahua` after the
write to drop any cached profile.

## Bootstrap — auto-template on fresh installs (gw-api ≥ 3.33.0)

A fresh workspace no longer needs the template created by hand. The embedded
suggestion `config/ingest/mappingsuggestions/dahua/dahua.trafficJunction.v1.json`
(matchRule: `Events exists`, 24 field mappings) is auto-applied: the **first**
Dahua event for a workspace with no template triggers `tryAutoApplySuggestion`,
which persists a per-workspace template `auto:dahua.trafficJunction.v1`. Taxonomy /
binaryRefs / pictureCoordinates come from code, so the flow is complete without a
manual `mapping_templates` insert. The manual `dahua-template-apply.js` remains for
overriding/extending the mappings.
