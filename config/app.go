// config/app.go
package config

import (
	"os"
	"strings"
)

type AppConfig struct {
	AppName    string
	AppVersion string
}

// WebBaseURL is the user-facing UI base URL used to build deep links sent in
// delivery messages (Telegram, LINE, Discord). Set via env var WEB_BASE_URL,
// e.g. "https://gateway.aisom.cloud". Empty means no default deep link is
// emitted; per-template extras["eventDetailsUrl"] still works.
var WebBaseURL string

func init() {
	WebBaseURL = strings.TrimRight(strings.TrimSpace(os.Getenv("WEB_BASE_URL")), "/")
}

func LoadAppConfig() AppConfig {
	version := os.Getenv("APP_VERSION")
	if version == "" {
		version = "dev"
	}

	return AppConfig{
		AppName:    "gateway",
		AppVersion: version,
	}
}

// EventDetailsURL builds the default deep link for an event when WebBaseURL
// is configured. Returns "" when no base URL is set.
func EventDetailsURL(eventId string) string {
	if WebBaseURL == "" || eventId == "" {
		return ""
	}
	return WebBaseURL + "/events/" + eventId
}
