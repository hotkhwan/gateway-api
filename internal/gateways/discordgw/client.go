// internal/gateways/discordgw/client.go
package discordgw

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
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
	Color       int                 `json:"color"`
	Fields      []DiscordEmbedField `json:"fields,omitempty"`
	Timestamp   string              `json:"timestamp,omitempty"`
}

type DiscordEmbedField struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Inline bool   `json:"inline,omitempty"`
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

// buildWebhookRequest formats the event for Discord webhook
func (c *Client) buildWebhookRequest(eventData map[string]interface{}) DiscordWebhookRequest {
	var eventType, eventId string

	if v, ok := eventData["eventType"].(string); ok {
		eventType = v
	}
	if v, ok := eventData["eventId"].(string); ok {
		eventId = v
	}

	// Green color for successful events
	color := 65280

	embed := DiscordEmbed{
		Title:       "🔔 New Event",
		Description: "A new event has been processed",
		Color:       color,
		Fields: []DiscordEmbedField{
			{
				Name:   "Event Type",
				Value:  eventType,
				Inline: true,
			},
			{
				Name:   "Event ID",
				Value:  eventId,
				Inline: true,
			},
		},
	}

	return DiscordWebhookRequest{
		Content: "New event detected",
		Embeds:  []DiscordEmbed{embed},
	}
}
