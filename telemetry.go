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

// Package-level tracer and meter. Safe to create before the providers are
// set; they start working as soon as initTelemetry registers the globals.
var (
	tracer = otel.Tracer("checkout_tracername")
	meter  = otel.Meter("checkout_metername")
)

// All three signals go to the OTel Collector, which fans them out:
// traces -> Jaeger, metrics -> Prometheus, logs -> Loki.
// Jaeger only stores traces, so the Collector is what makes logs and
// metrics possible without changing the app.
const collectorEndpoint = "localhost:4317"

// ---------------------------------------------------------------------------
// 0. Setup: resource -> (exporter -> provider) x 3 -> propagator
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
	// If a later step fails, flush whatever was already started.
	defer func() {
		if err != nil {
			_ = shutdown(ctx)
		}
	}()

	// Resource = "who is sending this telemetry". Shared by all three
	// signals, which is what lets backends line them up per service.
	res, err := resource.New(ctx, resource.WithAttributes(
		attribute.String("service.name", "checkout_servicename"),
		attribute.String("deployment.environment", "dev"),
		attribute.String("laptop", "fidelis-bfree"),
	))
	if err != nil {
		return nil, err
	}

	// --- Traces -------------------------------------------------------------
	traceExp, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithInsecure(),
		otlptracegrpc.WithEndpoint(collectorEndpoint),
	)
	if err != nil {
		return nil, err
	}
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExp), // batches spans instead of sending one by one
		sdktrace.WithResource(res),
	)
	shutdowns = append(shutdowns, tp.Shutdown)
	otel.SetTracerProvider(tp)



	// --- Metrics ------------------------------------------------------------
	metricExp, err := otlpmetricgrpc.New(ctx,
		otlpmetricgrpc.WithInsecure(),
		otlpmetricgrpc.WithEndpoint(collectorEndpoint),
	)
	if err != nil {
		return nil, err
	}
	mp := sdkmetric.NewMeterProvider(
		// Metrics are aggregated in-process and pushed on a timer, not per
		// event. 5s keeps the demo snappy; the default is 60s.
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExp,
			sdkmetric.WithInterval(5*time.Second))),
		sdkmetric.WithResource(res),
	)
	shutdowns = append(shutdowns, mp.Shutdown)
	otel.SetMeterProvider(mp)

	// Go runtime metrics: memory, GC, goroutines.
	if err := runtime.Start(); err != nil {
		return nil, err
	}



	// --- Logs ---------------------------------------------------------------
	logExp, err := otlploggrpc.New(ctx,
		otlploggrpc.WithInsecure(),
		otlploggrpc.WithEndpoint(collectorEndpoint),
	)
	if err != nil {
		return nil, err
	}
	lp := sdklog.NewLoggerProvider(
		sdklog.WithProcessor(sdklog.NewBatchProcessor(logExp)),
		sdklog.WithResource(res),
	)
	shutdowns = append(shutdowns, lp.Shutdown)
	otel.SetLoggerProvider(lp) // picked up by the otelslog bridge in logger.go



	// W3C traceparent header: this is how the trace ID travels between hops.
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))
	return shutdown, nil
}
