// internal/kafka/iwowncons/event.go
package iwowncons

import (
	"context"
	"encoding/json"

	"github.com/hotkhwan/gateway-api/internal/logger"
	"github.com/hotkhwan/gateway-api/internal/services/iwownsvc"
	"github.com/hotkhwan/gateway-api/models/iwownmod"
)

func HandleIwownEvent(ctx context.Context, key string, value []byte, headers map[string]string) error {
	log := logger.FromCtx(ctx, "iwowncons", "event")

	eventType := headers["event"]

	switch eventType {
	case "kwatch4g.pb":
		var frame iwownmod.IwownBinaryFrameEvent
		if err := json.Unmarshal(value, &frame); err != nil {
			log.Error().Err(err).Msg("❌ unmarshal pb frame failed")
			return err
		}
		return iwownsvc.HandlePBFrame(ctx, frame)

	case "kwatch4g.alarm":
		var frame iwownmod.IwownBinaryFrameEvent
		if err := json.Unmarshal(value, &frame); err != nil {
			log.Error().Err(err).Msg("❌ unmarshal alarm frame failed")
			return err
		}
		return iwownsvc.HandleAlarmFrame(ctx, frame)

	case "kwatch4g.calllog":
		var req iwownmod.IwownDeviceCallLogs
		if err := json.Unmarshal(value, &req); err != nil {
			log.Error().Err(err).Msg("❌ unmarshal calllog failed")
			return err
		}
		return iwownsvc.HandleCallLog(ctx, req)

	case "kwatch4g.deviceinfo":
		var req iwownmod.IwownDeviceInfo
		if err := json.Unmarshal(value, &req); err != nil {
			log.Error().Err(err).Msg("❌ unmarshal deviceinfo failed")
			return err
		}
		return iwownsvc.HandleDeviceInfo(ctx, req)

	case "kwatch4g.status":
		var req iwownmod.IwownDeviceStatus
		if err := json.Unmarshal(value, &req); err != nil {
			log.Error().Err(err).Msg("❌ unmarshal status failed")
			return err
		}
		return iwownsvc.HandleStatus(ctx, req)

	default:
		log.Warn().Str("event", eventType).Msg("⚠️ unknown iwown event type")
		return nil
	}
}
