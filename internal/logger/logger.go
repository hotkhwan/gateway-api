// internal/logger/logger.go
package logger

import (
	"context"
	"os"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"go.opentelemetry.io/otel/trace"
)

// --------- internal writers (stdout/stderr split) ---------

type levelWriter struct{}

func (lw levelWriter) Write(p []byte) (int, error) {
	return os.Stdout.Write(p)
}

func (lw levelWriter) WriteLevel(level zerolog.Level, p []byte) (int, error) {
	if level >= zerolog.ErrorLevel {
		return os.Stderr.Write(p)
	}
	return os.Stdout.Write(p)
}

// --------- boot/init ---------

// Init configures zerolog (level, pretty, caller).
// ENV:
//
//	LOG_LEVEL=debug|info|warn|error  (default: info)
//	LOG_PRETTY=true|false            (default: false)
//	LOG_CALLER=true|false            (default: false)
func Init() {
	zerolog.TimeFieldFormat = time.RFC3339Nano

	// level
	levelStr := strings.ToLower(os.Getenv("LOG_LEVEL"))
	level, err := zerolog.ParseLevel(levelStr)
	if err != nil {
		level = zerolog.InfoLevel
	}
	zerolog.SetGlobalLevel(level)

	// output
	var output zerolog.LevelWriter
	if os.Getenv("LOG_PRETTY") == "true" {
		cw := zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339}
		output = zerolog.MultiLevelWriter(cw)
	} else {
		output = levelWriter{}
	}

	// base logger
	builder := zerolog.New(output).Level(level).With().Timestamp()
	if os.Getenv("LOG_CALLER") == "true" {
		builder = builder.Caller()
	}
	log.Logger = builder.Logger()
}

// Boot returns a logger for boot/config phases (no context/trace yet).
// Same as WithMeta(component, source).
func Boot(component, source string) zerolog.Logger {
	return WithMeta(component, source)
}

// WithMeta builds a logger without context (e.g., boot, jobs, one-off tools).
// You can pass one optional map for extra fixed fields.
func WithMeta(component, source string, extra ...map[string]string) zerolog.Logger {
	b := log.With().Str("component", component).Str("source", source)
	if len(extra) > 0 {
		for k, v := range extra[0] {
			b = b.Str(k, v)
		}
	}
	return b.Logger()
}

// FromCtx builds a logger enriched with trace/span IDs from OpenTelemetry context.
// Use this in request handlers, Kafka consumers, service logic, etc.
func FromCtx(ctx context.Context, component, source string) zerolog.Logger {
	b := log.With().Str("component", component).Str("source", source)
	if ctx != nil {
		if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
			b = b.
				Str("traceId", sc.TraceID().String()).
				Str("spanId", sc.SpanID().String())
		}
	}
	return b.Logger()
}
