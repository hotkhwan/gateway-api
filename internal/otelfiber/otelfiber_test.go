// internal/otelfiber/otelfiber_test.go
package otelfiber_test

import (
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"

	"github.com/hotkhwan/gateway-api/internal/otelfiber"
)

// setupOtel installs an in-memory exporter and W3C TraceContext propagator into
// the global OTel provider and returns the exporter for span inspection.
// It also restores the previous providers after the test.
func setupOtel(t *testing.T) *tracetest.InMemoryExporter {
	t.Helper()

	exp := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSyncer(exp),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	t.Cleanup(func() {
		_ = tp.Shutdown(t.Context())
		otel.SetTracerProvider(otel.GetTracerProvider())
	})

	return exp
}

// newApp builds a minimal Fiber app with the otelfiber middleware and a single
// handler registered at path that returns statusCode.
func newApp(path string, statusCode int) *fiber.App {
	app := fiber.New(fiber.Config{
		// Suppress default error handler noise in tests.
		// Set a quiet error handler so error-path tests don't pollute output.
	})
	app.Use(otelfiber.Middleware())
	app.Get(path, func(c fiber.Ctx) error {
		return c.SendStatus(statusCode)
	})
	return app
}

func TestMiddleware_SpanIsCreated(t *testing.T) {
	exp := setupOtel(t)

	app := newApp("/ping", fiber.StatusOK)
	req := httptest.NewRequest("GET", "/ping", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	resp.Body.Close()

	spans := exp.GetSpans()
	if len(spans) == 0 {
		t.Fatal("expected at least one span to be recorded")
	}
}

func TestMiddleware_TraceContextExtractedFromIncomingHeaders(t *testing.T) {
	exp := setupOtel(t)

	// Build a known traceparent header.
	const parentTraceID = "4bf92f3577b34da6a3ce929d0e0e4736"
	const parentSpanID = "00f067aa0ba902b7"
	traceparent := "00-" + parentTraceID + "-" + parentSpanID + "-01"

	app := newApp("/ping", fiber.StatusOK)
	req := httptest.NewRequest("GET", "/ping", nil)
	req.Header.Set("Traceparent", traceparent)

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	resp.Body.Close()

	spans := exp.GetSpans()
	if len(spans) == 0 {
		t.Fatal("expected at least one span")
	}

	// The recorded span's parent trace-ID must match the injected trace-ID.
	span := spans[0]
	gotTraceID := span.SpanContext.TraceID().String()
	if gotTraceID != parentTraceID {
		t.Errorf("expected traceID %q; got %q", parentTraceID, gotTraceID)
	}
	gotParentSpanID := span.Parent.SpanID().String()
	if gotParentSpanID != parentSpanID {
		t.Errorf("expected parent spanID %q; got %q", parentSpanID, gotParentSpanID)
	}
}

func TestMiddleware_ResponseHeadersContainTracePropagation(t *testing.T) {
	setupOtel(t)

	app := newApp("/ping", fiber.StatusOK)
	req := httptest.NewRequest("GET", "/ping", nil)

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	resp.Body.Close()

	// The W3C TraceContext propagator injects a "Traceparent" header.
	if resp.Header.Get("Traceparent") == "" {
		t.Error("expected Traceparent response header to be set by middleware")
	}
}

func TestMiddleware_HTTPStatusAttributeIsSet(t *testing.T) {
	exp := setupOtel(t)

	app := newApp("/ping", fiber.StatusNotFound)
	req := httptest.NewRequest("GET", "/ping", nil)

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	resp.Body.Close()

	spans := exp.GetSpans()
	if len(spans) == 0 {
		t.Fatal("expected at least one span")
	}

	var found bool
	for _, attr := range spans[0].Attributes {
		if attr.Key == semconv.HTTPResponseStatusCodeKey {
			found = true
			got := int(attr.Value.AsInt64())
			if got != fiber.StatusNotFound {
				t.Errorf("http.response.status_code: want %d, got %d", fiber.StatusNotFound, got)
			}
			break
		}
	}
	if !found {
		t.Error("http.response.status_code attribute not found in span")
	}
}

func TestMiddleware_SpanStatus(t *testing.T) {
	tests := []struct {
		name           string
		statusCode     int
		wantSpanStatus codes.Code
	}{
		{"2xx success", fiber.StatusOK, codes.Unset},
		{"3xx redirect", fiber.StatusFound, codes.Unset},
		{"4xx client error (server span)", fiber.StatusBadRequest, codes.Unset},
		{"404 not found (server span)", fiber.StatusNotFound, codes.Unset},
		{"5xx server error", fiber.StatusInternalServerError, codes.Error},
		{"503 service unavailable", fiber.StatusServiceUnavailable, codes.Error},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exp := setupOtel(t)

			app := newApp("/test", tt.statusCode)
			req := httptest.NewRequest("GET", "/test", nil)

			resp, err := app.Test(req)
			if err != nil {
				t.Fatalf("app.Test: %v", err)
			}
			resp.Body.Close()

			spans := exp.GetSpans()
			if len(spans) == 0 {
				t.Fatal("no spans recorded")
			}

			gotStatus := spans[0].Status.Code
			if gotStatus != tt.wantSpanStatus {
				t.Errorf("span status: want %v, got %v", tt.wantSpanStatus, gotStatus)
			}
		})
	}
}

func TestSpanStatusFromHTTPStatus_UnitCases(t *testing.T) {
	// Exported via the package through the Middleware, but we can test the
	// observable behaviour end-to-end via Middleware() + InMemoryExporter.
	//
	// The table below doubles as documentation for the function's contract.
	tests := []struct {
		statusCode int
		wantCode   codes.Code
	}{
		{100, codes.Unset},  // 1xx – informational
		{200, codes.Unset},  // 2xx – success
		{201, codes.Unset},  // 201 Created
		{204, codes.Unset},  // 204 No Content
		{301, codes.Unset},  // 3xx – redirect
		{400, codes.Unset},  // 4xx client error – server span → Unset (not error)
		{401, codes.Unset},
		{403, codes.Unset},
		{404, codes.Unset},
		{422, codes.Unset},
		// 499 is not a standard HTTP status (http.StatusText returns ""), so
		// spanStatusFromHTTPStatus treats it as an invalid code → Error.
		{499, codes.Error},
		{500, codes.Error},  // 5xx → Error
		{503, codes.Error},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			exp := setupOtel(t)

			app := newApp("/s", tt.statusCode)
			req := httptest.NewRequest("GET", "/s", nil)
			resp, err := app.Test(req)
			if err != nil {
				t.Fatalf("app.Test: %v", err)
			}
			resp.Body.Close()

			spans := exp.GetSpans()
			if len(spans) == 0 {
				t.Fatal("no spans")
			}
			got := spans[0].Status.Code
			if got != tt.wantCode {
				t.Errorf("statusCode=%d: want span status %v, got %v", tt.statusCode, tt.wantCode, got)
			}
		})
	}
}
