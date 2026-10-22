package main

import (
	"context"
	"errors"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/runtime"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// tracer and meter are what the rest of the code uses (tracer.Start,
// meter.Int64Counter). They come from the OTel API, which on its own does
// nothing: every call is a no-op. So it is fine to create them here, before
// any setup. Once initTelemetry registers the real providers with otel.Set...,
// they start recording for real.
var (
	tracer = otel.Tracer("checkout_tracername")
	meter  = otel.Meter("checkout_metername")
)

// The app sends everything to one place: the OTel Collector (see
// docker-compose.yml and deploy/otel-collector.yaml). Its Datadog exporter
// converts each signal and forwards it to Datadog:
// traces -> APM, metrics -> Metrics, logs -> Logs.
// The app only speaks OTLP, so it has no Datadog code or library in it.
const collectorEndpoint = "localhost:4317"

// ---------------------------------------------------------------------------
// 0. Setup
//
// Every signal (traces, metrics, logs) is set up in the same 3 steps:
//
//  1. Exporter: HOW and WHERE to send data. Here: OTLP over gRPC to the
//     OTel Collector. It only sends what it is handed; it creates nothing.
//  2. Provider: the engine. It creates spans / counters / log records,
//     keeps them in memory, and decides WHEN to hand them to the exporter.
//  3. otel.Set...Provider: makes the provider global, so `tracer`, `meter`
//     and slog (and libraries like otelhttp, otelpgx) start using it.
// ---------------------------------------------------------------------------

func initTelemetry(ctx context.Context) (shutdown func(context.Context) error, err error) {
	var shutdowns []func(context.Context) error
	shutdown = func(ctx context.Context) error {
		var errs error
		for _, fn := range shutdowns {
			errs = errors.Join(errs, fn(ctx))
		}
		return errs
	}
	// shutdown sends whatever is still waiting in memory, then closes the
	// connections. main calls it on exit; without it the last few spans,
	// metrics and logs would be lost.
	//
	// If a later step fails, flush whatever was already started.
	defer func() {
		if err != nil {
			_ = shutdown(ctx)
		}
	}()

	// Resource = "who is sending this data". These labels are attached to
	// every span, metric and log. Datadog turns the first three into its
	// standard tags (service, version, env), which is how it matches a
	// trace, its logs and its metrics to the same service.
	res, err := resource.New(ctx, resource.WithAttributes(
		attribute.String("service.name", "checkout_servicename"),
		attribute.String("service.version", "0.1.0"),
		attribute.String("deployment.environment", "dev"),
		attribute.String("laptop", "fidelis-bfree"),
	))
	if err != nil {
		return nil, err
	}



	// --- Traces -------------------------------------------------------------

	// Exporter: packs finished spans into OTLP and sends them over gRPC.
	// WithInsecure = no TLS, fine for a Collector on the same machine.
	// It connects lazily, so this works even if the Collector is not up yet.
	traceExp, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithInsecure(),
		otlptracegrpc.WithEndpoint(collectorEndpoint),
	)
	if err != nil {
		return nil, err
	}
	// Provider: creates a span on every tracer.Start(...).
	// WithBatcher: when a span ends it goes into a queue, and the queue is
	// sent every few seconds (or when it fills up). So span.End() never
	// waits on the network.
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExp),
		sdktrace.WithResource(res),
	)
	shutdowns = append(shutdowns, tp.Shutdown)
	otel.SetTracerProvider(tp)



	// --- Metrics ------------------------------------------------------------

	// Exporter: same job as above, for metric data.
	metricExp, err := otlpmetricgrpc.New(ctx,
		otlpmetricgrpc.WithInsecure(),
		otlpmetricgrpc.WithEndpoint(collectorEndpoint),
	)
	if err != nil {
		return nil, err
	}
	// Provider: creates counters/histograms and keeps their running totals
	// in memory. counter.Add(1) only bumps a number; nothing is sent.
	mp := sdkmetric.NewMeterProvider(
		// Metrics use a reader instead of a batcher: every 5s it reads the
		// current totals and sends ONE snapshot, no matter how many requests
		// happened. 5s keeps the demo quick; the default is 60s.
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExp,
			sdkmetric.WithInterval(5*time.Second))),
		sdkmetric.WithResource(res),
	)
	shutdowns = append(shutdowns, mp.Shutdown)
	otel.SetMeterProvider(mp)

	// Free Go metrics: memory, GC, goroutines. Must run after
	// SetMeterProvider, because it registers on the global provider.
	if err := runtime.Start(); err != nil {
		return nil, err
	}



	// --- Logs ---------------------------------------------------------------

	// Exporter: same job again, for log records.
	logExp, err := otlploggrpc.New(ctx,
		otlploggrpc.WithInsecure(),
		otlploggrpc.WithEndpoint(collectorEndpoint),
	)
	if err != nil {
		return nil, err
	}
	// Provider: works like traces: records go into a queue that is sent in
	// batches. Our code never calls it directly; slog feeds it through the
	// otelslog bridge in logger.go.
	lp := sdklog.NewLoggerProvider(
		sdklog.WithProcessor(sdklog.NewBatchProcessor(logExp)),
		sdklog.WithResource(res),
	)
	shutdowns = append(shutdowns, lp.Shutdown)
	otel.SetLoggerProvider(lp) // picked up by the otelslog bridge in logger.go

	// Propagator: how the trace ID travels between services or through a
	// queue. Inject writes it into headers (the W3C "traceparent" header);
	// Extract reads it back on the other side. See queue.go and worker.go.
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))
	return shutdown, nil
}
