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

// LINE Messaging API endpoints
const (
	lineAPIPush      = "https://api.line.me/v2/bot/message/push"
	lineAPIMulticast = "https://api.line.me/v2/bot/message/multicast"
	lineAPIBroadcast = "https://api.line.me/v2/bot/message/broadcast"
)

// LINE Message API request structures
type linePushRequest struct {
	To       string        `json:"to"`
	Messages []lineMessage `json:"messages"`
}

type lineMulticastRequest struct {
	To       []string      `json:"to"`
	Messages []lineMessage `json:"messages"`
}

type lineBroadcastRequest struct {
	Messages []lineMessage `json:"messages"`
}

// lineMessage is a polymorphic message container. For "text" messages, Text is set.
// For "flex" messages, AltText + Contents are set. Unused fields stay empty and are
// omitted from the wire JSON via the omitempty tags.
type lineMessage struct {
	Type     string         `json:"type"`
	Text     string         `json:"text,omitempty"`
	AltText  string         `json:"altText,omitempty"`
	Contents map[string]any `json:"contents,omitempty"`
}

type lineErrorResponse struct {
	Message string `json:"message"`
	Details []struct {
		Message  string `json:"message"`
		Property string `json:"property"`
	} `json:"details"`
}

// Client sends events to LINE targets
type Client struct {
	config authzmod.TargetConfig
	client *http.Client
	logger zerolog.Logger
	token  string
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

// Send sends an event to LINE Messaging API.
// Routing: len(To)==0 → broadcast, len(To)==1 → push, len(To)>1 → multicast.
//
// Message shape: if payload contains a "flex" object (built by the delivery
// pipeline from template + binaryRefs), sends a Flex message; otherwise falls
// back to a plain-text message composed of title + body.
func (c *Client) Send(ctx context.Context, event interface{}, payload []byte) error {
	token := c.config.ChannelAccessToken
	if token == "" {
		token = c.config.ChannelAccessTokenRef
	}
	if token == "" {
		return fmt.Errorf("LINE channel access token is empty")
	}
	c.token = token

	messages := buildLineMessages(payload)

	recipients := c.config.To

	switch {
	case len(recipients) == 0:
		// Broadcast to all followers
		return c.broadcast(ctx, messages)
	case len(recipients) == 1:
		// Push to single user
		return c.push(ctx, recipients[0], messages)
	default:
		// Multicast to multiple users (max 500 per API call)
		return c.multicast(ctx, recipients, messages)
	}
}

func (c *Client) push(ctx context.Context, to string, messages []lineMessage) error {
	body, _ := json.Marshal(linePushRequest{To: to, Messages: messages})
	err := c.doRequest(ctx, lineAPIPush, body)
	if err == nil {
		c.logger.Debug().Str("to", to).Msg("LINE push sent")
	}
	return err
}

func (c *Client) multicast(ctx context.Context, to []string, messages []lineMessage) error {
	// LINE multicast supports max 500 recipients per call
	const batchSize = 500
	for i := 0; i < len(to); i += batchSize {
		end := i + batchSize
		if end > len(to) {
			end = len(to)
		}
		batch := to[i:end]
		body, _ := json.Marshal(lineMulticastRequest{To: batch, Messages: messages})
		if err := c.doRequest(ctx, lineAPIMulticast, body); err != nil {
			return fmt.Errorf("multicast batch %d-%d: %w", i, end-1, err)
		}
	}
	c.logger.Debug().Int("recipients", len(to)).Msg("LINE multicast sent")
	return nil
}

func (c *Client) broadcast(ctx context.Context, messages []lineMessage) error {
	body, _ := json.Marshal(lineBroadcastRequest{Messages: messages})
	err := c.doRequest(ctx, lineAPIBroadcast, body)
	if err == nil {
		c.logger.Debug().Msg("LINE broadcast sent")
	}
	return err
}

func (c *Client) doRequest(ctx context.Context, url string, body []byte) error {
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create LINE request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send LINE message: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errResp lineErrorResponse
		if err := json.NewDecoder(resp.Body).Decode(&errResp); err == nil {
			return fmt.Errorf("LINE API error (%d): %s - %v", resp.StatusCode, errResp.Message, errResp.Details)
		}
		return fmt.Errorf("LINE API returned status %d", resp.StatusCode)
	}
	return nil
}

// buildLineMessages produces the []lineMessage to send: prefers Flex when the
// payload envelope carries a "flex" object; otherwise emits a plain-text message.
func buildLineMessages(payload []byte) []lineMessage {
	var data map[string]any
	if err := json.Unmarshal(payload, &data); err != nil {
		return []lineMessage{{Type: "text", Text: string(payload)}}
	}

	if flex, ok := data["flex"].(map[string]any); ok && len(flex) > 0 {
		alt, _ := data["title"].(string)
		if alt == "" {
			alt, _ = data["body"].(string)
		}
		if alt == "" {
			alt = "Event notification"
		}
		return []lineMessage{{Type: "flex", AltText: alt, Contents: flex}}
	}

	return []lineMessage{{Type: "text", Text: buildMessageText(data)}}
}

// buildMessageText returns a plain-text rendering of the event envelope.
// Used as the LINE message body when no Flex bubble is built (e.g. no message
// template configured). Lat/lng and the eventDetailsUrl are appended so the
// receiver gets the same context the Flex card would have shown.
func buildMessageText(data map[string]any) string {
	title, _ := data["title"].(string)
	body, _ := data["body"].(string)

	var head string
	switch {
	case title != "" && body != "":
		head = fmt.Sprintf("%s\n%s", title, body)
	case body != "":
		head = body
	case title != "":
		head = title
	default:
		eventType, _ := data["eventType"].(string)
		eventId, _ := data["eventId"].(string)
		head = fmt.Sprintf("New Event\nType: %s\nID: %s", eventType, eventId)
	}

	out := head
	if lat, ok := data["lat"].(float64); ok {
		if lng, ok2 := data["lng"].(float64); ok2 && (lat != 0 || lng != 0) {
			out += fmt.Sprintf("\n📍 https://www.google.com/maps/search/?api=1&query=%f,%f", lat, lng)
		}
	}
	if url, ok := data["eventDetailsUrl"].(string); ok && url != "" {
		out += "\n🔎 " + url
	}
	return out
}
