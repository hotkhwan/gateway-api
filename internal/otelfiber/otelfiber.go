// internal/otelfiber/otelfiber.go
// Minimal OpenTelemetry tracing middleware for Fiber v3.
// This is an internal adapter because gofiber/contrib/otelfiber has no v3 release.
package otelfiber

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gofiber/fiber/v3"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
	oteltrace "go.opentelemetry.io/otel/trace"
)

const (
	instrumentationName = "github.com/hotkhwan/gateway-api/internal/otelfiber"

	metricNameHttpServerDuration       = "http.server.duration"
	metricNameHttpServerRequestSize    = "http.server.request.size"
	metricNameHttpServerResponseSize   = "http.server.response.size"
	metricNameHttpServerActiveRequests = "http.server.active_requests"

	unitDimensionless = "1"
	unitBytes         = "By"
	unitMilliseconds  = "ms"
)

// Middleware returns a Fiber v3 handler that instruments HTTP requests with OpenTelemetry traces.
func Middleware() fiber.Handler {
	tracerProvider := otel.GetTracerProvider()
	tracer := tracerProvider.Tracer(instrumentationName)
	propagators := otel.GetTextMapPropagator()

	meterProvider := otel.GetMeterProvider()
	meter := meterProvider.Meter(instrumentationName)

	httpServerDuration, _ := meter.Float64Histogram(metricNameHttpServerDuration,
		metric.WithUnit(unitMilliseconds),
		metric.WithDescription("measures the duration inbound HTTP requests"))
	httpServerRequestSize, _ := meter.Int64Histogram(metricNameHttpServerRequestSize,
		metric.WithUnit(unitBytes),
		metric.WithDescription("measures the size of HTTP request messages"))
	httpServerResponseSize, _ := meter.Int64Histogram(metricNameHttpServerResponseSize,
		metric.WithUnit(unitBytes),
		metric.WithDescription("measures the size of HTTP response messages"))
	httpServerActiveRequests, _ := meter.Int64UpDownCounter(metricNameHttpServerActiveRequests,
		metric.WithUnit(unitDimensionless),
		metric.WithDescription("measures the number of concurrent HTTP requests in-flight"))

	return func(c fiber.Ctx) error {
		start := time.Now()

		// Extract trace context from incoming request headers.
		reqHeader := make(http.Header)
		c.Request().Header.VisitAll(func(k, v []byte) {
			reqHeader.Add(string(k), string(v))
		})
		parentCtx := propagators.Extract(context.Background(), propagation.HeaderCarrier(reqHeader))

		// Build initial span attributes from the request.
		method := string(c.Method())
		scheme := c.Protocol()
		host := c.Hostname()
		path := string(c.Request().URI().Path())
		query := c.Request().URI().QueryArgs().String()

		requestMetricsAttrs := []attribute.KeyValue{
			semconv.URLScheme(scheme),
			semconv.ServerAddress(host),
			semconv.HTTPRequestMethodKey.String(method),
		}
		httpServerActiveRequests.Add(parentCtx, 1, metric.WithAttributes(requestMetricsAttrs...))

		spanAttrs := append([]attribute.KeyValue{
			semconv.HTTPRequestMethodKey.String(method),
			semconv.URLScheme(scheme),
			semconv.URLPath(path),
			semconv.URLQuery(query),
			semconv.URLFull(c.OriginalURL()),
			semconv.UserAgentOriginal(string(c.Request().Header.UserAgent())),
			semconv.ServerAddress(host),
			semconv.NetTransportTCP,
			semconv.HTTPRequestBodySize(c.Request().Header.ContentLength()),
		}, requestMetricsAttrs[2:]...)

		// Start a server span.
		spanName := path
		ctx, span := tracer.Start(parentCtx, spanName,
			oteltrace.WithAttributes(spanAttrs...),
			oteltrace.WithSpanKind(oteltrace.SpanKindServer),
		)
		defer span.End()

		// Attach the span context to the fiber Ctx so downstream handlers can use it.
		c.SetContext(ctx)

		requestSize := int64(len(c.Request().Body()))
		responseMetricAttrs := make([]attribute.KeyValue, len(requestMetricsAttrs))
		copy(responseMetricAttrs, requestMetricsAttrs)

		// Serve the request.
		if err := c.Next(); err != nil {
			span.RecordError(err)
			_ = c.App().Config().ErrorHandler(c, err)
		}

		// Post-response attributes.
		statusCode := c.Response().StatusCode()
		routePath := c.Route().Path
		responseSize := int64(len(c.Response().Body()))

		responseAttrs := []attribute.KeyValue{
			semconv.HTTPResponseStatusCode(statusCode),
			semconv.HTTPRouteKey.String(routePath),
		}
		span.SetAttributes(append(responseAttrs, semconv.HTTPResponseBodySizeKey.Int64(responseSize))...)
		span.SetName(routePath)

		// Set span status based on HTTP status code.
		spanStatus, spanMsg := spanStatusFromHTTPStatus(statusCode, oteltrace.SpanKindServer)
		span.SetStatus(spanStatus, spanMsg)

		// Propagate tracing context in response headers.
		tracingHeaders := make(propagation.HeaderCarrier)
		propagators.Inject(ctx, tracingHeaders)
		for _, k := range tracingHeaders.Keys() {
			c.Set(k, tracingHeaders.Get(k))
		}

		// Record metrics.
		allMetricAttrs := append(responseMetricAttrs, responseAttrs...)
		httpServerActiveRequests.Add(parentCtx, -1, metric.WithAttributes(requestMetricsAttrs...))
		httpServerDuration.Record(parentCtx, float64(time.Since(start).Microseconds())/1000, metric.WithAttributes(allMetricAttrs...))
		httpServerRequestSize.Record(parentCtx, requestSize, metric.WithAttributes(allMetricAttrs...))
		httpServerResponseSize.Record(parentCtx, responseSize, metric.WithAttributes(allMetricAttrs...))

		return nil
	}
}

// spanStatusFromHTTPStatus returns an OpenTelemetry span status from an HTTP status code.
func spanStatusFromHTTPStatus(code int, spanKind oteltrace.SpanKind) (codes.Code, string) {
	if http.StatusText(code) == "" {
		return codes.Error, fmt.Sprintf("Invalid HTTP status code %d", code)
	}
	if (code >= http.StatusContinue && code < http.StatusBadRequest) ||
		(spanKind == oteltrace.SpanKindServer && code >= http.StatusBadRequest && code <= 499) {
		return codes.Unset, ""
	}
	return codes.Error, ""
}
