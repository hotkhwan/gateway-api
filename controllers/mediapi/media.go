// controllers/mediapi/media.go
package mediapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"

	"klynx/internal/gateways/mediagw"
	"klynx/internal/repo/cachego/cacheklive"
	"klynx/internal/services/klivesvc"
	"klynx/models"
	"klynx/models/cachemod"
	"klynx/models/klivemod"
	"klynx/utils/httputil"
	"klynx/utils/traceutil"
)

// ===== lazy init client =====
var (
	mediaClient *mediagw.Client
	mediaOnce   sync.Once
)

func getClient() *mediagw.Client {
	mediaOnce.Do(func() {
		c := mediagw.NewFromEnv("ZKT")
		if c != nil && c.BaseURL != "" && strings.HasSuffix(c.BaseURL, "/media/index/api") {
			c.BaseURL = strings.TrimSuffix(c.BaseURL, "/media/index/api") + "/index/api"
		}
		mediaClient = c
	})
	return mediaClient
}

func d01p(p *int, def int) int {
	if p == nil {
		return def
	}
	if *p != 0 && *p != 1 {
		return def
	}
	return *p
}

// ListStream godoc
// @Summary      List media/proxies
// @Description  Proxy ไป ZLMediaKit: /getMediaList
// @Tags         Media
// @Produce      json
// @Success      200 {object} models.APIResponse[models.Any]
// @Failure      502 {object} gmod.ErrorResponse
// @Router       /media/stream [get]
// @Security     BearerAuth
func ListStream(c *fiber.Ctx) error {
	ctx, span, _ := traceutil.Start(c.UserContext(), "klynx/mediapi", "ListStream", "mediapi", "ListStream")
	defer span.End()

	out, status, err := getClient().GetMediaList(ctx)
	if err != nil {
		return httputil.FailNotFound(c, "UPSTREAM_ERROR", err.Error())
	}
	return c.Status(status).JSON(models.APIResponse[models.Any]{
		Code:    "SUCCESS",
		Message: "OK",
		Status:  true,
		Details: out,
	})
}

// CreateStream godoc
// @Summary      Create stream proxy
// @Description  Proxy ไป ZLMediaKit: /addStreamProxy (x-www-form-urlencoded)
// @Tags         Media
// @Accept       json
// @Produce      json
// @Param        body body models.CreateStreamRequest true "Create stream proxy payload"
// @Success      200 {object} models.APIResponse[models.Any]
// @Failure      400 {object} gmod.ErrorResponse
// @Failure      502 {object} gmod.ErrorResponse
// @Router       /media/stream [post]
// @Security     BearerAuth
func CreateStream(c *fiber.Ctx) error {
	ctx, span, log := traceutil.Start(
		c.UserContext(),
		"klynx/mediapi",
		"CreateStream",
		"mediapi", "CreateStream",
	)
	defer span.End()

	ctx2, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	var req models.CreateStreamRequest
	if err := c.BodyParser(&req); err != nil {
		return httputil.FailBadRequest(c, "INVALID_BODY", "Invalid request body")
	}

	req.Stream = strings.TrimSpace(req.Stream)
	req.URL = strings.TrimSpace(req.URL)
	if req.Stream == "" || req.URL == "" {
		return httputil.FailBadRequest(c, "BAD_REQUEST", "stream and url are required")
	}

	client := getClient()
	if client == nil || !client.Configured() {
		return httputil.FailInternalReason(c, "MEDIA_NOT_CONFIGURED", "media gateway is not configured")
	}

	vhost := strings.TrimSpace(req.VHost)
	if vhost == "" {
		vhost = "__defaultVhost__"
	}
	app := strings.TrimSpace(req.App)
	if app == "" {
		app = "live"
	}

	// ✅ เก็บ clientInfo จาก header จริง (ของ user) ตั้งแต่ CreateStream
	ttlSec := klivesvc.GetEffectiveSessionTTL(ctx2)
	if ttlSec <= 0 {
		ttlSec = 15
	}
	ttl := time.Duration(ttlSec) * time.Second
	ci := klivesvc.ExtractClientInfo(c)

	// ✅ trigger key: ทำให้ TTL reset ทุกครั้งที่กด stream เดิม
	_ = cacheklive.SetStreamTTLKey(ctx2, req.Stream, ttl)

	// ✅ clientinfo json (จะเก็บไว้ก็ได้)
	_ = cacheklive.SetKliveClientInfoByStream(ctx2, req.Stream, &ci, ttl)

	// ✅ peer_ip อยู่ยาวกว่า trigger เสมอ เพื่อให้ watcher อ่านได้หลัง trigger หมด
	peerIP := strings.TrimSpace(ci.IP)
	if peerIP == "" && ci.RawHeaders != nil {
		peerIP = strings.TrimSpace(ci.RawHeaders["cf-connecting-ip"])
		if peerIP == "" {
			peerIP = strings.TrimSpace(ci.RawHeaders["true-client-ip"])
		}
		if peerIP == "" {
			peerIP = strings.TrimSpace(ci.RawHeaders["x-real-ip"])
		}
		if peerIP == "" {
			peerIP = strings.TrimSpace(strings.Split(ci.RawHeaders["x-forwarded-for"], ",")[0])
		}
	}

	// จากเดิม ttl+5s → เปลี่ยนเป็น ttl+60s
	peerTTL := ttl + 60*time.Second
	if err := cacheklive.SetKlivePeerIPByStream(ctx2, req.Stream, peerIP, peerTTL); err != nil {
		log.Warn().Err(err).Str("stream", req.Stream).Msg("cache peer_ip failed")
	}

	if err := cacheklive.SetKlivePeerIPByStream(ctx2, req.Stream, peerIP, peerTTL); err != nil {
		log.Warn().Err(err).Str("stream", req.Stream).Msg("cache peer_ip failed")
	}

	log.Debug().
		Str("stream", req.Stream).
		Int("ttlSec", ttlSec).
		Str("ip", ci.IP).
		Msg("✅ cached klive clientInfo by stream")

	// ====== CACHE FAST-PATH ======
	deviceID := req.Stream
	urlHash := cacheklive.HashURL(req.URL)

	// 1) เช็คของจริงกับ ZLM ก่อน
	info, codeInfo, errInfo := client.GetMediaInfo(ctx2, "rtsp", vhost, app, req.Stream)
	if errInfo == nil && codeInfo == 200 && parseZlmOK(info) {
		alive, speed, ready := parseReady(info)

		now := time.Now().UTC()
		_ = cacheklive.SetDeviceStream(ctx2, deviceID, &cachemod.MediaStreamCache{
			DeviceId:    deviceID,
			StreamKey:   req.Stream,
			URLHash:     urlHash,
			VHost:       vhost,
			App:         app,
			IsPublic:    false,
			CreatedAt:   now,
			LastReadyAt: now,
			LastSeenAt:  now,
		}, 30*time.Minute)

		return c.Status(fiber.StatusOK).JSON(models.APIResponse[models.Any]{
			Code:    "SUCCESS",
			Message: "Stream ensured",
			Status:  true,
			Details: map[string]any{
				"stream":        req.Stream,
				"url":           req.URL,
				"exists_before": true,
				"created_now":   false,
				"ready":         ready,
				"mediaInfo":     info,
				"aliveSecond":   alive,
				"bytesSpeed":    speed,
			},
		})
	}

	_ = cacheklive.DeleteDeviceStream(ctx2, deviceID)

	// 2) lock กันสร้างแข่งกัน
	locked, lerr := cacheklive.AcquireStreamLock(ctx2, req.Stream, 10*time.Second)
	if lerr != nil {
		log.Warn().Err(lerr).Msg("⚠️ acquire stream lock failed")
	}
	if locked {
		defer cacheklive.ReleaseStreamLock(ctx2, req.Stream)
	}

	// 3) สร้าง proxy
	out, _, err := client.AddStreamProxy(ctx2, mediagw.CreateStreamForm{
		VHost:         vhost,
		App:           app,
		Stream:        req.Stream,
		URL:           req.URL,
		EnableHLS:     d01p(req.EnableHLS, 0),
		EnableFMP4:    d01p(req.EnableFMP4, 0),
		EnableMP4:     d01p(req.EnableMP4, 0),
		EnableRTMP:    d01p(req.EnableRTMP, 0),
		EnableRTSP:    d01p(req.EnableRTSP, 1),
		EnableTS:      d01p(req.EnableTS, 0),
		EnableHLSFMP4: d01p(req.EnableHLSFMP4, 0),
	})
	if err != nil {
		log.Error().Err(err).Msg("❌ add stream proxy failed")
		return httputil.FailNotFound(c, "UPSTREAM_ERROR", err.Error())
	}

	zlmCode := toInt(out["code"])
	zlmMsg, _ := out["msg"].(string)

	created := false
	switch zlmCode {
	case 0:
		created = true
	case -1:
		// already exists
	default:
		return httputil.FailNotFound(c, "UPSTREAM_ERROR",
			fmt.Sprintf("zlm addStreamProxy code=%d msg=%s", zlmCode, zlmMsg))
	}

	// 4) รอ ready สั้น ๆ
	ready := false
	var lastInfo map[string]any
	var alive int
	var speed float64

	for i := 0; i < 12; i++ {
		info3, code3, err3 := client.GetMediaInfo(ctx2, "rtsp", vhost, app, req.Stream)
		if err3 == nil && code3 == 200 && parseZlmOK(info3) {
			alive, speed, ready = parseReady(info3)
			lastInfo = info3
			if ready {
				break
			}
		}
		select {
		case <-ctx2.Done():
			return httputil.FailInternal(c, "TIMEOUT", "context canceled")
		case <-time.After(500 * time.Millisecond):
		}
	}

	now := time.Now().UTC()
	_ = cacheklive.SetDeviceStream(ctx2, deviceID, &cachemod.MediaStreamCache{
		DeviceId:    deviceID,
		StreamKey:   req.Stream,
		URLHash:     urlHash,
		VHost:       vhost,
		App:         app,
		IsPublic:    false,
		CreatedAt:   now,
		LastReadyAt: now,
		LastSeenAt:  now,
	}, 30*time.Minute)

	return c.Status(fiber.StatusOK).JSON(models.APIResponse[models.Any]{
		Code:    "SUCCESS",
		Message: "Stream ensured",
		Status:  true,
		Details: map[string]any{
			"stream":        req.Stream,
			"url":           req.URL,
			"exists_before": false,
			"created_now":   created,
			"ready":         ready,
			"aliveSecond":   alive,
			"bytesSpeed":    speed,
			"mediaInfo":     lastInfo,
		},
	})
}

// DeleteStream godoc
// @Summary      Delete stream proxy
// @Description  Proxy ไป ZLMediaKit: /delStreamProxy
// @Tags         Media
// @Produce      json
// @Param        streamId path string true "Stream ID"
// @Param        vhost    query string false "__defaultVhost__"
// @Param        app      query string false "live"
// @Success      200 {object} models.APIResponse[models.Any]
// @Failure      502 {object} gmod.ErrorResponse
// @Router       /media/stream/{streamId} [delete]
// @Security     BearerAuth
func DeleteStream(c *fiber.Ctx) error {
	ctx, span, log := traceutil.Start(c.UserContext(), "klynx/mediapi", "DeleteStream", "mediapi", "DeleteStream")
	defer span.End()

	streamID := c.Params("streamId")
	vhost := c.Query("vhost", "__defaultVhost__")
	app := c.Query("app", "live")

	out, status, err := getClient().DelStreamProxy(ctx, vhost, app, streamID)
	if err != nil {
		log.Error().Err(err).Msg("❌ delete stream proxy failed")
		return httputil.FailNotFound(c, "UPSTREAM_ERROR", err.Error())
	}

	code := status
	if code == 0 {
		code = fiber.StatusOK
	}
	return c.Status(code).JSON(models.APIResponse[models.Any]{
		Code:    "SUCCESS",
		Message: "Stream proxy deleted",
		Status:  true,
		Details: out,
	})
}

// -------------------- Helpers --------------------

func toInt(v any) int {
	switch t := v.(type) {
	case float64:
		return int(t)
	case float32:
		return int(t)
	case int:
		return t
	case int64:
		return int(t)
	case json.Number:
		i, _ := t.Int64()
		return int(i)
	default:
		return 0
	}
}

func toFloat(v any) float64 {
	switch t := v.(type) {
	case float64:
		return t
	case float32:
		return float64(t)
	case int:
		return float64(t)
	case int64:
		return float64(t)
	case json.Number:
		f, _ := t.Float64()
		return f
	default:
		return 0
	}
}

func parseZlmOK(info map[string]any) bool {
	if info == nil {
		return false
	}
	return toInt(info["code"]) == 0
}

func parseReady(info map[string]any) (alive int, speed float64, ready bool) {
	if info == nil {
		return 0, 0, false
	}
	alive = toInt(info["aliveSecond"])
	speed = toFloat(info["bytesSpeed"])
	ok := parseZlmOK(info)
	ready = ok && (alive >= 1 || speed > 0)
	return
}

func extractClientInfo(c *fiber.Ctx) klivemod.ClientInfo {
	ip := strings.TrimSpace(c.IP())
	ua := strings.TrimSpace(c.Get("User-Agent"))
	lang := strings.TrimSpace(c.Get("Accept-Language"))

	raw := map[string]string{
		"user-agent":         ua,
		"accept-language":    lang,
		"x-forwarded-for":    strings.TrimSpace(c.Get("X-Forwarded-For")),
		"x-real-ip":          strings.TrimSpace(c.Get("X-Real-IP")),
		"forwarded":          strings.TrimSpace(c.Get("Forwarded")),
		"cf-connecting-ip":   strings.TrimSpace(c.Get("CF-Connecting-IP")),
		"true-client-ip":     strings.TrimSpace(c.Get("True-Client-IP")),
		"sec-ch-ua":          strings.TrimSpace(c.Get("Sec-CH-UA")),
		"sec-ch-ua-platform": strings.TrimSpace(c.Get("Sec-CH-UA-Platform")),
		"sec-ch-ua-mobile":   strings.TrimSpace(c.Get("Sec-CH-UA-Mobile")),
		"x-forwarded-proto":  strings.TrimSpace(c.Get("X-Forwarded-Proto")),
		"x-forwarded-host":   strings.TrimSpace(c.Get("X-Forwarded-Host")),
		"x-forwarded-port":   strings.TrimSpace(c.Get("X-Forwarded-Port")),
		"host":               strings.TrimSpace(c.Get("Host")),
		"origin":             strings.TrimSpace(c.Get("Origin")),
		"referer":            strings.TrimSpace(c.Get("Referer")),
	}

	ipChain := extractIPChain(ip, raw["cf-connecting-ip"], raw["true-client-ip"], raw["x-real-ip"], raw["x-forwarded-for"], raw["forwarded"])
	browser, osName, device := parseUAQuick(ua, raw["sec-ch-ua"], raw["sec-ch-ua-platform"], raw["sec-ch-ua-mobile"])
	traceID := traceutil.TraceIDFromCtx(c.UserContext())

	return klivemod.ClientInfo{
		IP:         ip,
		UA:         ua,
		Browser:    browser,
		OS:         osName,
		Device:     device,
		Lang:       lang,
		IPChain:    ipChain,
		RawHeaders: raw,
		Origin:     raw["origin"],
		Referer:    raw["referer"],
		TraceID:    traceID,
	}
}

func normalizeIP2(s string) string {
	ip := strings.TrimSpace(s)
	if ip == "" {
		return ""
	}
	host, _, err := net.SplitHostPort(ip)
	if err == nil && host != "" {
		ip = host
	}
	ip = strings.Trim(ip, "[]")
	if net.ParseIP(ip) == nil {
		return ""
	}
	return ip
}

func extractIPChain(cip, cf, trueClient, xReal, xff, fwd string) []string {
	seen := map[string]bool{}
	out := []string{}

	add := func(s string) {
		s = normalizeIP2(s)
		if s == "" || seen[s] {
			return
		}
		seen[s] = true
		out = append(out, s)
	}

	add(cf)
	add(trueClient)
	add(xReal)
	add(cip)

	for _, part := range strings.Split(xff, ",") {
		add(part)
	}

	lf := strings.ToLower(fwd)
	for _, seg := range strings.Split(lf, ",") {
		seg = strings.TrimSpace(seg)
		if i := strings.Index(seg, "for="); i >= 0 {
			v := strings.TrimSpace(seg[i+4:])
			v = strings.Trim(v, `"'`)
			v = strings.Trim(v, "[]")
			add(v)
		}
	}
	return out
}

func parseUAQuick(ua, chUA, chPlatform, chMobile string) (browser, osName, device string) {
	u := strings.ToLower(ua)

	device = "Desktop"
	if strings.Contains(u, "mobile") || strings.Contains(strings.ToLower(chMobile), "?1") {
		device = "Mobile"
	}

	if p := strings.ToLower(chPlatform); p != "" {
		switch {
		case strings.Contains(p, "android"):
			osName = "Android"
		case strings.Contains(p, "ios"):
			osName = "iOS"
		case strings.Contains(p, "windows"):
			osName = "Windows"
		case strings.Contains(p, "mac"):
			osName = "macOS"
		default:
			osName = chPlatform
		}
	} else {
		switch {
		case strings.Contains(u, "android"):
			osName = "Android"
		case strings.Contains(u, "iphone") || strings.Contains(u, "ipad"):
			osName = "iOS"
		case strings.Contains(u, "mac os x") || strings.Contains(u, "macintosh"):
			osName = "macOS"
		case strings.Contains(u, "windows"):
			osName = "Windows"
		default:
			osName = "Unknown"
		}
	}

	switch {
	case strings.Contains(u, "edg/"):
		browser = "Edge"
	case strings.Contains(u, "chrome/"):
		browser = "Chrome"
	case strings.Contains(u, "safari/") && !strings.Contains(u, "chrome/"):
		browser = "Safari"
	case strings.Contains(u, "firefox/"):
		browser = "Firefox"
	default:
		browser = "Unknown"
	}

	_ = chUA
	return
}
