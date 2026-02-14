// internal/services/atasvc/handlemessage.go
package atasvc

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"klynx/internal/logger"
	"klynx/internal/mqtt/inframsg"
	"klynx/models/aimodel"
	"klynx/utils/traceutil"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"golang.org/x/sync/singleflight"
)

var (
	ataEventsPub     *Publisher
	ataEventsPubOnce sync.Once

	// ✅ ป้องกัน Persist + fetch video ซ้ำ
	ataEventGroup singleflight.Group
)

func getAtaEventsPub(topic string) *Publisher {
	ataEventsPubOnce.Do(func() {
		ataEventsPub = NewAtaEventsPublisher(strings.TrimSpace(topic))
	})
	return ataEventsPub
}

// ------------------------------------------------------------
// HandleEvent
// ------------------------------------------------------------
func HandleEvent(parent context.Context, msg aimodel.PusherEvent) error {
	ctx, end, log := traceutil.StartLite(
		parent,
		"klynx/atasvc",
		"atasvc.HandleEvent",
		"atasvc", "HandleEvent",
	)
	defer end()

	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	topic := strings.TrimSpace(os.Getenv("ANALYTIC_TOPIC_ATA"))
	if topic == "" {
		topic = "ata.events"
	}

	pub := getAtaEventsPub(topic)

	// ✅ Persist ด้วย single-flight
	doc, err := persistATAOnce(ctx, msg)
	if err != nil {
		log.Error().Err(err).Msg("❌ Persist ATA event failed")
		return err
	}
	if doc == nil {
		return nil
	}

	if pub != nil {
		pub.MarkDirty()
	}

	raw, _ := doc["raw"].(bson.M)
	if raw == nil {
		return nil
	}

	// ------------------------------------------------
	// blacklist detection
	// ------------------------------------------------
	shouldNotify := false
	if det, ok := raw["eventAttributeDetails"].(map[string]any); ok {
		if lt, ok := det["listType"].(string); ok {
			v := strings.ToLower(strings.TrimSpace(lt))
			if v == "blacklist" || v == "redlist" || v == "red list" {
				shouldNotify = true
			}
		}
	}
	if !shouldNotify {
		return nil
	}

	// ------------------------------------------------
	// build payload
	// ------------------------------------------------
	var firstImage string
	if v, ok := raw["pictureUrl"]; ok {
		switch vv := v.(type) {
		case []string:
			if len(vv) > 0 {
				firstImage = vv[0]
			}
		case []any:
			if len(vv) > 0 {
				if s, ok := vv[0].(string); ok {
					firstImage = s
				}
			}
		}
	}

	proxy := strings.TrimSpace(os.Getenv("FILES_PROXY_PATH"))
	if proxy != "" && firstImage != "" {
		if !strings.HasPrefix(proxy, "/") {
			proxy = "/" + proxy
		}
		firstImage = strings.TrimRight(proxy, "/") + firstImage
	}

	mongoID := ""
	switch v := doc["_id"].(type) {
	case primitive.ObjectID:
		mongoID = v.Hex()
	default:
		mongoID = fmt.Sprint(v)
	}

	out := bson.M{
		"id":      mongoID,
		"event":   "blacklist",
		"channel": msg.Channel,
		"timeMs":  msg.TimeMs,
	}

	for k, v := range raw {
		if k == "_id" || k == "event" || k == "channel" || k == "timeMs" || k == "pictureUrl" {
			continue
		}
		out[k] = v
	}
	if firstImage != "" {
		out["imageUrl"] = firstImage
	}

	b, _ := json.Marshal(out)
	return inframsg.PublishRaw(topic, 0, false, b)
}

// ------------------------------------------------------------
// single-flight persist
// ------------------------------------------------------------
func persistATAOnce(ctx context.Context, msg aimodel.PusherEvent) (bson.M, error) {
	log := logger.FromCtx(ctx, "atasvc", "persistATAOnce")

	key := ataEventKey(msg)
	v, err, shared := ataEventGroup.Do(key, func() (any, error) {
		ctx2, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		return PersistATAEvent(ctx2, msg)
	})

	if err != nil {
		log.Warn().
			Err(err).
			Bool("shared", shared).
			Str("key", key).
			Msg("⚠️ PersistATAEvent failed (best-effort)")
		return nil, nil
	}

	if doc, ok := v.(bson.M); ok {
		return doc, nil
	}
	return nil, nil
}

// ------------------------------------------------------------
// event key (based on real model)
// ------------------------------------------------------------
func ataEventKey(msg aimodel.PusherEvent) string {
	h := sha1.Sum(msg.Data)
	dataHash := hex.EncodeToString(h[:8]) // สั้นพอ + ชนยาก

	return fmt.Sprintf(
		"%s|%s|%d|%s",
		strings.TrimSpace(msg.Event),
		strings.TrimSpace(msg.Channel),
		msg.TimeMs,
		dataHash,
	)
}
