package instrumentation

import (
	"context"
	"fmt"
	"io"
	"os"

	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// TracerConfig configures the tracer provider created by NewTracerProvider.
// Writer is only used by TraceExporterConsole and defaults to stdout.
type TracerConfig struct {
	Exporter TraceExporter
	Writer   io.Writer
}

// TraceExporter selects where spans are exported to.
type TraceExporter int

const (
	TraceExporterNone TraceExporter = iota
	TraceExporterConsole
	TraceExporterOTLP
)

// ParseTraceExporter parses a trace exporter name as used by
// OTEL_TRACES_EXPORTER. Unknown values fall back to TraceExporterNone.
func ParseTraceExporter(s string) TraceExporter {
	switch s {
	case "console":
		return TraceExporterConsole
	case "otlp":
		return TraceExporterOTLP
	default:
		return TraceExporterNone
	}
}

// NewTracerProvider creates a tracer provider from the given config. A nil
// config falls back to the zero value (no exporter) and a nil writer to stdout.
// The resource and the OTLP exporter are configured through the standard
// OTEL_* environment variables. Callers must call Shutdown on the returned
// provider to flush pending spans.
func NewTracerProvider(ctx context.Context, cfg *TracerConfig) (*sdktrace.TracerProvider, error) {
	if cfg == nil {
		cfg = &TracerConfig{}
	}

	res, err := newResource(ctx)
	if err != nil {
		return nil, fmt.Errorf("create resource: %w", err)
	}

	opts := []sdktrace.TracerProviderOption{
		sdktrace.WithResource(res),
	}

	switch cfg.Exporter {
	case TraceExporterConsole:
		w := cfg.Writer
		if w == nil {
			w = os.Stdout
		}

		exporter, err := stdouttrace.New(stdouttrace.WithWriter(w))
		if err != nil {
			return nil, fmt.Errorf("create console exporter: %w", err)
		}
		opts = append(opts, sdktrace.WithBatcher(exporter))
	case TraceExporterOTLP:
		exporter, err := otlptracegrpc.New(ctx)
		if err != nil {
			return nil, fmt.Errorf("create otlp exporter: %w", err)
		}
		opts = append(opts, sdktrace.WithBatcher(exporter))
	}

	return sdktrace.NewTracerProvider(opts...), nil
}
