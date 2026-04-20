// internal/kafka/deliverycons/render.go
package deliverycons

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"

	"github.com/hotkhwan/gateway-api/models/ingestmod"
)

// renderContext builds the template data map used by Go text/template.
// Access pattern: {{.eventId}}, {{.eventType}}, {{.payload.listType_label}}, {{.source.orgId}}, etc.
//
// Note on sourceFamily: klynx-api's republish path stores it in Source.DeviceType,
// so we expose it as {{.sourceFamily}} for template convenience regardless of path.
func renderContext(event *ingestmod.NormalizedEvent) map[string]any {
	return map[string]any{
		"eventId":       event.EventId,
		"tenantId":      event.TenantId,
		"eventType":     event.EventType,
		"eventCategory": event.EventCategory,
		"eventAction":   event.EventAction,
		"eventClass":    event.EventClass,
		"eventSeverity": event.EventSeverity,
		"sourceFamily":  event.Source.DeviceType, // klynx republish puts SourceFamily here
		"occurredAt":    event.OccurredAt.UTC().Format("2006-01-02T15:04:05Z"),
		"payload":       event.Payload,
		"source": map[string]any{
			"workspaceId": event.Source.WorkspaceId,
			"orgId":       event.Source.WorkspaceId, // legacy alias; prefer .workspaceId
			"deviceId":    event.Source.DeviceId,
			"deviceType":  event.Source.DeviceType,
			"subType":     event.Source.SubType,
			"vendor":      event.Source.Vendor,
		},
		"location": map[string]any{
			"lat":  event.Location.Lat,
			"lng":  event.Location.Lng,
			"site": event.Location.Site,
			"zone": event.Location.Zone,
		},
		"geo": map[string]any{
			"countryCode": event.Geo.CountryCode,
			"adminName":   event.Geo.AdminName,
			"adminCode":   event.Geo.AdminCode,
		},
	}
}

// renderText executes a Go text/template string with the given data map.
func renderText(tmplStr string, data map[string]any) (string, error) {
	t, err := template.New("").Funcs(template.FuncMap{
		"default": func(def, val any) any {
			if val == nil || val == "" {
				return def
			}
			return val
		},
	}).Parse(tmplStr)
	if err != nil {
		return "", fmt.Errorf("template parse error: %w", err)
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("template execute error: %w", err)
	}
	return buf.String(), nil
}

// selectTemplateByKey returns the first MessageTemplate matching the given key.
// Returns nil if no template has that key.
func selectTemplateByKey(templates []ingestmod.MessageTemplate, key string) *ingestmod.MessageTemplate {
	for i := range templates {
		if templates[i].Key == key {
			return &templates[i]
		}
	}
	return nil
}

// selectTemplate returns the best matching MessageTemplate for channelType and locale.
// Fallback chain: exact (channelType+locale) → (channelType+defaultLocale) → (channelType+en) → nil
func selectTemplate(templates []ingestmod.MessageTemplate, channelType, locale, defaultLocale string) *ingestmod.MessageTemplate {
	fallbacks := dedupe([]string{locale, defaultLocale, "en"})
	for _, loc := range fallbacks {
		for i := range templates {
			if strings.EqualFold(templates[i].ChannelType, channelType) &&
				strings.EqualFold(templates[i].Locale, loc) {
				return &templates[i]
			}
		}
	}
	// Last resort: any template with matching channelType
	for i := range templates {
		if strings.EqualFold(templates[i].ChannelType, channelType) {
			return &templates[i]
		}
	}
	return nil
}

// renderMessage renders title and body for a given template and event.
func renderMessage(mt *ingestmod.MessageTemplate, event *ingestmod.NormalizedEvent) (title, body string, err error) {
	data := renderContext(event)
	title, err = renderText(mt.Title, data)
	if err != nil {
		return "", "", err
	}
	body, err = renderText(mt.Body, data)
	if err != nil {
		return "", "", err
	}
	return title, body, nil
}

func dedupe(ss []string) []string {
	seen := make(map[string]struct{}, len(ss))
	out := make([]string, 0, len(ss))
	for _, s := range ss {
		if s == "" {
			continue
		}
		if _, ok := seen[s]; !ok {
			seen[s] = struct{}{}
			out = append(out, s)
		}
	}
	return out
}
