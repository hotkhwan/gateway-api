// internal/gateways/mediagw/client.go
package mediagw

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"github.com/hotkhwan/gateway-api/internal/logger"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

type Client struct {
	BaseURL string
	Secret  string
}

func NewFromEnv(prefix string) *Client {
	return &Client{
		BaseURL: os.Getenv(prefix + "_API"),    // e.g. ZKT_API=https://aliza.k-lynx.com/media/index/api
		Secret:  os.Getenv(prefix + "_SECRET"), // e.g. ZKT_SECRET=1Sq6rbtDKe8P
	}
}

func (c *Client) Configured() bool { return c.BaseURL != "" && c.Secret != "" }

// ---- helpers ----

func crop(b []byte, max int) string {
	if len(b) <= max {
		return string(b)
	}
	return string(b[:max]) + "...(truncated)"
}

func (c *Client) doGET(ctx context.Context, path string, q url.Values) (map[string]any, int, error) {
	log := logger.FromCtx(ctx, "mediagw", "GET "+path)

	if !c.Configured() {
		return nil, 0, fmt.Errorf("mediagw not configured")
	}

	if q == nil {
		q = url.Values{}
	}
	q.Set("secret", c.Secret)

	u := c.BaseURL + path + "?" + q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("build request: %w", err)
	}

	hc := &http.Client{Timeout: 15 * time.Second}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	log.Debug().
		Str("url", path).
		Int("status", resp.StatusCode).
		Str("qs", q.Encode()).
		Str("resp", crop(body, 800)).
		Msg("↗️ ZLMediaKit GET")

	var out map[string]any
	if err := json.Unmarshal(body, &out); err != nil {
		out = map[string]any{"raw": string(body)}
	}
	return out, resp.StatusCode, nil
}

func (c *Client) doPOSTForm(ctx context.Context, path string, q url.Values, form url.Values) (map[string]any, int, error) {
	log := logger.FromCtx(ctx, "mediagw", "POST "+path)

	if !c.Configured() {
		return nil, 0, fmt.Errorf("mediagw not configured")
	}

	if form == nil {
		form = url.Values{}
	}
	form.Set("secret", c.Secret)

	u := c.BaseURL + path
	if q != nil {
		u += "?" + q.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, 0, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	hc := &http.Client{Timeout: 15 * time.Second}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	// ไม่ log secret
	formSafe := cloneFormWithout(form, "secret")
	formSafe.Del("secret")

	log.Debug().
		Str("url", path).
		Int("status", resp.StatusCode).
		Str("qs", func() string {
			if q == nil {
				return ""
			}
			return q.Encode()
		}()).
		Str("form", formSafe.Encode()).
		Str("resp", crop(body, 800)).
		Msg("↗️ ZLMediaKit POST")

	var out map[string]any
	if err := json.Unmarshal(body, &out); err != nil {
		out = map[string]any{"raw": string(body)}
	}
	return out, resp.StatusCode, nil
}

// ---- API mapping ----
func (c *Client) GetMediaList(ctx context.Context) (map[string]any, int, error) {
	return c.doGET(ctx, "/getMediaList", nil)
}

func (c *Client) GetServerConfig(ctx context.Context) (map[string]any, int, error) {
	return c.doGET(ctx, "/getServerConfig", nil)
}

func (c *Client) GetAllSession(ctx context.Context) (map[string]any, int, error) {
	return c.doGET(ctx, "/getAllSession", nil)
}

type CreateStreamForm struct {
	VHost         string
	App           string
	Stream        string
	URL           string
	EnableHLS     int
	EnableFMP4    int
	EnableMP4     int
	EnableRTMP    int
	EnableRTSP    int
	EnableTS      int
	EnableHLSFMP4 int
}

func (c *Client) AddStreamProxy(ctx context.Context, in CreateStreamForm) (map[string]any, int, error) {
	if in.VHost == "" {
		in.VHost = "__defaultVhost__"
	}
	if in.App == "" {
		in.App = "live"
	}

	form := url.Values{}
	form.Set("vhost", in.VHost)
	form.Set("app", in.App)
	form.Set("stream", in.Stream)
	form.Set("url", in.URL)
	form.Set("enable_hls", itoa01(in.EnableHLS, 1))
	form.Set("enable_fmp4", itoa01(in.EnableFMP4, 0))
	form.Set("enable_mp4", itoa01(in.EnableMP4, 0))
	form.Set("enable_rtmp", itoa01(in.EnableRTMP, 0))
	form.Set("enable_rtsp", itoa01(in.EnableRTSP, 1))
	form.Set("enable_ts", itoa01(in.EnableTS, 0))
	form.Set("enable_hls_fmp4", itoa01(in.EnableHLSFMP4, 0))

	// ✅ ใส่ secret ใน query เหมือน cURL
	q := url.Values{}
	q.Set("secret", c.Secret)

	return c.doPOSTForm(ctx, "/addStreamProxy", q, form)
}

func (c *Client) DelStreamProxy(ctx context.Context, vhost, app, streamID string) (map[string]any, int, error) {
	if vhost == "" {
		vhost = "__defaultVhost__"
	}
	if app == "" {
		app = "live"
	}
	key := vhost + "/" + app + "/" + streamID

	q := url.Values{}
	q.Set("key", key)

	form := url.Values{}
	form.Set("vhost", vhost)
	form.Set("key", key)

	return c.doPOSTForm(ctx, "/delStreamProxy", q, form)
}

func itoa01(v, def int) string {
	if v != 0 && v != 1 {
		v = def
	}
	if v == 1 {
		return "1"
	}
	return "0"
}

func cloneFormWithout(form url.Values, hiddenKeys ...string) url.Values {
	hide := map[string]struct{}{}
	for _, k := range hiddenKeys {
		hide[strings.ToLower(k)] = struct{}{}
	}
	cp := url.Values{}
	for k, v := range form {
		if _, ok := hide[strings.ToLower(k)]; ok {
			continue
		}
		dst := make([]string, len(v))
		copy(dst, v)
		cp[k] = dst
	}
	return cp
}

func (c *Client) GetMediaInfo(ctx context.Context, schema, vhost, app, stream string) (map[string]any, int, error) {
	if schema == "" {
		schema = "rtsp"
	}
	if vhost == "" {
		vhost = "__defaultVhost__"
	}
	if app == "" {
		app = "live"
	}

	q := url.Values{}
	q.Set("secret", c.Secret)
	q.Set("schema", schema)
	q.Set("vhost", vhost)
	q.Set("app", app)
	q.Set("stream", stream)

	return c.doGET(ctx, "/getMediaInfo", q)
}

func (c *Client) KickSession(ctx context.Context, session string) (map[string]any, int, error) {
	q := url.Values{}
	q.Set("id", session)
	return c.doGET(ctx, "/kick_session", q)
}

// KickSessions disconnects TCP sessions by peer_ip and optional local_port
// API: /index/api/kick_sessions?secret=...&peer_ip=...&local_port=554
func (c *Client) KickSessions(ctx context.Context, peerIP string, localPort int) (map[string]any, int, error) {
	q := url.Values{}
	peerIP = strings.TrimSpace(peerIP)
	if peerIP != "" {
		q.Set("peer_ip", peerIP)
	}
	if localPort > 0 {
		q.Set("local_port", fmt.Sprintf("%d", localPort))
	}
	return c.doGET(ctx, "/kick_sessions", q)
}
