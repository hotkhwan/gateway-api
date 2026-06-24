// docs/reference/dahua-template-apply.js
// Enrich the production Dahua "Dahua default (catch-all)" MappingTemplate.
// Verified against a full TrafficJunction/VehicleDetect payload (2026-06-24).
// See dahua-anpr-template.md for the field table + rationale.
//
// RUN (on production node 192.168.1.57):
//   URI=$(kubectl -n iot get cm gw-api-config -o jsonpath='{.data.MONGO_URI}')
//   kubectl -n app exec -i gw-mongodb-0 -c mongod -- mongosh "$URI" --quiet < dahua-template-apply.js
// THEN clear the cached source profile so the change takes effect immediately:
//   RP=$(kubectl -n iot get secret gw-api-secret -o jsonpath='{.data.REDIS_PASSWORD}' | base64 -d)
//   kubectl -n cache exec redis-ha-server-0 -c redis -- redis-cli -a "$RP" DEL source:profile:dahua

const d = db.getSiblingDB("gateway");
const ID = ObjectId("6a3b689f8a8562b5cbd32dd9"); // templateId 77cc6e2f-43d3-4a6c-96fc-cfba5ea5819f

const before = d.mapping_templates.findOne({ _id: ID });
print("BACKUP old mappings (" + (before.mappings || []).length + "):");
print(JSON.stringify((before.mappings || []).map(m => [m.targetPath, m.sourcePath])));

const now = new Date().toISOString();
const mk = (t, s, tr = "") => ({ targetPath: t, sourcePath: s, transform: tr, required: false, confidence: 1, updatedAt: now });

const mappings = [
  mk("occurredAt", "Events.0.Data.RealUTC", "timestamp"),
  mk("eventCode", "Events.0.Code"),
  mk("eventName", "Events.0.Data.Name"),
  mk("objectClass", "Events.0.Data.Class"),
  mk("objectType", "Events.0.Data.Object.ObjectType"),
  mk("plate", "Events.0.Data.TrafficCar.PlateNumber"),
  mk("plateColor", "Events.0.Data.TrafficCar.PlateColor"),
  mk("plateType", "Events.0.Data.TrafficCar.PlateType"),
  mk("vehicleType", "Events.0.Data.Vehicle.Category"),   // FIX: was TrafficCar.VehicleType (null)
  mk("vehicleBrand", "Events.0.Data.Vehicle.Text"),      // FIX: was TrafficCar.VehicleBrand (null)
  mk("vehicleColor", "Events.0.Data.TrafficCar.VehicleColor"),
  mk("vehicleSize", "Events.0.Data.TrafficCar.VehicleSize"),
  mk("vehicleAction", "Events.0.Data.Vehicle.Action"),
  mk("direction", "Events.0.Data.TrafficCar.Direction"),
  mk("drivingDirection", "Events.0.Data.TrafficCar.DrivingDirection.0"),
  mk("speed", "Events.0.Data.TrafficCar.Speed"),
  mk("speedLimit", "Events.0.Data.TrafficCar.UpperSpeedLimit"),
  mk("lane", "Events.0.Data.TrafficCar.PhysicalLane"),
  mk("driverCalling", "Events.0.Data.Vehicle.MainSeat.DriverCalling"),
  mk("driverSmoking", "Events.0.Data.Vehicle.MainSeat.DriverSmoking"),
  mk("safeBelt", "Events.0.Data.Vehicle.MainSeat.SafeBelt"),
  mk("detectConfidence", "Events.0.Data.CategoryConfidence"),
  mk("machineName", "Events.0.Data.TrafficCar.MachineName"),
  mk("violationCode", "Events.0.Data.TrafficCar.ViolationCode"),
  mk("violationDesc", "Events.0.Data.TrafficCar.ViolationDesc"),
];

const r = d.mapping_templates.updateOne({ _id: ID }, { $set: { mappings: mappings, updatedAt: now } });
print("UPDATE matched=" + r.matchedCount + " modified=" + r.modifiedCount + " newCount=" + mappings.length);
