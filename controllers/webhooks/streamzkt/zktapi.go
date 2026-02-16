// controllers/webhooks/streamzkt/zktapi.go
package streamzkt

import (
	"context"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"

	"github.com/hotkhwan/gateway-api/internal/gateways/mediagw"
	"github.com/hotkhwan/gateway-api/internal/kafka"
	"github.com/hotkhwan/gateway-api/internal/logger"
	"github.com/hotkhwan/gateway-api/internal/repo/cachego/cacheklive"
	"github.com/hotkhwan/gateway-api/internal/services/klivesvc"
	"github.com/hotkhwan/gateway-api/models/hookmod/zktmod"
	"github.com/hotkhwan/gateway-api/models/klivemod"
	"github.com/hotkhwan/gateway-api/utils/traceutil"
)

// ---------- onPlay ----------
func OnPlay(c *fiber.Ctx) error {
	ctx := c.UserContext()
	tracer := otel.Tracer("github.com/hotkhwan/gateway-api/streamzkt")
	ctx, span := tracer.Start(ctx, "webhook.OnPlay")
	defer span.End()

	if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
		ctx = traceutil.WithTraceID(ctx, sc.TraceID().String())
	}
	log := logger.FromCtx(ctx, "streamzkt", "OnPlay")

	var req zktmod.ZlmOnPlayReq
	if err := c.BodyParser(&req); err != nil {
		log.Warn().Err(err).Msg("invalid onPlay payload")
		return c.JSON(zktmod.HookResp{Code: -1, Msg: "invalid payload"})
	}

	if !allowByParams(req.Params) {
		return c.JSON(zktmod.HookResp{Code: -1, Msg: "unauthorized"})
	}

	stream := strings.TrimSpace(req.Stream)
	if stream != "" {
		ttlSec := klivesvc.GetEffectiveSessionTTL(ctx)
		if ttlSec <= 0 {
			ttlSec = 15
		}
		ttl := time.Duration(ttlSec) * time.Second

		// ✅ merge hook ip เข้า clientInfo(by stream)
		upsertClientInfoByStream(ctx, stream, strings.TrimSpace(req.Ip), ttl)
	}

	out := map[string]any{
		"event":         "media.hook.on_play",
		"id":            req.Stream,
		"schema":        req.Schema,
		"vhost":         req.Vhost,
		"app":           req.App,
		"stream":        req.Stream,
		"clientIp":      strings.TrimSpace(req.Ip),
		"port":          req.Port,
		"mediaServerId": req.MediaServerId,
		"params":        req.Params,
		"ts":            time.Now().UTC().Format(time.RFC3339Nano),
	}
	rawBody := c.Body() // []byte
	log.Info().
		Str("rawBody", string(rawBody)).
		Msg("onPlay raw body")
	query := c.Queries()

	log.Info().
		Interface("query", query).
		Msg("onPlay query params")
	go publishKliveEvent(traceutil.DetachWithParent(ctx), "media.hook.on_play", out)
	return c.JSON(zktmod.HookResp{Code: 0, Msg: "success"})
}

// ---------- onPublish ----------
func OnPublish(c *fiber.Ctx) error {
	ctx := c.UserContext()
	tracer := otel.Tracer("github.com/hotkhwan/gateway-api/streamzkt")
	ctx, span := tracer.Start(ctx, "webhook.OnPublish")
	defer span.End()

	if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
		ctx = traceutil.WithTraceID(ctx, sc.TraceID().String())
	}
	log := logger.FromCtx(ctx, "streamzkt", "OnPublish")

	var req zktmod.ZlmOnPublishReq
	if err := c.BodyParser(&req); err != nil {
		log.Warn().Err(err).Msg("invalid onPublish payload")
		return c.JSON(zktmod.HookResp{Code: -1, Msg: "invalid payload"})
	}

	if !allowByParams(req.Params) {
		return c.JSON(zktmod.HookResp{Code: -1, Msg: "unauthorized"})
	}

	stream := strings.TrimSpace(req.Stream)
	if stream != "" {
		ttlSec := klivesvc.GetEffectiveSessionTTL(ctx)
		if ttlSec <= 0 {
			ttlSec = 15
		}
		ttl := time.Duration(ttlSec) * time.Second

		// ✅ merge hook ip เข้า clientInfo(by stream)
		upsertClientInfoByStream(ctx, stream, strings.TrimSpace(req.Ip), ttl)
	}

	out := map[string]any{
		"event":         "media.hook.on_publish",
		"id":            req.Stream,
		"schema":        req.Schema,
		"vhost":         req.Vhost,
		"app":           req.App,
		"stream":        req.Stream,
		"clientIp":      strings.TrimSpace(req.Ip),
		"port":          req.Port,
		"mediaServerId": req.MediaServerId,
		"params":        req.Params,
		"ts":            time.Now().UTC().Format(time.RFC3339Nano),
	}

	go publishKliveEvent(traceutil.DetachWithParent(ctx), "media.hook.on_publish", out)
	return c.JSON(zktmod.HookResp{Code: 0, Msg: "success"})
}

// ---------- onStreamNoneReader ----------
var (
	mediagwOnce   = make(chan struct{}, 1)
	mediagwClient *mediagw.Client
)

func getMediaClient() *mediagw.Client {
	if mediagwClient != nil {
		return mediagwClient
	}
	select {
	case mediagwOnce <- struct{}{}:
		c := mediagw.NewFromEnv("ZKT")
		if c != nil && c.BaseURL != "" && strings.HasSuffix(c.BaseURL, "/media/index/api") {
			c.BaseURL = strings.TrimSuffix(c.BaseURL, "/media/index/api") + "/index/api"
		}
		mediagwClient = c
	default:
		time.Sleep(10 * time.Millisecond)
	}
	return mediagwClient
}

func OnStreamNoneReader(c *fiber.Ctx) error {
	ctx := c.UserContext()
	tracer := otel.Tracer("github.com/hotkhwan/gateway-api/streamzkt")
	ctx, span := tracer.Start(ctx, "webhook.OnStreamNoneReader")
	defer span.End()

	if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
		ctx = traceutil.WithTraceID(ctx, sc.TraceID().String())
	}
	log := logger.FromCtx(ctx, "streamzkt", "OnStreamNoneReader")

	var req zktmod.ZlmOnStreamNoneReaderReq
	if err := c.BodyParser(&req); err != nil {
		log.Warn().Err(err).Msg("invalid onStreamNoneReader payload")
		return c.JSON(zktmod.HookResp{Code: -1, Msg: "invalid payload"})
	}

	vhost := strings.TrimSpace(req.Vhost)
	if vhost == "" {
		vhost = "__defaultVhost__"
	}
	app := strings.TrimSpace(req.App)
	if app == "" {
		app = "live"
	}
	stream := strings.TrimSpace(req.Stream)
	if stream == "" {
		return c.JSON(zktmod.HookResp{Code: 0, Msg: "ok"})
	}

	go func(vhost, app, stream string) {
		bg := context.Background()
		client := getMediaClient()
		if client == nil || !client.Configured() {
			log.Warn().Msg("mediagw not configured, skip cleanup")
			return
		}

		time.Sleep(20 * time.Second)

		deviceID := stream
		cached, ok, err := cacheklive.GetDeviceStream(bg, deviceID)
		if err != nil {
			log.Warn().Err(err).Str("device", deviceID).Msg("redis get device stream failed")
			return
		}
		if !ok || cached == nil {
			log.Info().Str("device", deviceID).Str("stream", stream).Msg("no cache, skip")
			return
		}

		streamKey := vhost + "/" + app + "/" + stream
		got, err := cacheklive.AcquireStreamLock(bg, streamKey, 25*time.Second)
		if err != nil || !got {
			if err != nil {
				log.Warn().Err(err).Str("stream", stream).Msg("acquire lock failed")
			}
			return
		}
		defer func() { _ = cacheklive.ReleaseStreamLock(bg, streamKey) }()

		info, status, err := client.GetMediaInfo(bg, "rtsp", vhost, app, stream)
		if err != nil || status != 200 {
			_ = cacheklive.DeleteDeviceStream(bg, deviceID)
			log.Info().Str("stream", stream).Str("device", deviceID).Msg("mediaInfo failed => evict redis cache")
			return
		}
		if getIntAny(info["code"]) != 0 {
			_ = cacheklive.DeleteDeviceStream(bg, deviceID)
			log.Info().Str("stream", stream).Str("device", deviceID).Int("code", getIntAny(info["code"])).Msg("mediaInfo code!=0 => evict redis cache")
			return
		}

		readerCount := getIntAny(info["readerCount"])
		if readerCount > 0 {
			log.Info().Str("stream", stream).Int("readerCount", readerCount).Msg("readerCount>0 => skip cleanup")
			return
		}

		if _, _, err := client.DelStreamProxy(bg, vhost, app, stream); err != nil {
			_ = cacheklive.DeleteDeviceStream(bg, deviceID)
			log.Warn().Err(err).Str("stream", stream).Msg("delStreamProxy failed, evict cache anyway")
			return
		}

		_ = cacheklive.DeleteDeviceStream(bg, deviceID)
		log.Info().Str("stream", stream).Str("device", deviceID).Msg("cleanup: del proxy + evict redis cache")
	}(vhost, app, stream)

	return c.JSON(zktmod.HookResp{Code: 0, Msg: "ok"})
}

// ---------- onStreamNotFound ----------
func OnStreamNotFound(c *fiber.Ctx) error {
	ctx := c.UserContext()
	tracer := otel.Tracer("github.com/hotkhwan/gateway-api/streamzkt")
	ctx, span := tracer.Start(ctx, "webhook.OnStreamNotFound")
	defer span.End()

	if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
		ctx = traceutil.WithTraceID(ctx, sc.TraceID().String())
	}
	log := logger.FromCtx(ctx, "streamzkt", "OnStreamNotFound")

	var req zktmod.OnStreamNotFoundReq
	if err := c.BodyParser(&req); err != nil {
		log.Warn().Err(err).Str("body", string(c.Body())).Msg("invalid onStreamNotFound payload")
		return c.JSON(zktmod.HookResp{Code: 0, Msg: "ok"})
	}

	if c.Get("ZKT_SECRET") != os.Getenv("ZKT_SECRET") {
		return c.JSON(zktmod.HookResp{Code: -1, Msg: "unauthorized"})
	}

	stream := strings.TrimSpace(req.Stream)
	if stream != "" {
		_ = cacheklive.DeleteDeviceStream(context.Background(), stream)
		_ = cacheklive.DeleteKliveClientInfoByStream(context.Background(), stream) // ✅ clear clientInfo เมื่อ stream หาย
		log.Info().
			Str("stream", stream).
			Str("app", req.App).
			Str("vhost", req.Vhost).
			Str("schema", req.Schema).
			Msg("🧹 cache deleted due to on_stream_not_found")
	}
	return c.JSON(zktmod.HookResp{Code: 0, Msg: "success"})
}

// -------------------- helpers --------------------

func allowByParams(p string) bool {
	if p == "" {
		return true
	}
	_, _ = url.ParseQuery(p)
	return true
}

func getIntAny(v any) int {
	switch t := v.(type) {
	case int:
		return t
	case int32:
		return int(t)
	case int64:
		return int(t)
	case float64:
		return int(t)
	case float32:
		return int(t)
	default:
		return 0
	}
}

func publishKliveEvent(parent context.Context, event string, payload map[string]any) {
	// detach อีกชั้นก็ได้ กันคนเผลอส่ง request ctx เข้ามา
	ctx := traceutil.DetachWithParent(parent)

	// timeout เฉพาะงาน publish
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	log := logger.FromCtx(ctx, "streamzkt", "publishKliveEvent")

	topic := strings.TrimSpace(os.Getenv("KAFKA_LIVE_TOPIC"))
	if topic == "" {
		topic = "klive.player"
	}

	headers := map[string]string{"source": "zlm-hook"}
	traceutil.InjectHeaders(ctx, headers)

	if err := kafka.PublishEventTo(ctx, topic, event, payload, headers); err != nil {
		log.Warn().Err(err).Msg("send kafka failed")
	}
}

// ✅ merge hook-ip เข้า clientInfo(by stream)
func upsertClientInfoByStream(ctx context.Context, stream string, ipFromHook string, ttl time.Duration) {
	stream = strings.TrimSpace(stream)
	ipFromHook = strings.TrimSpace(ipFromHook)
	if stream == "" {
		return
	}

	exist, ok, _ := cacheklive.GetKliveClientInfoByStream(ctx, stream)
	if ok && exist != nil {
		if exist.RawHeaders == nil {
			exist.RawHeaders = map[string]string{}
		}
		if ipFromHook != "" {
			exist.RawHeaders["zlm-hook-ip"] = ipFromHook
		}
		// ถ้า IP ฝั่ง user ยังไม่รู้ ให้ fallback เป็น hook ip
		if strings.TrimSpace(exist.IP) == "" && ipFromHook != "" {
			exist.IP = ipFromHook
		}
		if ipFromHook != "" {
			found := false
			for _, x := range exist.IPChain {
				if strings.TrimSpace(x) == ipFromHook {
					found = true
					break
				}
			}
			if !found {
				exist.IPChain = append(exist.IPChain, ipFromHook)
			}
		}

		_ = cacheklive.SetKliveClientInfoByStream(ctx, stream, exist, ttl)
		return
	}

	ci := &klivemod.ClientInfo{
		IP:      ipFromHook,
		IPChain: []string{},
		RawHeaders: map[string]string{
			"zlm-hook-ip": ipFromHook,
		},
	}
	if ipFromHook != "" {
		ci.IPChain = []string{ipFromHook}
	}
	_ = cacheklive.SetKliveClientInfoByStream(ctx, stream, ci, ttl)
}

// ---------- Helper functions ----------
type mapCarrier map[string]string

func (c mapCarrier) Get(key string) string { return c[key] }
func (c mapCarrier) Set(key, val string)   { c[key] = val }
func (c mapCarrier) Keys() []string {
	out := make([]string, 0, len(c))
	for k := range c {
		out = append(out, k)
	}
	return out
}

func injectTraceToHeaders(ctx context.Context, headers map[string]string) {
	// ต้อง ensure ไว้ใน main: otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))
	otel.GetTextMapPropagator().Inject(ctx, mapCarrier(headers))
}
