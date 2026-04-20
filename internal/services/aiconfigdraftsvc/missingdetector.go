// internal/services/aiconfigdraftsvc/missingdetector.go
package aiconfigdraftsvc

// detectMissing returns required fields that are not yet resolved in the draft.
// Rules are deterministic per action type — not AI-determined.
func detectMissing(intent DraftIntent) []MissingFieldHint {
	var hints []MissingFieldHint

	for _, action := range intent.DeliveryActions {
		if action.Resolved {
			continue
		}
		switch action.Type {
		case "webhook":
			if action.RawTarget == "" || action.SecretRef == "" {
				hints = append(hints, MissingFieldHint{
					Field:     "url",
					Reason:    "webhook action requires a target URL",
					ForAction: "webhook",
				})
			}
		case "line":
			hints = append(hints, MissingFieldHint{
				Field:     "channelAccessToken",
				Reason:    "LINE action requires a channel access token or notify token",
				ForAction: "line",
			})
		case "discord":
			hints = append(hints, MissingFieldHint{
				Field:     "webhookUrl",
				Reason:    "Discord action requires a webhook URL",
				ForAction: "discord",
			})
		case "mqtt":
			hints = append(hints, MissingFieldHint{
				Field:     "topic",
				Reason:    "MQTT action requires a topic",
				ForAction: "mqtt",
			})
		}
	}

	return hints
}
