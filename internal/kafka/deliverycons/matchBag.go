// internal/kafka/deliverycons/matchBag.go
package deliverycons

import (
	"sync/atomic"

	"github.com/hotkhwan/gateway-api/models/ingestmod"
)

// deliverySkipCounters holds in-process counters for the two template-level
// skip reasons enforced in dispatchToTargets (plan decision D9).
// Exported via package-level getters for tests and /metrics-style probes.
var (
	deliverySkipDisabled         atomic.Uint64
	deliverySkipDeliveryRuleMiss atomic.Uint64
)

// DeliverySkipCounts returns a snapshot of the skip counters since process start.
func DeliverySkipCounts() (disabled, deliveryRuleMiss uint64) {
	return deliverySkipDisabled.Load(), deliverySkipDeliveryRuleMiss.Load()
}

// resetDeliverySkipCounters is a test-only helper.
func resetDeliverySkipCounters() {
	deliverySkipDisabled.Store(0)
	deliverySkipDeliveryRuleMiss.Store(0)
}

// buildDeliveryMatchBag flattens a NormalizedEvent into a generic map suitable
// for evaluation by templatematcher.Evaluate.
//
// Namespace rules (plan decision D2, contract §5.5):
//   - Top-level canonical fields and their source* aliases.
//   - source.*, location.*, geo.*, geoCell.* as nested maps.
//   - payload.* flattened as a nested map.
//   - raw.* is intentionally absent — raw payload does not exist at delivery
//     time; admin validation rejects raw.* on deliveryMatchAll / deliveryMatchAny.
func buildDeliveryMatchBag(event *ingestmod.NormalizedEvent) map[string]any {
	if event == nil {
		return map[string]any{}
	}

	bag := map[string]any{
		"eventId":       event.EventId,
		"tenantId":      event.TenantId,
		"eventType":     event.EventType,
		"eventCategory": event.EventCategory,
		"eventAction":   event.EventAction,
		"eventClass":    event.EventClass,
		"eventSeverity": event.EventSeverity,
		"occurredAt":    event.OccurredAt,
		// FE-friendly aliases (UI exposes the source* names; the ingestmod
		// struct uses the event* names — map one-to-one so authors can use
		// either form per the contract).
		"sourceType":     event.EventType,
		"sourceCategory": event.EventCategory,
		"sourceAction":   event.EventAction,
		"templateId":     event.Meta.TemplateId,
		"workspaceId":    event.Source.WorkspaceId,
	}

	bag["source"] = map[string]any{
		"deviceId":          event.Source.DeviceId,
		"deviceType":        event.Source.DeviceType,
		"deviceName":        event.Source.DeviceName,
		"deviceDescription": event.Source.DeviceDescription,
		"subType":           event.Source.SubType,
		"vendor":            event.Source.Vendor,
		"protocol":          event.Source.Protocol,
		"workspaceId":       event.Source.WorkspaceId,
	}

	bag["location"] = map[string]any{
		"lat":  event.Location.Lat,
		"lng":  event.Location.Lng,
		"site": event.Location.Site,
		"zone": event.Location.Zone,
	}

	bag["geo"] = map[string]any{
		"countryCode": event.Geo.CountryCode,
		"adminLevel":  event.Geo.AdminLevel,
		"adminName":   event.Geo.AdminName,
		"adminCode":   event.Geo.AdminCode,
		"idScheme":    event.Geo.IdScheme,
	}

	bag["geoCell"] = map[string]any{
		"scheme":    event.GeoCell.Scheme,
		"precision": event.GeoCell.Precision,
		"cell":      event.GeoCell.Cell,
	}

	if event.Payload != nil {
		bag["payload"] = event.Payload
	} else {
		bag["payload"] = map[string]any{}
	}

	return bag
}
