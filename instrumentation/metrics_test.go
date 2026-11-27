package instrumentation

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewMeterProvider(t *testing.T) {
	tests := []struct {
		name       string
		env        map[string]string
		cfg        func(w *bytes.Buffer) *MeterConfig
		wantOutput []string
		// noCollector skips the final export on shutdown, which would otherwise
		// block until the export times out.
		noCollector bool
	}{
		{
			name:       "nil config exports nothing",
			cfg:        func(w *bytes.Buffer) *MeterConfig { return nil },
			wantOutput: nil,
		},
		{
			name: "none exports nothing",
			cfg: func(w *bytes.Buffer) *MeterConfig {
				return &MeterConfig{Exporter: MetricExporterNone, Writer: w}
			},
			wantOutput: nil,
		},
		{
			name: "console with default service name",
			cfg: func(w *bytes.Buffer) *MeterConfig {
				return &MeterConfig{Exporter: MetricExporterConsole, Writer: w}
			},
			wantOutput: []string{"test.counter", "agentoffice"},
		},
		{
			name: "console with service name from env",
			env:  map[string]string{"OTEL_SERVICE_NAME": "from-env"},
			cfg: func(w *bytes.Buffer) *MeterConfig {
				return &MeterConfig{Exporter: MetricExporterConsole, Writer: w}
			},
			wantOutput: []string{"test.counter", "from-env"},
		},
		{
			name: "otlp",
			env: map[string]string{
				"OTEL_EXPORTER_OTLP_ENDPOINT": "http://localhost:4317",
				"OTEL_EXPORTER_OTLP_INSECURE": "true",
			},
			cfg: func(w *bytes.Buffer) *MeterConfig {
				return &MeterConfig{Exporter: MetricExporterOTLP}
			},
			wantOutput:  nil,
			noCollector: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// arrange
			ctx := t.Context()
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			var buf bytes.Buffer

			// act
			mp, err := NewMeterProvider(ctx, tt.cfg(&buf))

			// assert
			require.NoError(t, err)
			if tt.noCollector {
				canceledCtx, cancel := context.WithCancel(ctx)
				cancel()
				_ = mp.Shutdown(canceledCtx)
				return
			}
			if tt.wantOutput == nil {
				assert.NoError(t, mp.Shutdown(ctx))
				assert.Empty(t, buf.String())
				return
			}

			counter, err := mp.Meter("test").Int64Counter("test.counter")
			require.NoError(t, err)
			counter.Add(ctx, 1)
			require.NoError(t, mp.Shutdown(ctx))
			for _, want := range tt.wantOutput {
				assert.Contains(t, buf.String(), want)
			}
		})
	}
}

func TestParseMetricExporter(t *testing.T) {
	tests := []struct {
		name         string
		s            string
		wantExporter MetricExporter
	}{
		{name: "none", s: "none", wantExporter: MetricExporterNone},
		{name: "console", s: "console", wantExporter: MetricExporterConsole},
		{name: "otlp", s: "otlp", wantExporter: MetricExporterOTLP},
		{name: "empty defaults to none", s: "", wantExporter: MetricExporterNone},
		{name: "unknown defaults to none", s: "prometheus", wantExporter: MetricExporterNone},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// arrange
			// act
			exporter := ParseMetricExporter(tt.s)

			// assert
			assert.Equal(t, tt.wantExporter, exporter)
		})
	}
}
