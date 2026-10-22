package main

import (
	"context"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// Package-level tracer. Safe to create before the provider is set;
// it starts working as soon as initTracer registers the global provider.
var tracer = otel.Tracer("checkout_tracername")

// ---------------------------------------------------------------------------
// 0. Setup: exporter -> provider -> propagator
// ---------------------------------------------------------------------------

func initTracer(ctx context.Context) (func(context.Context) error, error) {
	// Sends spans over OTLP/gRPC to a Collector or Jaeger on :4317.
	exp, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithInsecure(),
		otlptracegrpc.WithEndpoint("localhost:4317"),
	)
	if err != nil {
		return nil, err
	}

	// Resource = "who is sending this telemetry".
	res, err := resource.New(ctx, resource.WithAttributes(
		attribute.String("service.name", "checkout_servicename"),
		attribute.String("deployment.environment", "dev"),
		attribute.String("laptop", "fidelis-bfree"),
	))
	if err != nil {
		return nil, err
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp), // batches spans instead of sending one by one
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)

	// W3C traceparent header: this is how the trace ID travels between hops.
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))
	return tp.Shutdown, nil
}
