// internal/gateways/telegw/client.go
package telegw

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

// Telegram Bot API request/response structures
type TelegramMessageRequest struct {
	ChatID    string `json:"chat_id"`
	Text      string `json:"text"`
	ParseMode string `json:"parse_mode,omitempty"`
}

type TelegramErrorResponse struct {
	Ok          bool   `json:"ok"`
	ErrorCode   int    `json:"error_code"`
	Description string `json:"description"`
}

// Client sends events to Telegram targets
type Client struct {
	config authzmod.TargetConfig
	client *http.Client
	logger zerolog.Logger
}

// NewClient creates a new Telegram client
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
		logger: logger.WithMeta("telegw", "Client"),
	}
}

// Send sends an event to Telegram Bot API
func (c *Client) Send(ctx context.Context, event interface{}, payload []byte) error {
	if c.config.BotToken == "" {
		return fmt.Errorf("Telegram bot token is empty")
	}
	if c.config.ChatId == "" {
		return fmt.Errorf("Telegram chat ID is empty")
	}

	// Parse event to get message content
	var eventData map[string]interface{}
	if err := json.Unmarshal(payload, &eventData); err != nil {
		return fmt.Errorf("failed to parse event payload: %w", err)
	}

	// Build Telegram message
	messageText := c.buildMessageText(eventData)

	telegramReq := TelegramMessageRequest{
		ChatID:    c.config.ChatId,
		Text:      messageText,
		ParseMode: "HTML",
	}

	// Marshal request
	reqBody, err := json.Marshal(telegramReq)
	if err != nil {
		return fmt.Errorf("failed to marshal Telegram request: %w", err)
	}

	// Create HTTP request
	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", c.config.BotToken)
	req, err := http.NewRequestWithContext(
		ctx,
		"POST",
		apiURL,
		bytes.NewReader(reqBody),
	)
	if err != nil {
		return fmt.Errorf("failed to create Telegram request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")

	// Send request
	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send Telegram message: %w", err)
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode != http.StatusOK {
		var errResp TelegramErrorResponse
		if err := json.NewDecoder(resp.Body).Decode(&errResp); err == nil {
			return fmt.Errorf("Telegram API error: %s (code: %d)", errResp.Description, errResp.ErrorCode)
		}
		return fmt.Errorf("Telegram API returned non-success status: %d", resp.StatusCode)
	}

	c.logger.Debug().
		Str("chatId", c.config.ChatId).
		Int("statusCode", resp.StatusCode).
		Msg("Telegram message sent successfully")

	return nil
}

// buildMessageText formats the event for Telegram messaging
func (c *Client) buildMessageText(eventData map[string]interface{}) string {
	var eventType, eventId string

	if v, ok := eventData["eventType"].(string); ok {
		eventType = v
	}
	if v, ok := eventData["eventId"].(string); ok {
		eventId = v
	}

	return fmt.Sprintf("<b>🔔 New Event</b>\n\n<b>Type:</b> %s\n<b>ID:</b> %s", eventType, eventId)
}
