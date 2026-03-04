// internal/gateways/linegw/client.go
package linegw

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

// LINE Message API request/response structures
type LineMessageRequest struct {
	To       string        `json:"to"`
	Messages []LineMessage `json:"messages"`
}

type LineMessage struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type LineErrorResponse struct {
	Message string `json:"message"`
	Details []struct {
		Message string `json:"message"`
		Property string `json:"property"`
	} `json:"details"`
}

// Client sends events to LINE targets
type Client struct {
	config authzmod.TargetConfig
	client *http.Client
	logger zerolog.Logger
}

// NewClient creates a new LINE client
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
		logger: logger.WithMeta("linegw", "Client"),
	}
}

// Send sends an event to LINE Messaging API
func (c *Client) Send(ctx context.Context, event interface{}, payload []byte) error {
	if c.config.ChannelAccessToken == "" {
		return fmt.Errorf("LINE channel access token is empty")
	}

	// Parse event to get message content
	var eventData map[string]interface{}
	if err := json.Unmarshal(payload, &eventData); err != nil {
		return fmt.Errorf("failed to parse event payload: %w", err)
	}

	// Build LINE message
	messageText := c.buildMessageText(eventData)

	lineReq := LineMessageRequest{
		To: c.config.To,
		Messages: []LineMessage{
			{
				Type: "text",
				Text: messageText,
			},
		},
	}

	// Marshal request
	reqBody, err := json.Marshal(lineReq)
	if err != nil {
		return fmt.Errorf("failed to marshal LINE request: %w", err)
	}

	// Create HTTP request
	req, err := http.NewRequestWithContext(
		ctx,
		"POST",
		"https://api.line.me/v2/bot/message/push",
		bytes.NewReader(reqBody),
	)
	if err != nil {
		return fmt.Errorf("failed to create LINE request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.config.ChannelAccessToken)

	// Send request
	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send LINE message: %w", err)
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode != http.StatusOK {
		var errResp LineErrorResponse
		if err := json.NewDecoder(resp.Body).Decode(&errResp); err == nil {
			return fmt.Errorf("LINE API error: %s - %s", errResp.Message, errResp.Details)
		}
		return fmt.Errorf("LINE API returned non-success status: %d", resp.StatusCode)
	}

	c.logger.Debug().
		Str("to", c.config.To).
		Int("statusCode", resp.StatusCode).
		Msg("LINE message sent successfully")

	return nil
}

// buildMessageText formats the event for LINE messaging
func (c *Client) buildMessageText(eventData map[string]interface{}) string {
	var eventType, eventId string

	if v, ok := eventData["eventType"].(string); ok {
		eventType = v
	}
	if v, ok := eventData["eventId"].(string); ok {
		eventId = v
	}

	return fmt.Sprintf("🔔 New Event\nType: %s\nID: %s", eventType, eventId)
}
