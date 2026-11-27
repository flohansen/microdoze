package instrumentation

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewTracerProvider(t *testing.T) {
	tests := []struct {
		name       string
		env        map[string]string
		cfg        func(w *bytes.Buffer) *TracerConfig
		wantOutput []string
	}{
		{
			name:       "nil config exports nothing",
			cfg:        func(w *bytes.Buffer) *TracerConfig { return nil },
			wantOutput: nil,
		},
		{
			name: "none exports nothing",
			cfg: func(w *bytes.Buffer) *TracerConfig {
				return &TracerConfig{Exporter: TraceExporterNone, Writer: w}
			},
			wantOutput: nil,
		},
		{
			name: "console with default service name",
			cfg: func(w *bytes.Buffer) *TracerConfig {
				return &TracerConfig{Exporter: TraceExporterConsole, Writer: w}
			},
			wantOutput: []string{"test-span", "agentoffice"},
		},
		{
			name: "console with service name from env",
			env:  map[string]string{"OTEL_SERVICE_NAME": "from-env"},
			cfg: func(w *bytes.Buffer) *TracerConfig {
				return &TracerConfig{Exporter: TraceExporterConsole, Writer: w}
			},
			wantOutput: []string{"test-span", "from-env"},
		},
		{
			name: "otlp",
			env: map[string]string{
				"OTEL_EXPORTER_OTLP_ENDPOINT": "http://localhost:4317",
				"OTEL_EXPORTER_OTLP_INSECURE": "true",
			},
			cfg: func(w *bytes.Buffer) *TracerConfig {
				return &TracerConfig{Exporter: TraceExporterOTLP}
			},
			wantOutput: nil,
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
			tp, err := NewTracerProvider(ctx, tt.cfg(&buf))

			// assert
			require.NoError(t, err)
			if tt.wantOutput == nil {
				assert.NoError(t, tp.Shutdown(ctx))
				assert.Empty(t, buf.String())
				return
			}

			_, span := tp.Tracer("test").Start(ctx, "test-span")
			span.End()
			require.NoError(t, tp.Shutdown(ctx))
			for _, want := range tt.wantOutput {
				assert.Contains(t, buf.String(), want)
			}
		})
	}
}

func TestParseTraceExporter(t *testing.T) {
	tests := []struct {
		name         string
		s            string
		wantExporter TraceExporter
	}{
		{name: "none", s: "none", wantExporter: TraceExporterNone},
		{name: "console", s: "console", wantExporter: TraceExporterConsole},
		{name: "otlp", s: "otlp", wantExporter: TraceExporterOTLP},
		{name: "empty defaults to none", s: "", wantExporter: TraceExporterNone},
		{name: "unknown defaults to none", s: "jaeger", wantExporter: TraceExporterNone},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// arrange
			// act
			exporter := ParseTraceExporter(tt.s)

			// assert
			assert.Equal(t, tt.wantExporter, exporter)
		})
	}
}
