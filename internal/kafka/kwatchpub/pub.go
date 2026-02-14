// internal/kafka/kwatchpub/pub.go
package kwatchpub

import (
	"context"
	"fmt"
	"klynx/internal/kafka"
	"klynx/utils"
	"os"
	"strconv"
	"time"
)

var (
	topicWatchlist = getenv("KAFKA_KWATCH_WATCHLIST_SYNC_TOPIC", "kwatch.watchlist.sync")
)

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

/* ---------- เดิม (คงไว้ใช้จุดอื่นได้) ---------- */
type UpsertEvent struct {
	Event     string    `json:"event"` // "watchlist.crimes.upsert"
	ID        string    `json:"id"`
	Rev       int64     `json:"rev"`
	Source    string    `json:"source"`
	PersonKey string    `json:"personKey"`
	RunID     string    `json:"runId"`
	At        time.Time `json:"at"`
}

func PublishUpsert(ctx context.Context, source, personKey, runID string) {
	ev := UpsertEvent{
		Event:     "watchlist.crimes.upsert",
		ID:        personKey,
		Rev:       time.Now().Unix(),
		Source:    source,
		PersonKey: personKey,
		RunID:     runID,
		At:        time.Now().UTC(),
	}
	_ = kafka.PublishEventTo(ctx, topicWatchlist, personKey, ev, map[string]string{"event": ev.Event})
}

/* ---------- ใหม่: batch upsert ---------- */
type UpsertBatchEvent struct {
	Event  string    `json:"event"` // "watchlist.crimes.upsert.batch"
	ID     string    `json:"id"`    // runId
	Rev    int64     `json:"rev"`
	Source string    `json:"source"` // "crimes"
	RunID  string    `json:"runId"`
	Seq    int       `json:"seq"`   // ลำดับแบตช์ (1-based)
	Total  int       `json:"total"` // จำนวนแบตช์ทั้งหมดใน run เดียวกันนี้ (best-effort ต่อ flush)
	Keys   []string  `json:"keys"`
	Count  int       `json:"count"`
	At     time.Time `json:"at"`
}

func PublishUpsertBatches(ctx context.Context, source string, runID string, keys []string) {
	if len(keys) == 0 {
		return
	}

	// limit ต่อข้อความ ปรับได้ด้วย env; ค่าปลอดภัยสำหรับ default max.message.bytes
	perMsg := utils.GetenvInt("KWATCH_BATCHSIZE", 2000)

	total := (len(keys) + perMsg - 1) / perMsg
	seq := 0
	for start := 0; start < len(keys); start += perMsg {
		end := start + perMsg
		if end > len(keys) {
			end = len(keys)
		}
		seq++

		ev := UpsertBatchEvent{
			Event:  "watchlist.crimes.upsert.batch",
			ID:     runID,
			Rev:    time.Now().Unix(),
			Source: source,
			RunID:  runID,
			Seq:    seq,
			Total:  total,
			Keys:   keys[start:end],
			Count:  end - start,
			At:     time.Now().UTC(),
		}

		// ใช้ key เป็น runId:seq กระจายพาร์ติชัน & idempotent ต่อ batch
		key := fmt.Sprintf("%s:%d", runID, seq)
		_ = kafka.PublishEventTo(ctx, topicWatchlist, key, ev, map[string]string{
			"event": ev.Event,
			"seq":   strconv.Itoa(ev.Seq),
			"total": strconv.Itoa(ev.Total),
		})
	}
}

/* ---------- delete (เดิม) ---------- */
type IBOCRef struct {
	ID     string `json:"id,omitempty"`
	FaceID string `json:"faceId,omitempty"`
}

type DeletePayload struct {
	PersonKey      string `json:"personKey"`
	IDCard         string `json:"idcard,omitempty"`
	PhotoOriginKey string `json:"photoOriginKey,omitempty"`
	PhotoFaceKey   string `json:"photoFaceKey,omitempty"`
	PhotoKeyLegacy string `json:"photoKey,omitempty"`

	IBOC    IBOCRef `json:"iboc,omitempty"`
	IBOCDev IBOCRef `json:"ibocdev,omitempty"`

	// ⬇️ เพิ่มสองฟิลด์นี้
	External map[string]any `json:"external,omitempty"`
	Rev      int64          `json:"rev,omitempty"`

	RunID string    `json:"runId"`
	At    time.Time `json:"at"`
}
type DeleteEvent struct {
	Event  string `json:"event"` // "watchlist.crimes.delete"
	ID     string `json:"id"`
	Rev    int64  `json:"rev"`
	Source string `json:"source"`
	DeletePayload
}

func PublishDelete(ctx context.Context, source string, p DeletePayload) {
	ev := DeleteEvent{
		Event:         "watchlist.crimes.delete",
		ID:            p.PersonKey,
		Rev:           time.Now().Unix(),
		Source:        source,
		DeletePayload: p,
	}
	_ = kafka.PublishEventTo(ctx, topicWatchlist, p.PersonKey, ev, map[string]string{
		"event": ev.Event,
	})
}
