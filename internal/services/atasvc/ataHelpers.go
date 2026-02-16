// internal/services/atasvc/ataHelpers.go
package atasvc

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hotkhwan/gateway-api/utils"

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
