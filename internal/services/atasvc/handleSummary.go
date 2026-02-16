// internal/services/atasvc/handleSummary.go
package atasvc

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/hotkhwan/gateway-api/internal/mqtt/inframsg"
	"github.com/hotkhwan/gateway-api/internal/services/dashsvc"
)

func NewAtaEventsPublisher(topic string) *Publisher {
	// debounce 3s (ตามที่ตกลง)
	debounce := 3 * time.Second

	// throttle: EVENTS_PER_SEC=5 => minInterval=200ms
	min1 := 1
	max100 := 100
	eps := EnvInt("EVENTS_PER_SEC", 5, IntOpt{Min: &min1, Max: &max100})
	minInterval := time.Second / time.Duration(eps)

	// publish func
	pubFn := func() error {
		loc, err := time.LoadLocation("Asia/Bangkok")
		if err != nil {
			loc = time.Local
		}
		now := time.Now().In(loc)
		start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
		end := now

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		details, err := dashsvc.BuildAtaDashboard(ctx, start, end)
		if err != nil {
			return err
		}

		// ✅ ตัด blacklist ออก (ตาม requirement)
		// สร้าง map เอง จะได้ไม่ต้องแก้ struct tag
		detailsMap := map[string]any{
			"event":              details.Event,
			"camera":             details.Camera,
			"kcontrol":           details.Kcontrol,
			"peopleCountingArea": details.PeopleCounting,
			"notification":       details.Notification,
			// ❌ no "blacklist"
		}

		payload := map[string]any{
			"event": "events",
			"time":  now.Format(time.RFC3339),
			"range": map[string]any{
				"start": start.Format(time.RFC3339),
				"end":   end.Format(time.RFC3339),
			},
			"details": detailsMap,
		}

		b, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		return inframsg.PublishRaw(strings.TrimSpace(topic), 0, false, b)
	}

	return New(Options{
		Debounce:    debounce,
		MinInterval: minInterval,
		Publish:     pubFn,
	})
}
