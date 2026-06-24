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

## Not coverable by template (need code — `internal/sourcemapping/dahua`, deferred)

These match AIBOX richness but cannot come from a flat path mapping:

- **`pictureCoordinates`** (AIBOX-style `{x1,y1,x2,y2,width,height}`): Dahua gives
  raw pixel boxes — `Object.BoundingBox` `[866,2291,1161,2506]`,
  `Vehicle.BoundingBox` `[230,659,1954,3552]` — that must be normalized into the
  AIBOX object shape in code.
- **`binaryRefs`** (the images — *"image ยังไม่มี"*): the JPEGs are embedded in the
  344 KB multipart by byte offset, described in the payload:
  `SceneImage{Offset:0,Length:233444}`, `Vehicle.Image{Offset:233444,Length:181243}`,
  `Object.Image{Offset:414687,Length:8670}`. Producing `binaryRefs` means slicing
  those byte ranges → S3 → ref descriptors (mirrors AIBOX `pictureList_*.jpg`). This
  is the deferred snapshot phase (plan §F).

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
