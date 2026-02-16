// internal/services/atasvc/client.go
package atasvc

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/hotkhwan/gateway-api/models/aimodel"
)

// Config อ่านจาก config.ATA (ซึ่ง load จาก .env อีกที)
type Config struct {
	BaseAPI   *url.URL
	WSBase    *url.URL
	Username  string
	Password  string
	APIKey    string
	APISecret string
}

type Client struct {
	cfg    Config
	client *http.Client
}

// ============ Public ============

// NewClientFromEnv: ใช้ค่า config.ATA ที่โหลดไว้แล้ว (แม้ชื่อจะยังเขียนว่า FromEnv)
func NewClientFromEnv() (*Client, error) {
	raw := strings.TrimSpace(os.Getenv("ATA_API_URL"))
	if raw == "" {
		return nil, fmt.Errorf("ATA_API_URL is empty")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("invalid ATA_API_URL: %w", err)
	}

	// base ws: https://host:port/api/v1 -> wss://host:port
	wsURL := &url.URL{
		Scheme: "wss",
		Host:   u.Host,
	}
	wsBase, _ := url.Parse(wsURL.String())

	insecure := strings.EqualFold(strings.TrimSpace(os.Getenv("ATA_INSECURE_TLS")), "true")

	cfg := Config{
		BaseAPI:   u,
		WSBase:    wsBase,
		Username:  strings.TrimSpace(os.Getenv("ATA_USERNAME")),
		Password:  os.Getenv("ATA_PASSWORD"),
		APIKey:    strings.TrimSpace(os.Getenv("ATA_API_KEY")),
		APISecret: os.Getenv("ATA_API_SECRET"),
	}

	if cfg.Username == "" || cfg.Password == "" {
		return nil, fmt.Errorf("ATA_USERNAME or ATA_PASSWORD is empty")
	}

	httpClient := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				Timeout:   5 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: insecure, // ⚠ ใช้เฉพาะตอน dev/self-signed
			},
		},
	}

	return &Client{
		cfg:    cfg,
		client: httpClient,
	}, nil
}

// NewClientFromConfig: สร้าง client จากค่าที่เตรียมไว้แล้ว (เช่นจาก Mongo edge config)
func NewClientFromConfig(cfg Config, insecureTLS bool) (*Client, error) {
	if cfg.BaseAPI == nil {
		return nil, fmt.Errorf("BaseAPI is nil")
	}
	if cfg.WSBase == nil {
		// ถ้าไม่ส่งมา จะ derive จาก BaseAPI ให้
		wsURL := &url.URL{
			Scheme: "wss",
			Host:   cfg.BaseAPI.Host,
		}
		cfg.WSBase = wsURL
	}
	if strings.TrimSpace(cfg.Username) == "" || cfg.Password == "" {
		return nil, fmt.Errorf("Username or Password is empty")
	}

	httpClient := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				Timeout:   5 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: insecureTLS, // ⚠ self-signed/dev only
			},
		},
	}

	return &Client{
		cfg:    cfg,
		client: httpClient,
	}, nil
}

// Login -> Token
func (c *Client) Login(ctx context.Context) (string, error) {
	body := aimodel.ATALoginRequest{
		Name:     c.cfg.Username,
		Password: ataTripleHashPassword(c.cfg.Username, c.cfg.Password),
	}

	var resp aimodel.ATAResponse[aimodel.ATALoginData]
	if err := c.doJSON(ctx, http.MethodPost, "/user/login", nil, body, &resp); err != nil {
		return "", err
	}
	if resp.ErrCode != 0 {
		return "", fmt.Errorf("ATA login failed: %d %s", resp.ErrCode, resp.ErrMsg)
	}
	if resp.Data.Token == "" {
		return "", fmt.Errorf("ATA login response has empty token")
	}
	return resp.Data.Token, nil
}

// GetDevices ใช้ Token header (web-style)
func (c *Client) GetDevices(ctx context.Context, token string) ([]aimodel.ATADevice, error) {
	reqBody := aimodel.ATADeviceListRequest{
		PageIndex: 1,
		PageSize:  50,
	}

	var resp aimodel.ATAResponse[aimodel.ATADeviceListData]
	h := map[string]string{
		"Token": token,
	}
	if err := c.doJSON(ctx, http.MethodPost, "/device/get-devices", h, reqBody, &resp); err != nil {
		return nil, err
	}
	if resp.ErrCode != 0 {
		return nil, fmt.Errorf("GetDevices failed: %d %s", resp.ErrCode, resp.ErrMsg)
	}
	devs := resp.Data.Devices
	if len(devs) == 0 && len(resp.Data.List) > 0 {
		devs = resp.Data.List
	}
	return devs, nil
}

// GetChannelsByDevice -> channels (RTSP)
func (c *Client) GetChannelsByDevice(ctx context.Context, token string, deviceID int64) ([]aimodel.ATAChannel, error) {
	reqBody := aimodel.ATAChannelListRequest{
		PageIndex: 1,
		PageSize:  100,
		DeviceId:  deviceID,
	}

	var resp aimodel.ATAResponse[aimodel.ATAChannelListData]
	h := map[string]string{
		"Token": token,
	}
	if err := c.doJSON(ctx, http.MethodPost, "/channel/get-channels", h, reqBody, &resp); err != nil {
		return nil, err
	}
	if resp.ErrCode != 0 {
		return nil, fmt.Errorf("GetChannels failed: %d %s", resp.ErrCode, resp.ErrMsg)
	}
	return resp.Data.List, nil
}

// GetChannelDetail ใช้สำหรับ endpoint stream โดยตรง (ไม่แตะ DB ก็ได้)
func (c *Client) GetChannelDetail(ctx context.Context, token string, channelID int64) (*aimodel.ATAChannel, error) {
	reqBody := aimodel.ATAChannelDetailRequest{ID: channelID}

	var resp aimodel.ATAResponse[aimodel.ATAChannel]
	h := map[string]string{"Token": token}

	if err := c.doJSON(ctx, http.MethodPost, "/channel/get-channel", h, reqBody, &resp); err != nil {
		return nil, err
	}
	if resp.ErrCode != 0 {
		return nil, fmt.Errorf("GetChannelDetail failed: %d %s", resp.ErrCode, resp.ErrMsg)
	}
	return &resp.Data, nil
}

// / BuildFLVWSURL: สร้าง WSS URL ตาม pattern
// wss://host:port/media-api/ws/flv?src=edgertsp%253A%252F%252F...
func (c *Client) BuildFLVWSURL(sn string, channelID int64) (string, error) {
	if sn == "" || channelID == 0 {
		return "", fmt.Errorf("sn or channelID is empty")
	}
	if c.cfg.WSBase == nil || c.cfg.BaseAPI == nil {
		return "", fmt.Errorf("WSBase or BaseAPI not configured")
	}

	hostPort := c.cfg.BaseAPI.Host // atanywhere.ddns.net:8081

	// raw src
	srcRaw := fmt.Sprintf(
		"edgertsp://%s/%s/%d?chid=%d&sn=%s&enable=1&chn=1",
		hostPort, sn, channelID, channelID, sn,
	)

	// ✅ encode แค่ชั้นเดียวให้เป็น %3A %2F %26
	srcEnc := url.QueryEscape(srcRaw)

	ws := *c.cfg.WSBase
	ws.Path = "/media-api/ws/flv"

	// ✅ เขียน RawQuery ตรง ๆ จะไม่ถูก encode เพิ่ม
	ws.RawQuery = "src=" + srcEnc

	return ws.String(), nil
}

// ============ Internal helpers ============

func (c *Client) doJSON(ctx context.Context, method, path string, headers map[string]string, body any, out any) error {
	u := *c.cfg.BaseAPI
	u.Path = strings.TrimRight(u.Path, "/") + path

	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			return fmt.Errorf("encode body: %w", err)
		}
	}

	req, err := http.NewRequestWithContext(ctx, method, u.String(), &buf)
	if err != nil {
		return fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	for k, v := range headers {
		req.Header.Set(k, v)
	}

	res, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("http do: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode < 200 || res.StatusCode >= 300 {
		var raw any
		_ = json.NewDecoder(res.Body).Decode(&raw)
		return fmt.Errorf("ATA HTTP %d: %v", res.StatusCode, raw)
	}

	if out == nil {
		return nil
	}
	if err := json.NewDecoder(res.Body).Decode(out); err != nil {
		return fmt.Errorf("decode json: %w", err)
	}
	return nil
}

// ตาม doc: password = sha256(sha256(sha256(password) + 'AVCIT') + name)
func ataTripleHashPassword(username, password string) string {
	h1 := sha256.Sum256([]byte(password))
	h1Str := hex.EncodeToString(h1[:])

	h2 := sha256.Sum256([]byte(h1Str + "AVCIT"))
	h2Str := hex.EncodeToString(h2[:])

	h3 := sha256.Sum256([]byte(h2Str + username))
	return hex.EncodeToString(h3[:])
}
