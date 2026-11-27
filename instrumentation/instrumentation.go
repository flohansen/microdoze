// Package instrumentation sets up logging, tracing and metrics for a service.
// Use NewInstrumentation to configure them in one call and install them as the
// process-wide defaults for log/slog and OpenTelemetry.
package instrumentation

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

const (
	defaultShutdownTimeout = 5 * time.Second
)

// Instrument is a telemetry component that must be shut down to flush pending
// data, such as a tracer or meter provider.
type Instrument interface {
	Shutdown(ctx context.Context) error
}

// Instrumentation holds the instruments created by NewInstrumentation. Call
// Shutdown before the process exits to flush pending telemetry.
type Instrumentation struct {
	shutdownTimeout time.Duration
	instruments     []Instrument
	shutdownOnce    sync.Once
}

// NewInstrumentation creates the logger and the tracer and meter providers
// enabled through opts. Tracing and metrics with a None exporter are skipped.
//
// Only after all of them were created successfully, it installs them as the
// default slog logger and the global OpenTelemetry providers, and sets the W3C
// TraceContext and Baggage propagator. On error, instruments that were already
// created are shut down and the globals are left unchanged.
func NewInstrumentation(ctx context.Context, opts ...InstrumentationOption) (*Instrumentation, error) {
	instOpts := &instrumentationOptions{
		shutdownTimeout: defaultShutdownTimeout,
	}
	for _, opt := range opts {
		opt(instOpts)
	}

	inst := &Instrumentation{
		shutdownTimeout: instOpts.shutdownTimeout,
	}

	var (
		logger *slog.Logger
		tp     *sdktrace.TracerProvider
		mp     *sdkmetric.MeterProvider
		err    error
	)

	if instOpts.logger != nil {
		logger = NewLogger(instOpts.logger)
	}
	if instOpts.tracer != nil && instOpts.tracer.Exporter != TraceExporterNone {
		tp, err = NewTracerProvider(ctx, instOpts.tracer)
		if err != nil {
			return nil, inst.cleanupOnError(ctx, fmt.Errorf("new tracer provider: %w", err))
		}
		inst.instruments = append(inst.instruments, tp)
	}
	if instOpts.meter != nil && instOpts.meter.Exporter != MetricExporterNone {
		mp, err = NewMeterProvider(ctx, instOpts.meter)
		if err != nil {
			return nil, inst.cleanupOnError(ctx, fmt.Errorf("new meter provider: %w", err))
		}
		inst.instruments = append(inst.instruments, mp)
	}

	if logger != nil {
		slog.SetDefault(logger)
	}
	if tp != nil {
		otel.SetTracerProvider(tp)
	}
	if mp != nil {
		otel.SetMeterProvider(mp)
	}

	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))
	return inst, nil
}

func (i *Instrumentation) cleanupOnError(ctx context.Context, err error) error {
	if shutdownErr := i.Shutdown(context.WithoutCancel(ctx)); shutdownErr != nil {
		err = fmt.Errorf("%w; shutdown instruments: %w", err, shutdownErr)
	}
	return err
}

// Shutdown stops all instruments concurrently. It waits at most the configured
// shutdown timeout, or until ctx is done, whichever comes first.
//
// Shutdown is safe to call multiple times and from multiple goroutines. Only
// the first call shuts down the instruments; later calls block until it is
// done and return nil.
func (i *Instrumentation) Shutdown(ctx context.Context) error {
	var err error
	i.shutdownOnce.Do(func() {
		err = i.shutdown(ctx)
	})
	return err
}

func (i *Instrumentation) shutdown(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, i.shutdownTimeout)
	defer cancel()

	var (
		wg  sync.WaitGroup
		mu  sync.Mutex
		err error
	)

	for _, instrument := range i.instruments {
		wg.Go(func() {
			if shutdownErr := instrument.Shutdown(ctx); shutdownErr != nil {
				mu.Lock()
				if err == nil {
					err = shutdownErr
				} else {
					err = fmt.Errorf("%w; %w", err, shutdownErr)
				}
				mu.Unlock()
			}
		})
	}
	wg.Wait()

	return err
}

type instrumentationOptions struct {
	logger *LoggerConfig
	tracer *TracerConfig
	meter  *MeterConfig

	shutdownTimeout time.Duration
}

// InstrumentationOption configures NewInstrumentation.
type InstrumentationOption func(*instrumentationOptions)

// WithLogging enables a logger built from cfg and installs it as the default
// slog logger.
func WithLogging(cfg LoggerConfig) InstrumentationOption {
	return func(opts *instrumentationOptions) {
		opts.logger = &cfg
	}
}

// WithTracing enables a tracer provider built from cfg and installs it as the
// global OpenTelemetry tracer provider.
func WithTracing(cfg TracerConfig) InstrumentationOption {
	return func(opts *instrumentationOptions) {
		opts.tracer = &cfg
	}
}

// WithMetrics enables a meter provider built from cfg and installs it as the
// global OpenTelemetry meter provider.
func WithMetrics(cfg MeterConfig) InstrumentationOption {
	return func(opts *instrumentationOptions) {
		opts.meter = &cfg
	}
}

// WithShutdownTimeout sets the maximum time Shutdown waits for instruments to
// stop. It also applies to the cleanup after a failed setup. Defaults to 5 seconds.
func WithShutdownTimeout(timeout time.Duration) InstrumentationOption {
	return func(opts *instrumentationOptions) {
		opts.shutdownTimeout = timeout
	}
}
