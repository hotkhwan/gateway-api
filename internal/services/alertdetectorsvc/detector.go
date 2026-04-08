// internal/services/alertdetectorsvc/detector.go
package alertdetectorsvc

// AlertDetector inspects a raw JSON payload and extracts alert fields.
// Detection is workspace-scoped: each workspace may define its own alert keys.
// For Phase 1, a simple key-presence check is used (configurable via env ALERT_KEYS).
type AlertDetector struct {
	alertKeys []string // field names that trigger a fast alert
}

// DefaultAlertKeys are the field names that indicate an alert event.
// Configurable per-workspace in future phases.
var DefaultAlertKeys = []string{
	"alert", "alarm", "event_type", "eventType",
	"motion", "intrusion", "fire", "smoke", "tamper",
	"faceDetected", "plateDetected", "personDetected",
}

// New creates a detector with the given alert keys.
func New(alertKeys []string) *AlertDetector {
	if len(alertKeys) == 0 {
		alertKeys = DefaultAlertKeys
	}
	return &AlertDetector{alertKeys: alertKeys}
}

// HasAlert returns true if the payload map contains any alert key with a non-nil, non-false value.
func (d *AlertDetector) HasAlert(payload map[string]any) bool {
	for _, key := range d.alertKeys {
		if v, ok := payload[key]; ok && isTruthy(v) {
			return true
		}
	}
	return false
}

// Extract returns only the alert-relevant fields from the payload.
func (d *AlertDetector) Extract(payload map[string]any) map[string]any {
	fields := make(map[string]any)
	for _, key := range d.alertKeys {
		if v, ok := payload[key]; ok {
			fields[key] = v
		}
	}
	return fields
}

func isTruthy(v any) bool {
	if v == nil {
		return false
	}
	switch val := v.(type) {
	case bool:
		return val
	case string:
		return val != "" && val != "false" && val != "0"
	case float64:
		return val != 0
	case int:
		return val != 0
	}
	return true
}
