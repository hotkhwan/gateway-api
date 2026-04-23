// internal/gateways/discordgw/client.go
package discordgw

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/hotkhwan/gateway-api/internal/logger"
	"github.com/hotkhwan/gateway-api/models/authzmod"
	"github.com/rs/zerolog"
)

// Discord Webhook request/response structures
type DiscordWebhookRequest struct {
	Content  string         `json:"content"`
	Username string         `json:"username,omitempty"`
	Embeds   []DiscordEmbed `json:"embeds,omitempty"`
}

type DiscordEmbed struct {
	Title       string              `json:"title"`
	Description string              `json:"description"`
	URL         string              `json:"url,omitempty"`
	Color       int                 `json:"color"`
	Fields      []DiscordEmbedField `json:"fields,omitempty"`
	Image       *DiscordEmbedImage  `json:"image,omitempty"`
	Timestamp   string              `json:"timestamp,omitempty"`
}

type DiscordEmbedField struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Inline bool   `json:"inline,omitempty"`
}

type DiscordEmbedImage struct {
	URL string `json:"url"`
}

type DiscordErrorResponse struct {
	Message string `json:"message"`
	Code    int    `json:"code"`
}

// Client sends events to Discord webhook targets
type Client struct {
	config authzmod.TargetConfig
	client *http.Client
	logger zerolog.Logger
}

// NewClient creates a new Discord client
func NewClient(config authzmod.TargetConfig) *Client {
	timeout := 30 * time.Second
	if config.TimeoutMs > 0 {
		timeout = time.Duration(config.TimeoutMs) * time.Millisecond
	}

	return &Client{
		config: config,
		client: &http.Client{
			Timeout: timeout,
		},
		logger: logger.WithMeta("discordgw", "Client"),
	}
}

// Send sends an event to Discord webhook
func (c *Client) Send(ctx context.Context, event interface{}, payload []byte) error {
	if c.config.URL == "" {
		return fmt.Errorf("Discord webhook URL is empty")
	}

	// Parse event to get message content
	var eventData map[string]interface{}
	if err := json.Unmarshal(payload, &eventData); err != nil {
		return fmt.Errorf("failed to parse event payload: %w", err)
	}

	// Build Discord message
	discordReq := c.buildWebhookRequest(eventData)

	// Marshal request
	reqBody, err := json.Marshal(discordReq)
	if err != nil {
		return fmt.Errorf("failed to marshal Discord request: %w", err)
	}

	// Create HTTP request
	req, err := http.NewRequestWithContext(
		ctx,
		"POST",
		c.config.URL,
		bytes.NewReader(reqBody),
	)
	if err != nil {
		return fmt.Errorf("failed to create Discord request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")

	// Add custom headers from config
	for key, value := range c.config.Headers {
		req.Header.Set(key, value)
	}

	// Add signing header if enabled
	if c.config.SigningEnabled && c.config.SigningSecret != "" {
		// TODO: Implement HMAC signature verification
		c.logger.Warn().Msg("Discord webhook signing enabled but not implemented yet")
	}

	// Send request
	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send Discord webhook: %w", err)
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var errResp DiscordErrorResponse
		if err := json.NewDecoder(resp.Body).Decode(&errResp); err == nil {
			return fmt.Errorf("Discord API error: %s (code: %d)", errResp.Message, errResp.Code)
		}
		return fmt.Errorf("Discord webhook returned non-success status: %d", resp.StatusCode)
	}

	c.logger.Debug().
		Str("url", c.config.URL).
		Int("statusCode", resp.StatusCode).
		Msg("Discord webhook sent successfully")

	return nil
}

// buildWebhookRequest formats the event for Discord webhook.
// Prefers the rendered `title` / `body` from the dispatch envelope
// (MappingTemplate.MessageTemplates) and falls back to a minimal
// eventType/eventId summary when no template is configured.
//
// Enrichments wired off the envelope:
//   - imageUrl       → embed.image (renders below the description)
//   - eventDetailsUrl → embed.url   (makes the title clickable)
//   - lat/lng        → "Location" field with a Google Maps link
func (c *Client) buildWebhookRequest(eventData map[string]interface{}) DiscordWebhookRequest {
	title, _ := eventData["title"].(string)
	body, _ := eventData["body"].(string)
	imageURL := strings.TrimSpace(stringField(eventData, "imageUrl"))
	detailsURL := strings.TrimSpace(stringField(eventData, "eventDetailsUrl"))

	// Green — neutral default until severity→color mapping lands (step 2).
	color := 65280

	embed := DiscordEmbed{Color: color}
	if title == "" && body == "" {
		embed.Title = "New Event"
		embed.Description = "A new event has been processed"
		eventType, _ := eventData["eventType"].(string)
		eventId, _ := eventData["eventId"].(string)
		if eventType != "" {
			embed.Fields = append(embed.Fields, DiscordEmbedField{Name: "Event Type", Value: eventType, Inline: true})
		}
		if eventId != "" {
			embed.Fields = append(embed.Fields, DiscordEmbedField{Name: "Event ID", Value: eventId, Inline: true})
		}
	} else {
		if title == "" {
			title = "Event notification"
		}
		embed.Title = title
		embed.Description = body
	}

	if imageURL != "" {
		embed.Image = &DiscordEmbedImage{URL: imageURL}
	}
	if detailsURL != "" {
		embed.URL = detailsURL
	}

	if lat, hasLat := floatField(eventData, "lat"); hasLat {
		if lng, hasLng := floatField(eventData, "lng"); hasLng && (lat != 0 || lng != 0) {
			mapsURL := fmt.Sprintf("https://www.google.com/maps/search/?api=1&query=%f,%f", lat, lng)
			embed.Fields = append(embed.Fields, DiscordEmbedField{
				Name:   "📍 Location",
				Value:  fmt.Sprintf("[%.6f, %.6f](%s)", lat, lng, mapsURL),
				Inline: false,
			})
		}
	}

	if detailsURL != "" {
		embed.Fields = append(embed.Fields, DiscordEmbedField{
			Name:   "🔎 Details",
			Value:  fmt.Sprintf("[View Event Details](%s)", detailsURL),
			Inline: false,
		})
	}

	return DiscordWebhookRequest{Embeds: []DiscordEmbed{embed}}
}

func stringField(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

// floatField pulls a numeric value out of the envelope, accepting either a
// float64 (the JSON default) or a json.Number (when the decoder is configured
// for it). Returns false on missing/wrong-type so the caller can skip cleanly.
func floatField(m map[string]interface{}, key string) (float64, bool) {
	switch v := m[key].(type) {
	case float64:
		return v, true
	case json.Number:
		if f, err := v.Float64(); err == nil {
			return f, true
		}
	}
	return 0, false
}
