// internal/gateways/webhookgw/client.go
package webhookgw

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/hotkhwan/gateway-api/internal/logger"
	"github.com/hotkhwan/gateway-api/models/authzmod"
	"github.com/rs/zerolog"
)

// Client sends events to webhook targets
type Client struct {
	config authzmod.TargetConfig
	client *http.Client
	logger zerolog.Logger
}

// NewClient creates a new webhook client
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
		logger: logger.WithMeta("webhookgw", "Client"),
	}
}

// Send sends an event to the webhook URL
func (c *Client) Send(ctx context.Context, event interface{}, payload []byte) error {
	if c.config.URL == "" {
		return fmt.Errorf("webhook URL is empty")
	}

	// Create request
	req, err := http.NewRequestWithContext(ctx, "POST", c.config.URL, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
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
		c.logger.Warn().Msg("webhook signing enabled but not implemented yet")
	}

	// Send request
	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send webhook: %w", err)
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webhook returned non-success status: %d", resp.StatusCode)
	}

	c.logger.Debug().
		Str("url", c.config.URL).
		Int("statusCode", resp.StatusCode).
		Msg("webhook sent successfully")

	return nil
}

// SendWithRetry sends event with retry logic
func (c *Client) SendWithRetry(ctx context.Context, event interface{}, payload []byte, maxRetries int) error {
	var lastErr error

	for i := 0; i < maxRetries; i++ {
		err := c.Send(ctx, event, payload)
		if err == nil {
			return nil
		}

		lastErr = err

		if i < maxRetries-1 {
			// Exponential backoff
			backoff := time.Duration(1<<uint(i)) * time.Second
			select {
			case <-time.After(backoff):
				c.logger.Warn().
					Int("attempt", i+1).
					Dur("backoff", backoff).
					Err(err).
					Msg("webhook retry")
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}

	return fmt.Errorf("webhook failed after %d retries: %w", maxRetries, lastErr)
}
