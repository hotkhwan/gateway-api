// internal/mqtt/alertmsg/publish.go
package alertmsg

import (
	"fmt"

	"github.com/hotkhwan/gateway-api/internal/adapters/alertdispatcher"
	"github.com/hotkhwan/gateway-api/internal/mqtt/inframsg"
)

// TopicAlert returns the MQTT topic for fast alerts.
// Pattern: phibek/ws/{workspaceId}/alert/{sourceFamily}
func TopicAlert(workspaceID, sourceFamily string) string {
	return fmt.Sprintf("phibek/ws/%s/alert/%s", workspaceID, sourceFamily)
}

// TopicCanonical returns the MQTT topic for canonical event notifications.
// Pattern: phibek/ws/{workspaceId}/events/{sourceFamily}
func TopicCanonical(workspaceID, sourceFamily string) string {
	return fmt.Sprintf("phibek/ws/%s/events/%s", workspaceID, sourceFamily)
}

// PublishAlert sends a provisional fast alert via MQTT (Path A).
// QoS 0 — fire-and-forget, non-blocking. Failure is logged but not returned.
func PublishAlert(alert alertdispatcher.FastAlertEnvelope) error {
	topic := TopicAlert(alert.WorkspaceID, alert.SourceFamily)
	return inframsg.PublishJSON(topic, 0, false, alert)
}

// PublishCanonicalNotify sends a lightweight canonical event notification via MQTT (Path B).
// Used after normalizedcons writes the canonical event to MongoDB.
func PublishCanonicalNotify(workspaceID, sourceFamily, eventID string) error {
	topic := TopicCanonical(workspaceID, sourceFamily)
	payload := map[string]any{
		"eventId":      eventID,
		"workspaceId":  workspaceID,
		"sourceFamily": sourceFamily,
		"canonical":    true,
		"provisional":  false,
	}
	return inframsg.PublishJSON(topic, 0, false, payload)
}
