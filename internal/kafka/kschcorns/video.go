// internal/kafka/kschcons/video.go
package kschcorns

import (
	"fmt"
	"os"
	"strings"

	"klynx/internal/kafka"
	"klynx/internal/logger"
	"klynx/models/kschmod"
)

func StartKsearchConsumer(broker, topic string) {
	log := logger.WithMeta("kschsvc", "StartKsearchConsumer").With().Str("topic", topic).Logger()
	log.Info().Msg("🟢 Starting Kafka ksearch Consumer")

	// groupID := "KAFKA_GROUP.kwatch.watchlist"
	baseGroup := strings.TrimSpace(os.Getenv("KAFKA_GROUP"))
	var groupID string
	if baseGroup != "" {
		// เช่น klynx.consumer-feature.kwatch.watchlist
		groupID = fmt.Sprintf("%s.%s", baseGroup, topic)
	} else {
		groupID = "ksearch.video"
	}

	kafka.StartConsumer(broker, topic, groupID, func(msg kschmod.VideoEvent) error {
		ev := strings.ToLower(strings.TrimSpace(msg.Event))
		st := strings.ToLower(strings.TrimSpace(msg.State))

		if strings.TrimSpace(msg.ID) == "" {
			log.Warn().Str("event", ev).Msg("⚠️ skip: empty id")
			return nil
		}

		log.Info().
			Str("ksearchID", msg.ID).
			Str("event", ev).
			Str("state", st).
			Int64("rev", msg.Rev).
			Msg("📥 Received ksearchID")

		switch ev {
		// case "video.deleted":
		// 	return kwatsvc.HandleVideoDelete(msg, gw)

		// case "video.created":
		// 	return kwatsvc.HandleVideoCreate(msg, gw)

		// case "video.updated", "video.updated.withface":
		// 	return kwatsvc.HandleVideoUpdate(msg, gw)

		default:
			log.Info().Str("event", ev).Msg("ℹ️ skip: unknown event")
			return nil
		}
	})
}
