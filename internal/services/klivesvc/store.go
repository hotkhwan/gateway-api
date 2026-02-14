// internal/services/klivesvc/store.go
package klivesvc

import (
	"context"
	"net/url"
	"strings"
	"time"

	"klynx/internal/repo/cachego/cacheklive"
	"klynx/internal/repo/stomongo"
	"klynx/models/klivemod"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

const collName = "klive_events"

// ---------- Normalizers / Parsers ----------
func normalizeEvent(ev string) string {
	switch strings.ToLower(strings.TrimSpace(ev)) {
	case "media.hook.on_play":
		return "klive.play.started"
	case "media.hook.on_publish":
		return "klive.publish.started"
	case "media.hook.on_stream_none_reader":
		return "klive.play.stopped"
	default:
		return strings.ToLower(strings.TrimSpace(ev))
	}
}

func parseSessionFromParams(params string, fallback string) string {
	if params == "" {
		return fallback
	}
	if uq, err := url.ParseQuery(params); err == nil {
		if s := uq.Get("session"); s != "" {
			return s
		}
	}
	return fallback
}

func parseTimeOrNow(ts string) time.Time {
	if ts == "" {
		return time.Now()
	}
	if t, err := time.Parse(time.RFC3339Nano, ts); err == nil {
		return t
	}
	if t, err := time.Parse(time.RFC3339, ts); err == nil {
		return t
	}
	return time.Now()
}

// ---------- Builders ----------
func buildDocFromEvent(msg klivemod.LiveEvent, ci *klivemod.ClientInfo) klivemod.KliveEventDoc {
	ev := normalizeEvent(msg.Event)
	// session := parseSessionFromParams(msg.Params, msg.Raw.ID)
	hookIndex := msg.Raw.HookIndex

	// raw := map[string]interface{}{
	// 	"clientIP":      msg.ClientIP,
	// 	"port":          msg.Port,
	// 	"vhost":         msg.VHost,
	// 	"event":         msg.Event,
	// 	"id":            msg.ID,
	// 	"params":        msg.Params,
	// 	"stream":        msg.Stream,
	// 	"mediaServerId": msg.MediaServer,
	// 	"app":           msg.App,
	// 	"schema":        msg.Schema,
	// 	"ts":            msg.TS,
	// 	"zlm":           msg.Raw,
	// }
	// if ci != nil {
	// 	raw["clientInfo"] = ci
	// }

	return klivemod.KliveEventDoc{
		KliveID: strings.TrimSpace(msg.ID),
		Event:   ev,
		App:     msg.App,
		// ClientIP:    msg.ClientIP,
		ClientInfo: ci,
		Schema:     msg.Schema,
		Stream:     msg.Stream,
		VHost:      msg.VHost,
		Params:     msg.Params,
		// Session:     session, // เก็บได้ แต่ไม่ใช้ kill แล้ว
		MediaServer: msg.MediaServer,
		Port:        msg.Port,
		HookIndex:   hookIndex,
		Protocol:    msg.Raw.Protocol,
		Timestamp:   parseTimeOrNow(msg.TS),
		CreatedAt:   time.Now(),
		Rev:         msg.Rev,
		// Raw:         raw,
	}
}

// ---------- Store ----------
func InsertEvent(ctx context.Context, doc klivemod.KliveEventDoc) error {
	_, err := stomongo.InsertOne(ctx, collName, doc)
	return err
}

func SaveKliveEvent(ctx context.Context, msg klivemod.LiveEvent) error {
	tr := otel.Tracer("klynx.klive")
	ctx, span := tr.Start(ctx, "klivesvc.SaveKliveEvent")
	defer span.End()

	if strings.TrimSpace(msg.ID) == "" {
		span.SetAttributes(attribute.String("skip.reason", "empty_id"))
		return nil
	}

	// 1) resolve clientInfo (stream only)
	ci := resolveClientInfo(ctx, msg)

	// 2) resolve ip (prefer clientInfo.ip -> kafka clientIp -> raw ip)
	msg.ClientIP = resolveClientIP(ctx, msg, ci)

	span.SetAttributes(
		attribute.String("klive.id", msg.ID),
		attribute.String("klive.event.raw", msg.Event),
		attribute.String("klive.event.norm", normalizeEvent(msg.Event)),
		attribute.String("klive.stream", msg.Stream),
		attribute.String("klive.session", parseSessionFromParams(msg.Params, msg.Raw.ID)),
		attribute.String("klive.clientIp", msg.ClientIP),
	)
	if ci != nil {
		span.SetAttributes(
			attribute.String("klive.ua", ci.UA),
			attribute.String("klive.browser", ci.Browser),
			attribute.String("klive.os", ci.OS),
			attribute.String("klive.device", ci.Device),
			attribute.String("klive.lang", ci.Lang),
			attribute.String("klive.country", ci.Country),
		)
	}

	doc := buildDocFromEvent(msg, ci)
	return InsertEvent(ctx, doc)
}

// ----------- Helpers -----------
func resolveClientInfo(ctx context.Context, msg klivemod.LiveEvent) *klivemod.ClientInfo {
	if s := strings.TrimSpace(msg.Stream); s != "" {
		if info, ok, _ := cacheklive.GetKliveClientInfoByStream(ctx, s); ok && info != nil {
			return info
		}
	}
	return nil
}

func resolveClientIP(_ context.Context, msg klivemod.LiveEvent, ci *klivemod.ClientInfo) string {
	if ci != nil {
		if ip := strings.TrimSpace(ci.IP); ip != "" {
			return ip
		}
	}
	if s := strings.TrimSpace(msg.ClientIP); s != "" {
		return s
	}
	if s := strings.TrimSpace(msg.Raw.IP); s != "" {
		return s
	}
	return ""
}
