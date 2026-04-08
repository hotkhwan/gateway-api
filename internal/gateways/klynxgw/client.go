// internal/gateways/klynxgw/client.go
package klynxgw

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/hotkhwan/gateway-api/internal/eventschema"
	"github.com/hotkhwan/gateway-api/utils/traceutil"
)

// Client delivers normalized events to klynx-api via HTTPS POST (saasPublic profile only).
//
// Auth: phibek HMAC scheme —
//
//	X-Phibek-Timestamp: unix seconds (int64)
//	X-Phibek-Signature: sha256=HMAC-SHA256(secret, timestamp + "." + sha256(body))
//
// Config loaded from env:
//
//	KLYNX_DELIVERY_WEBHOOK_URL    — klynx-api endpoint (e.g. https://api.klynx.io/phibek/events)
//	KLYNX_DELIVERY_WEBHOOK_SECRET — HMAC secret (must match klynx-api PHIBEK_WEBHOOK_SECRET)
type Client struct {
	url    string
	secret string
	http   *http.Client
}

// New creates a Client. Returns an error if KLYNX_DELIVERY_WEBHOOK_URL is not set.
func New() (*Client, error) {
	url := os.Getenv("KLYNX_DELIVERY_WEBHOOK_URL")
	if url == "" {
		return nil, fmt.Errorf("klynxgw: KLYNX_DELIVERY_WEBHOOK_URL not set")
	}
	return &Client{
		url:    url,
		secret: os.Getenv("KLYNX_DELIVERY_WEBHOOK_SECRET"),
		http:   &http.Client{Timeout: 15 * time.Second},
	}, nil
}

// Send POSTs a normalized event to klynx-api with HMAC signature headers.
// Trace context is injected via standard W3C traceparent/tracestate headers.
func (c *Client) Send(ctx context.Context, event eventschema.NormalizedEvent) error {
	body, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("klynxgw: marshal event: %w", err)
	}

	ts := strconv.FormatInt(time.Now().Unix(), 10)
	sig := computeHMAC(c.secret, ts, body)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("klynxgw: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Phibek-Timestamp", ts)
	req.Header.Set("X-Phibek-Signature", "sha256="+sig)

	// Propagate trace context into HTTP headers
	traceHeaders := map[string]string{}
	traceutil.InjectHeaders(ctx, traceHeaders)
	for k, v := range traceHeaders {
		req.Header.Set(k, v)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("klynxgw: POST failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("klynxgw: non-2xx response: %d", resp.StatusCode)
	}
	return nil
}

// computeHMAC computes HMAC-SHA256(secret, timestamp + "." + sha256(body)).
// This matches the signature scheme verified by klynx-api's VerifyPhibekHMAC middleware.
func computeHMAC(secret, ts string, body []byte) string {
	bodyHashBytes := sha256.Sum256(body)
	bodyHash := hex.EncodeToString(bodyHashBytes[:])
	data := ts + "." + bodyHash

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(data))
	return hex.EncodeToString(mac.Sum(nil))
}
