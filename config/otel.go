// config/otel.go
package config

import (
	"context"
	"os"
	"strings"
	"sync"
	"time"

	"klynx/internal/logger"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	sdkresource "go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

var (
	telemetryShutdown func(context.Context) error
	telemetryOnceInit sync.Once
	telemetryOnceStop sync.Once
)

func InitTelemetry(ctx context.Context) {
	telemetryOnceInit.Do(func() {
		log := logger.Boot("otel", "config-InitTelemetry")

		endpoint := getenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
		if endpoint == "" {
			// ถ้าอยากแยก endpoint สำหรับ traces โดยเฉพาะ ให้ลองอ่าน OTEL_EXPORTER_OTLP_TRACES_ENDPOINT ด้วยก็ได้
			endpoint = os.Getenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT")
		}
		svcName := getenv("OTEL_SERVICE_NAME", "klynx-api")
		svcVer := getenv("OTEL_SERVICE_VERSION", "dev")
		ratio := getRatio()

		var (
			exp        *otlptrace.Exporter
			expEnabled bool
			err        error
		)

		if endpoint == "" {
			log.Warn().Msg("OTEL_EXPORTER_OTLP_ENDPOINT empty → tracing local-only (no export)")
		} else {
			// เลือก HTTP ถ้าเป็น :4318 (หรือระบุผ่าน OTEL_EXPORTER_OTLP_TRACES_PROTOCOL=http/protobuf)
			proto := strings.ToLower(os.Getenv("OTEL_EXPORTER_OTLP_TRACES_PROTOCOL"))
			useHTTP := strings.HasSuffix(endpoint, ":4318") || proto == "http/protobuf"

			if useHTTP {
				exp, err = otlptracehttp.New(ctx,
					otlptracehttp.WithEndpoint(endpoint),
					otlptracehttp.WithInsecure(), // Jaeger collector:4318 เปิด http (ไม่ TLS)
					// otlptracehttp.WithURLPath("/v1/traces"), // ดีฟอลต์ก็ /v1/traces ไม่ต้องใส่ก็ได้
				)
				log.Info().Str("endpoint", endpoint).Msg("Using OTLP/HTTP exporter")
			} else {
				exp, err = otlptracegrpc.New(ctx,
					otlptracegrpc.WithEndpoint(endpoint),
					otlptracegrpc.WithInsecure(), // Jaeger collector:4317 เปิด gRPC h2c (ไม่ TLS)
				)
				log.Info().Str("endpoint", endpoint).Msg("Using OTLP/gRPC exporter")
			}

			if err != nil {
				log.Error().Err(err).Str("endpoint", endpoint).Msg("❌ create OTLP exporter failed")
			} else {
				expEnabled = true
			}
		}

		res, _ := sdkresource.Merge(
			sdkresource.Default(),
			sdkresource.NewWithAttributes(
				semconv.SchemaURL,
				semconv.ServiceName(svcName),
				semconv.ServiceVersion(svcVer),
			),
		)

		tpOpts := []sdktrace.TracerProviderOption{
			sdktrace.WithResource(res),
			sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(ratio))),
		}
		if expEnabled {
			tpOpts = append(tpOpts, sdktrace.WithBatcher(exp))
		}
		tp := sdktrace.NewTracerProvider(tpOpts...)
		otel.SetTracerProvider(tp)
		otel.SetTextMapPropagator(
			propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}),
		)
		telemetryShutdown = tp.Shutdown

		log.Info().
			Str("service", svcName).
			Str("version", svcVer).
			Str("endpoint", endpoint).
			Float64("sampler_ratio", ratio).
			Bool("exporter_enabled", expEnabled).
			Msg("✅ OpenTelemetry initialized")
	})
}

func DisconnectTelemetry() {
	telemetryOnceStop.Do(func() {
		log := logger.Boot("otel", "config-DisconnectTelemetry")
		if telemetryShutdown == nil {
			log.Warn().Msg("telemetry not initialized or already shutdown")
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := telemetryShutdown(ctx); err != nil {
			log.Warn().Err(err).Msg("⚠️ telemetry shutdown with error")
		} else {
			log.Info().Msg("🧹 telemetry shutdown complete")
		}
	})
}

func getRatio() float64 {
	if v := os.Getenv("OTEL_TRACES_SAMPLER_ARG"); v == "" || v == "1.0" {
		return 1.0
	}
	return 0.1
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
