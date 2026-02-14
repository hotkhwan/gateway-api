// internal/services/ibocsvc/helpers.go
package ibocsvc

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
	"strconv"
	"strings"
	"sync"
	"time"

	"klynx/models/aimodel"
	"klynx/utils"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func parseDateRangeYYYYMMDD(dateTime string) (time.Time, time.Time, error) {
	now := time.Now()

	// ไม่ส่ง dateTime → วันนี้ 00:00:00 ถึง now
	if strings.TrimSpace(dateTime) == "" {
		start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		return start.UTC(), now.UTC(), nil
	}

	parts := strings.Split(dateTime, ",")
	if len(parts) != 2 {
		return time.Time{}, time.Time{}, fmt.Errorf("invalid dateTime, expected YYYY-MM-DD,YYYY-MM-DD")
	}

	s := strings.TrimSpace(parts[0])
	e := strings.TrimSpace(parts[1])

	start, err := time.Parse("2006-01-02", s)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("invalid start date")
	}
	end, err := time.Parse("2006-01-02", e)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("invalid end date")
	}

	start = start.UTC()
	end = end.UTC().Add(23*time.Hour + 59*time.Minute + 59*time.Second)
	return start, end, nil
}

func toInt64(v any) int64 {
	switch x := v.(type) {
	case int32:
		return int64(x)
	case int64:
		return x
	case int:
		return int64(x)
	case float64:
		return int64(x)
	default:
		return 0
	}
}

func toFloat64(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case float32:
		return float64(x)
	case int:
		return float64(x)
	case int32:
		return float64(x)
	case int64:
		return float64(x)
	default:
		return 0
	}
}

func toStringSlice(v any) []string {
	arr, ok := v.(bson.A)
	if !ok {
		if s, ok := v.([]string); ok {
			return s
		}
		return []string{}
	}
	out := make([]string, 0, len(arr))
	for _, it := range arr {
		out = append(out, fmt.Sprint(it))
	}
	return out
}

func toAnySlice(v any) []any {
	if v == nil {
		return nil
	}
	if arr, ok := v.(bson.A); ok {
		out := make([]any, 0, len(arr))
		for _, it := range arr {
			out = append(out, it)
		}
		return out
	}
	if s, ok := v.([]any); ok {
		return s
	}
	return []any{v}
}

func toTimeISO(v any) string {
	t, ok := v.(time.Time)
	if !ok {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

// แปลงให้กลายเป็น /image/<bucket>/<key> (กัน full URL)
func buildImageProxyURL(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}

	imagePath := utils.GetFilesProxyPath() // "/image"

	if strings.HasPrefix(p, imagePath+"/") {
		return p
	}

	if strings.HasPrefix(p, "http://") || strings.HasPrefix(p, "https://") {
		if idx := strings.Index(p, "://"); idx >= 0 {
			after := p[idx+3:]
			if slash := strings.Index(after, "/"); slash >= 0 {
				p = after[slash:]
			}
		}
	}

	p = strings.TrimLeft(p, "/")
	return imagePath + "/" + p
}

// Blacklist
func toRFC3339(v any) string {
	switch x := v.(type) {
	case time.Time:
		return x.UTC().Format(time.RFC3339)
	case primitive.DateTime:
		return x.Time().UTC().Format(time.RFC3339)
	default:
		return ""
	}
}

type PublishFunc func() error

type Publisher struct {
	mu sync.Mutex

	dirty       bool
	lastEvent   time.Time
	lastPublish time.Time
	timer       *time.Timer

	debounce    time.Duration
	minInterval time.Duration

	publish PublishFunc
}

type Options struct {
	Debounce    time.Duration
	MinInterval time.Duration
	Publish     PublishFunc
}

func New(opts Options) *Publisher {
	return &Publisher{
		debounce:    opts.Debounce,
		minInterval: opts.MinInterval,
		publish:     opts.Publish,
	}
}

// MarkDirty: เรียกเมื่อ "มี event เข้า" แล้วอยากให้ยิง summary ภายหลัง
func (p *Publisher) MarkDirty() {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.dirty = true
	p.lastEvent = time.Now()

	// ✅ ถ้ามี timer รันอยู่แล้ว ไม่ต้อง reset (กัน debounce ไม่จบ)
	if p.timer != nil {
		return
	}

	p.timer = time.NewTimer(p.debounce)
	go p.loop()
}

func (p *Publisher) loop() {
	for {
		<-p.timer.C

		p.mu.Lock()
		if !p.dirty {
			// ✅ ไม่มี event ใหม่แล้ว ไม่ probe ฟรี → ปิด timer จบ
			p.timer = nil
			p.mu.Unlock()
			return
		}
		// ✅ เคลียร์ dirty ก่อน แล้วปล่อย lock
		p.dirty = false
		p.lastPublish = time.Now()
		p.mu.Unlock()

		if p.publish != nil {
			_ = p.publish()
		}

		// ✅ ถ้าระหว่าง publish มี event เข้า MarkDirty จะ set dirty=true
		// เราตั้ง timer ใหม่อีกรอบเพื่อยิงรอบถัดไป “ถ้ามี dirty”
		p.mu.Lock()
		if p.timer == nil {
			// เผื่อถูกหยุดไปแล้ว
			p.timer = time.NewTimer(p.debounce)
			p.mu.Unlock()
			continue
		}
		p.timer.Reset(p.debounce)
		p.mu.Unlock()
	}
}

type IntOpt struct {
	Min *int
	Max *int
}

func EnvInt(key string, def int, opt ...IntOpt) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return clampInt(def, opt...)
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return clampInt(def, opt...)
	}
	return clampInt(n, opt...)
}

func clampInt(n int, opt ...IntOpt) int {
	if len(opt) == 0 {
		return n
	}
	o := opt[0]
	if o.Min != nil && n < *o.Min {
		return *o.Min
	}
	if o.Max != nil && n > *o.Max {
		return *o.Max
	}
	return n
}

func parseIntSetFromEnv(envKey string) map[int]struct{} {
	out := map[int]struct{}{}
	raw := strings.TrimSpace(os.Getenv(envKey))
	if raw == "" {
		return out
	}

	parts := strings.Split(raw, ",")
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if v, err := strconv.Atoi(p); err == nil {
			out[v] = struct{}{}
		}
	}
	return out
}

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

// Login -> Token
func (c *Client) Login(ctx context.Context) (string, error) {
	body := aimodel.ATALoginRequest{
		Name:     c.cfg.Username,
		Password: tripleHashPassword(c.cfg.Username, c.cfg.Password),
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

// ตาม doc: password = sha256(sha256(sha256(password) + 'aliza') + name)
func tripleHashPassword(username, password string) string {
	h1 := sha256.Sum256([]byte(password))
	h1Str := hex.EncodeToString(h1[:])

	h2 := sha256.Sum256([]byte(h1Str + "aliza"))
	h2Str := hex.EncodeToString(h2[:])

	h3 := sha256.Sum256([]byte(h2Str + username))
	return hex.EncodeToString(h3[:])
}

func buildCameraDocFromChannel(c *Client, dev aimodel.ATADevice, ch aimodel.ATAChannel) (map[string]any, error) {
	now := time.Now().UTC()

	if strings.TrimSpace(ch.MainUrl) == "" {
		return nil, fmt.Errorf("channel %d empty mainUrl", ch.ID)
	}

	// --- map ตาม schema camera เดิม ---
	doc := map[string]any{}

	// url / key
	doc["url"] = ch.MainUrl
	doc["key"] = ch.MainUrl // เดิม key = url

	// name / district / brand
	doc["name"] = firstNonEmpty(ch.Name, fmt.Sprintf("ATA-%d-%d", dev.ID, ch.ID))
	doc["district"] = firstNonEmpty(ch.Zone, dev.Name) // ใช้ zone เป็น district
	doc["brand"] = "ATA"                               // หรือ "EDGEAI" ถ้าอยาก

	// channel เดิมคุณเก็บ 1 ตลอด เราจะตาม pattern เดิม
	doc["channel"] = 1

	// เพิ่ม streamID = channel id จาก ATA
	doc["streamID"] = ch.ID
	doc["cameraID"] = ch.ID

	// gbid (ถ้ามี)
	doc["gbid"] = strings.TrimSpace(ch.GB28181ID)

	// // lat / long Mapping
	// var lat, lon float64
	// if v, err := parseFloatStringSafe(ch.Latitude); err == nil {
	// 	lat = v
	// }
	// if v, err := parseFloatStringSafe(ch.Longitude); err == nil {
	// 	lon = v
	// }
	// doc["lat"] = lat
	// doc["long"] = lon

	// state / status
	doc["state"] = "synced"
	doc["status"] = ch.Enable == 1

	// datetime
	doc["dateTimeCreate"] = now
	doc["dateTimeUpdate"] = now

	// ip จาก device.ip หรือ host ใน RTSP
	ip := strings.TrimSpace(dev.IP)
	if ip == "" {
		ip = extractIPFromURL(ch.MainUrl)
	}
	if ip != "" {
		doc["ip"] = ip
	}

	// เพิ่ม meta ATA แยกอีกชั้น
	meta := bson.M{
		"deviceId":   dev.ID,
		"deviceSn":   dev.SN,
		"deviceName": dev.Name,

		"channelId":   ch.ID,
		"channelName": ch.Name,
		"zone":        ch.Zone,
		"enable":      ch.Enable,
		"width":       ch.Width,
		"height":      ch.Height,
		"codec":       ch.Codec,
		"tasks":       ch.Tasks,
		"taskInfos":   ch.TaskInfos,
	}

	// เพิ่ม wss flv url ให้ frontend
	sn := strings.TrimSpace(ch.SN)
	if sn == "" {
		sn = strings.TrimSpace(dev.SN)
	}

	if sn != "" && ch.ID != 0 {
		if ws, err := c.BuildFLVWSURL(sn, ch.ID); err == nil {
			meta["wsFlvUrl"] = ws
			doc["ataWsFlvUrl"] = ws
		}
	}

	doc["ata"] = meta

	return doc, nil
}

func extractIPFromURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err == nil && u.Host != "" {
		host := u.Host
		if i := strings.IndexByte(host, ':'); i >= 0 {
			host = host[:i]
		}
		return host
	}

	// fallback regex ง่าย ๆ
	for _, part := range strings.Fields(raw) {
		if strings.Count(part, ".") == 3 {
			return part
		}
	}
	return ""
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
