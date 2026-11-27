package instrumentation

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	tracenoop "go.opentelemetry.io/otel/trace/noop"
)

func TestNewInstrumentation(t *testing.T) {
	tests := []struct {
		name            string
		env             map[string]string
		opts            func(w *bytes.Buffer) []InstrumentationOption
		wantErr         bool
		wantInstruments int
		wantGlobals     bool
	}{
		{
			name:            "no options",
			opts:            func(w *bytes.Buffer) []InstrumentationOption { return nil },
			wantInstruments: 0,
		},
		{
			name: "none exporters create no providers",
			opts: func(w *bytes.Buffer) []InstrumentationOption {
				return []InstrumentationOption{
					WithTracing(TracerConfig{Exporter: TraceExporterNone}),
					WithMetrics(MeterConfig{Exporter: MetricExporterNone}),
				}
			},
			wantInstruments: 0,
		},
		{
			name: "console exporters set globals",
			opts: func(w *bytes.Buffer) []InstrumentationOption {
				return []InstrumentationOption{
					WithLogging(LoggerConfig{Writer: w}),
					WithTracing(TracerConfig{Exporter: TraceExporterConsole, Writer: w}),
					WithMetrics(MeterConfig{Exporter: MetricExporterConsole, Writer: w}),
				}
			},
			wantInstruments: 2,
			wantGlobals:     true,
		},
		{
			name: "setup error leaves globals unchanged",
			env:  map[string]string{"OTEL_RESOURCE_ATTRIBUTES": "invalid"},
			opts: func(w *bytes.Buffer) []InstrumentationOption {
				return []InstrumentationOption{
					WithLogging(LoggerConfig{Writer: w}),
					WithTracing(TracerConfig{Exporter: TraceExporterConsole, Writer: w}),
				}
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// arrange
			ctx := t.Context()
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			resetGlobals(t)
			logger := slog.Default()
			var buf bytes.Buffer

			// act
			inst, err := NewInstrumentation(ctx, tt.opts(&buf)...)

			// assert
			if tt.wantErr {
				require.Error(t, err)
				assert.Nil(t, inst)
				assert.IsType(t, tracenoop.TracerProvider{}, otel.GetTracerProvider())
				assert.IsType(t, metricnoop.MeterProvider{}, otel.GetMeterProvider())
				assert.Same(t, logger, slog.Default())
				return
			}

			require.NoError(t, err)
			assert.Len(t, inst.instruments, tt.wantInstruments)
			assert.ElementsMatch(t, []string{"traceparent", "tracestate", "baggage"}, otel.GetTextMapPropagator().Fields())
			if tt.wantGlobals {
				assert.IsType(t, &sdktrace.TracerProvider{}, otel.GetTracerProvider())
				assert.IsType(t, &sdkmetric.MeterProvider{}, otel.GetMeterProvider())
				assert.NotSame(t, logger, slog.Default())
			} else {
				assert.IsType(t, tracenoop.TracerProvider{}, otel.GetTracerProvider())
				assert.IsType(t, metricnoop.MeterProvider{}, otel.GetMeterProvider())
			}
			assert.NoError(t, inst.Shutdown(ctx))
		})
	}
}

func TestInstrumentation_Shutdown(t *testing.T) {
	errA := errors.New("a failed")
	errB := errors.New("b failed")

	tests := []struct {
		name        string
		timeout     time.Duration
		instruments []*fakeInstrument
		wantErrs    []error
	}{
		{
			name:        "no instruments",
			timeout:     time.Second,
			instruments: nil,
		},
		{
			name:        "all succeed",
			timeout:     time.Second,
			instruments: []*fakeInstrument{{}, {}},
		},
		{
			name:        "joins errors",
			timeout:     time.Second,
			instruments: []*fakeInstrument{{err: errA}, {}, {err: errB}},
			wantErrs:    []error{errA, errB},
		},
		{
			name:        "applies shutdown timeout",
			timeout:     10 * time.Millisecond,
			instruments: []*fakeInstrument{{block: true}},
			wantErrs:    []error{context.DeadlineExceeded},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// arrange
			inst := &Instrumentation{shutdownTimeout: tt.timeout}
			for _, instrument := range tt.instruments {
				inst.instruments = append(inst.instruments, instrument)
			}

			// act
			err := inst.Shutdown(t.Context())

			// assert
			if tt.wantErrs == nil {
				assert.NoError(t, err)
			}
			for _, want := range tt.wantErrs {
				assert.ErrorIs(t, err, want)
			}
			assert.NotContains(t, errString(err), "\n")
			for _, instrument := range tt.instruments {
				assert.Equal(t, int32(1), instrument.calls.Load())
			}
		})
	}
}

func TestInstrumentation_Shutdown_OnlyOnce(t *testing.T) {
	// arrange
	instrument := &fakeInstrument{err: errors.New("failed")}
	inst := &Instrumentation{
		shutdownTimeout: time.Second,
		instruments:     []Instrument{instrument},
	}

	// act
	firstErr := inst.Shutdown(t.Context())
	secondErr := inst.Shutdown(t.Context())

	// assert
	assert.Error(t, firstErr)
	assert.NoError(t, secondErr)
	assert.Equal(t, int32(1), instrument.calls.Load())
}

func TestInstrumentation_CleanupOnError(t *testing.T) {
	setupErr := errors.New("setup failed")
	shutdownErr := errors.New("shutdown failed")

	tests := []struct {
		name       string
		instrument *fakeInstrument
		wantErrs   []error
		wantMsg    string
	}{
		{
			name:       "returns setup error",
			instrument: &fakeInstrument{},
			wantErrs:   []error{setupErr},
			wantMsg:    "setup failed",
		},
		{
			name:       "joins shutdown error",
			instrument: &fakeInstrument{err: shutdownErr},
			wantErrs:   []error{setupErr, shutdownErr},
			wantMsg:    "setup failed; shutdown instruments: shutdown failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// arrange
			inst := &Instrumentation{
				shutdownTimeout: time.Second,
				instruments:     []Instrument{tt.instrument},
			}
			ctx, cancel := context.WithCancel(t.Context())
			cancel()

			// act
			err := inst.cleanupOnError(ctx, setupErr)

			// assert
			for _, want := range tt.wantErrs {
				assert.ErrorIs(t, err, want)
			}
			assert.EqualError(t, err, tt.wantMsg)
			assert.Equal(t, int32(1), tt.instrument.calls.Load())
			assert.NoError(t, tt.instrument.ctxErr, "cleanup must not inherit cancellation")
		})
	}
}

type fakeInstrument struct {
	err    error
	block  bool
	calls  atomic.Int32
	ctxErr error
}

func (f *fakeInstrument) Shutdown(ctx context.Context) error {
	f.calls.Add(1)
	f.ctxErr = ctx.Err()
	if f.block {
		<-ctx.Done()
		return ctx.Err()
	}
	return f.err
}

// resetGlobals installs fresh no-op providers and restores the default logger
// and propagator after the test, as NewInstrumentation changes them globally.
func resetGlobals(t *testing.T) {
	t.Helper()

	logger := slog.Default()
	propagator := otel.GetTextMapPropagator()
	otel.SetTracerProvider(tracenoop.NewTracerProvider())
	otel.SetMeterProvider(metricnoop.NewMeterProvider())
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator())

	t.Cleanup(func() {
		slog.SetDefault(logger)
		otel.SetTextMapPropagator(propagator)
		otel.SetTracerProvider(tracenoop.NewTracerProvider())
		otel.SetMeterProvider(metricnoop.NewMeterProvider())
	})
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
