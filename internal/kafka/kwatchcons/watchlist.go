// internal/kafka/kwatchcons/watchlist.go
package kwatchcons

import (
	"context"
	"fmt"
	"os"
	"strings"

	"klynx/internal/kafka"
	"klynx/internal/logger"
	"klynx/internal/services/kwatsvc"
	"klynx/models/kwatmod"
	"klynx/utils/traceutil"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

func StartWatchlistConsumer(broker, topic string) {
	ctx := context.Background()
	tracer := otel.Tracer("klynx/kwatchcons")
	log := logger.FromCtx(ctx, "kwatchcons", "StartWatchlistConsumer")
	log.Info().Str("topic", topic).Msg("consumer_boot")

	// ✅ เตรียม Gateways ตามที่ service handlers ต้องการ
	gw := kwatsvc.NewGatewaysFromEnv()
	gw.LogWiring()

	// groupID := "KAFKA_GROUP.kwatch.watchlist"
	baseGroup := strings.TrimSpace(os.Getenv("KAFKA_GROUP"))
	var groupID string
	if baseGroup != "" {
		// เช่น klynx.consumer-feature.kwatch.watchlist
		groupID = fmt.Sprintf("%s.%s", baseGroup, topic)
	} else {
		groupID = "kwatch.watchlist"
	}

	kafka.StartConsumerWithHeaders(broker, topic, groupID,
		func(m kwatmod.WatchlistEvent, headers map[string]string) error {
			base := context.Background()

			// ✅ Extract W3C trace context (traceparent/tracestate) from Kafka headers
			ctx := traceutil.ExtractHeaders(base, headers)

			// fallback: bind custom traceID for logger (optional)
			if strings.TrimSpace(traceutil.TraceIDFromCtx(ctx)) == "" && strings.TrimSpace(m.TraceID) != "" {
				ctx = traceutil.WithTraceID(ctx, m.TraceID)
			}

			// Start consumer span
			ctx, span := tracer.Start(ctx, "Kwatch.WatchlistEvent", trace.WithSpanKind(trace.SpanKindConsumer))
			span.SetAttributes(
				attribute.String("messaging.system", "kafka"),
				attribute.String("messaging.destination", topic),
				attribute.String("kwatch.event", strings.ToLower(strings.TrimSpace(m.Event))),
			)
			defer span.End()

			lg := logger.FromCtx(ctx, "kwatchcons", "kwatchWatchlistConsumer").With().
				Str("topic", topic).
				Str("event", strings.ToLower(strings.TrimSpace(m.Event))).
				Str("state", strings.ToLower(strings.TrimSpace(m.State))).
				Str("watchlistID", strings.TrimSpace(m.ID)).
				Int64("rev", m.Rev).
				Logger()

			// ข้ามกรณีไม่มี ID (ยกเว้น task แบบ batch)
			if strings.TrimSpace(m.ID) == "" && strings.ToLower(strings.TrimSpace(m.Event)) != "watchlist.iboc.sync.task" {
				lg.Warn().Msg("skip_empty_id")
				return nil
			}
			lg.Debug().Msg("consumer_received")

			// internal/kafka/kwatchcons/watchlist.go
			ev := strings.ToLower(strings.TrimSpace(m.Event))
			switch ev {
			case "watchlist.deleted", "watchlist.crimes.delete":
				return kwatsvc.HandleWatchlistDelete(ctx, m, gw)
			case "watchlist.created":
				return kwatsvc.HandleWatchlistCreate(ctx, m, gw)
			case "watchlist.updated",
				"watchlist.updated.withface",
				"watchlist.updated.typeto2",
				"watchlist.updated.typeto1",
				"watchlist.updated.meta":
				return kwatsvc.HandleWatchlistUpdate(ctx, m, gw)
			case "watchlist.iboc.sync.task":
				return kwatsvc.HandleSyncIbocTask(ctx, m)
			case "watchlist.ibocdev.sync.task":
				return kwatsvc.HandleSyncIbocDevTask(ctx, m)
			default:
				lg.Info().Msg("skip_unknown_event")
				return nil
			}
		},
	)
}
