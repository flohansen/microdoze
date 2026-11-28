package instrumentation

import (
	"context"
	"fmt"
	"io"
	"os"

	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/stdout/stdoutmetric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

// MeterConfig configures the meter provider created by NewMeterProvider. Writer
// is only used by MetricExporterConsole and defaults to stdout.
type MeterConfig struct {
	Exporter MetricExporter
	Writer   io.Writer
}

// MetricExporter selects where metrics are exported to.
type MetricExporter int

const (
	MetricExporterNone MetricExporter = iota
	MetricExporterConsole
	MetricExporterOTLP
)

// ParseMetricExporter parses a metric exporter name as used by
// OTEL_METRICS_EXPORTER. Unknown values fall back to MetricExporterNone.
func ParseMetricExporter(s string) MetricExporter {
	switch s {
	case "console":
		return MetricExporterConsole
	case "otlp":
		return MetricExporterOTLP
	default:
		return MetricExporterNone
	}
}

// ParseMetricWriter parses the name of the writer used by
// MetricExporterConsole. Unknown values fall back to stdout.
func ParseMetricWriter(s string) io.Writer {
	switch s {
	case "stderr":
		return os.Stderr
	default:
		return os.Stdout
	}
}

// NewMeterProvider creates a meter provider from the given config. A nil config
// falls back to the zero value (no exporter) and a nil writer to stdout. The
// resource, the OTLP exporter and the export interval are configured through
// the standard OTEL_* environment variables. Callers must call Shutdown on the
// returned provider to flush pending metrics.
func NewMeterProvider(ctx context.Context, cfg *MeterConfig) (*sdkmetric.MeterProvider, error) {
	if cfg == nil {
		cfg = &MeterConfig{}
	}

	res, err := newResource(ctx)
	if err != nil {
		return nil, fmt.Errorf("create resource: %w", err)
	}

	opts := []sdkmetric.Option{
		sdkmetric.WithResource(res),
	}

	switch cfg.Exporter {
	case MetricExporterConsole:
		w := cfg.Writer
		if w == nil {
			w = os.Stdout
		}

		exporter, err := stdoutmetric.New(stdoutmetric.WithWriter(w))
		if err != nil {
			return nil, fmt.Errorf("create console exporter: %w", err)
		}
		opts = append(opts, sdkmetric.WithReader(sdkmetric.NewPeriodicReader(exporter)))
	case MetricExporterOTLP:
		exporter, err := otlpmetricgrpc.New(ctx)
		if err != nil {
			return nil, fmt.Errorf("create otlp exporter: %w", err)
		}
		opts = append(opts, sdkmetric.WithReader(sdkmetric.NewPeriodicReader(exporter)))
	}

	return sdkmetric.NewMeterProvider(opts...), nil
}
