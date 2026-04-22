// internal/gateways/telegw/client.go
package telegw

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"strings"
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

type TelegramPhotoRequest struct {
	ChatID    string `json:"chat_id"`
	Photo     string `json:"photo"`
	Caption   string `json:"caption,omitempty"`
	ParseMode string `json:"parse_mode,omitempty"`
}

type TelegramErrorResponse struct {
	Ok          bool   `json:"ok"`
	ErrorCode   int    `json:"error_code"`
	Description string `json:"description"`
}

// Telegram limits — caption is much shorter than message text, hence the split.
const (
	telegramTextLimit    = 4096
	telegramCaptionLimit = 1024
)

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

// Send sends an event to Telegram Bot API. When the envelope carries an
// imageUrl, it uses sendPhoto with the message as caption; otherwise it
// falls back to sendMessage with plain HTML text.
func (c *Client) Send(ctx context.Context, event interface{}, payload []byte) error {
	botToken := strings.TrimSpace(c.config.BotToken)
	chatID := strings.TrimSpace(c.config.ChatId)
	if botToken == "" {
		return fmt.Errorf("Telegram bot token is empty")
	}
	if chatID == "" {
		return fmt.Errorf("Telegram chat ID is empty")
	}

	var envelope map[string]interface{}
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return fmt.Errorf("failed to parse event payload: %w", err)
	}

	imageURL := strings.TrimSpace(stringField(envelope, "imageUrl"))

	if imageURL != "" {
		caption := truncateForTelegram(c.buildMessageText(envelope), telegramCaptionLimit)
		photoReq := TelegramPhotoRequest{
			ChatID:    chatID,
			Photo:     imageURL,
			Caption:   caption,
			ParseMode: "HTML",
		}
		if err := c.post(ctx, botToken, "sendPhoto", photoReq); err != nil {
			// sendPhoto can fail when Telegram cannot fetch the URL (e.g. presigned
			// link expired, host unreachable). Fall through to a plain text message
			// so the user still gets the event.
			c.logger.Warn().
				Err(err).
				Str("chatId", chatID).
				Str("imageUrl", imageURL).
				Msg("Telegram sendPhoto failed — falling back to text message")
		} else {
			c.logger.Debug().
				Str("chatId", chatID).
				Str("imageUrl", imageURL).
				Msg("Telegram photo sent successfully")
			return nil
		}
	}

	textReq := TelegramMessageRequest{
		ChatID:    chatID,
		Text:      truncateForTelegram(c.buildMessageText(envelope), telegramTextLimit),
		ParseMode: "HTML",
	}
	if err := c.post(ctx, botToken, "sendMessage", textReq); err != nil {
		return err
	}
	c.logger.Debug().
		Str("chatId", chatID).
		Msg("Telegram message sent successfully")
	return nil
}

// post serialises the request body and POSTs it to the Telegram Bot API.
func (c *Client) post(ctx context.Context, botToken, method string, body interface{}) error {
	reqBody, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("failed to marshal Telegram request: %w", err)
	}

	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/%s", botToken, method)
	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewReader(reqBody))
	if err != nil {
		return fmt.Errorf("failed to create Telegram request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send Telegram %s: %w", method, err)
	}
	defer resp.Body.Close()

	// Telegram can return HTTP 200 with {"ok": false, ...} in some edge cases
	// (rate-limit soft fails, deprecated URL schemes). Parse the body on both
	// success and failure paths so "ok: false" never slips through as success.
	var apiResp struct {
		Ok          bool   `json:"ok"`
		ErrorCode   int    `json:"error_code"`
		Description string `json:"description"`
	}
	if decErr := json.NewDecoder(resp.Body).Decode(&apiResp); decErr != nil {
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("Telegram API returned non-success status: %d", resp.StatusCode)
		}
		return fmt.Errorf("Telegram %s: failed to decode response (status %d): %w", method, resp.StatusCode, decErr)
	}
	if !apiResp.Ok {
		return fmt.Errorf("Telegram API error: %s (code: %d, status: %d)", apiResp.Description, apiResp.ErrorCode, resp.StatusCode)
	}
	return nil
}

// buildMessageText formats the envelope into HTML for Telegram. Title/body
// from a message template take precedence; otherwise it falls back to a
// minimal eventType/eventId summary. Lat/lng are appended as a Google Maps
// link when present.
func (c *Client) buildMessageText(envelope map[string]interface{}) string {
	title := strings.TrimSpace(stringField(envelope, "title"))
	body := strings.TrimSpace(stringField(envelope, "body"))
	eventType := stringField(envelope, "eventType")
	eventId := stringField(envelope, "eventId")

	var sb strings.Builder
	if title != "" {
		sb.WriteString("<b>")
		sb.WriteString(html.EscapeString(title))
		sb.WriteString("</b>")
	} else {
		sb.WriteString("<b>🔔 New Event</b>")
	}

	if body != "" {
		sb.WriteString("\n\n")
		sb.WriteString(html.EscapeString(body))
	} else if title == "" {
		// Default summary only when the template did not provide title/body.
		if eventType != "" {
			sb.WriteString("\n\n<b>Type:</b> ")
			sb.WriteString(html.EscapeString(eventType))
		}
		if eventId != "" {
			sb.WriteString("\n<b>ID:</b> ")
			sb.WriteString(html.EscapeString(eventId))
		}
	}

	lat, hasLat := floatField(envelope, "lat")
	lng, hasLng := floatField(envelope, "lng")
	if (hasLat || hasLng) && (lat != 0 || lng != 0) {
		mapsURL := fmt.Sprintf("https://www.google.com/maps/search/?api=1&query=%f,%f", lat, lng)
		sb.WriteString("\n\n📍 <a href=\"")
		sb.WriteString(html.EscapeString(mapsURL))
		sb.WriteString("\">")
		sb.WriteString(fmt.Sprintf("%.6f, %.6f", lat, lng))
		sb.WriteString("</a>")
	}

	return sb.String()
}

func stringField(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

// floatField pulls a numeric field out of the envelope, accepting either
// float64 (the JSON default) or json.Number (when the decoder is configured
// for it).
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

// truncateForTelegram cuts a string at limit, replacing the tail with an
// ellipsis so the receiver knows content was clipped.
func truncateForTelegram(s string, limit int) string {
	r := []rune(s)
	if len(r) <= limit {
		return s
	}
	if limit <= 1 {
		return string(r[:limit])
	}
	return string(r[:limit-1]) + "…"
}
