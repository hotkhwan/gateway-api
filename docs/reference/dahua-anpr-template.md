# Reference — Dahua ANPR (TrafficJunction) MappingTemplate

Concrete field mappings for the `dahua` source family, derived from a live
`raw.events` `TrafficJunction` payload (2026-06-24). Requires the array-index
mapper (gateway-api ≥ 3.21.0) — source paths reach into the `Events[]` array
(`Events.0.Data.TrafficCar.*`). See `docs/plan/dahua-camera-flow.md`.

The mapped output becomes the normalized event `payload` (the normalizer
re-applies the same `mappings` to the canonical rawBody — `normalize.go`).

## Field mappings (`mappings[]`)

| targetPath (→ normalized payload) | sourcePath | transform | notes |
|---|---|---|---|
| `occurredAt` | `Events.0.Data.RealUTC` | `timestamp` | unix sec → event time (else falls back to receivedAt) |
| `eventCode` | `Events.0.Code` | — | e.g. `TrafficJunction` |
| `plate` | `Events.0.Data.TrafficCar.PlateNumber` | — | null when ANPR doesn't read a plate |
| `plateColor` | `Events.0.Data.TrafficCar.PlateColor` | — | |
| `plateType` | `Events.0.Data.TrafficCar.PlateType` | — | e.g. `Unknown` |
| `vehicleType` | `Events.0.Data.TrafficCar.VehicleType` | — | sedan/… (null on low-confidence) |
| `vehicleColor` | `Events.0.Data.TrafficCar.VehicleColor` | — | e.g. `Black` |
| `vehicleBrand` | `Events.0.Data.TrafficCar.VehicleBrand` | — | |
| `direction` | `Events.0.Data.TrafficCar.Direction` | — | |
| `machineName` | `Events.0.Data.TrafficCar.MachineName` | — | configurable, NOT a unique camera id |

All `required: false` — plate/vehicleType are routinely null per frame; never drop the event.

## Notes / limits
- **Single-event:** maps `Events[0]` only. Dahua may batch `Events: [...]`; multi-event fan-out is a separate decision (see plan §B.3).
- **Camera identity:** the payload has no hardware unique id — attribution comes from the camID path (`/events/{org}/dahua/{camID}`), not these fields. `machineName` is informational only.
- **Snapshots:** the parser strips JPEG bytes (`_binaries` descriptors); S3 upload + `binaryRefs` is a later phase.
- **Classification:** `classificationRules` (eventClass/severity) intentionally omitted here — add once the severity vocab is agreed with the hub.

## Apply to a workspace template
Set these on the workspace's `dahua` MappingTemplate (`mapping_templates`), e.g. update the existing
`Dahua default (catch-all)` doc's `mappings` array. Takes effect once gateway-api ≥ 3.21.0 is deployed.
