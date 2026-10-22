package main

import (
	"context"

	"go.opentelemetry.io/otel/metric"
)

// Custom business metrics. HTTP server metrics (http.server.request.duration,
// etc.) come for free from otelhttp, DB metrics from otelpgx, and Go runtime
// metrics from the runtime package started in initTelemetry.
var (
	ordersCreated     metric.Int64Counter
	ordersFailed      metric.Int64Counter
	orderAmount       metric.Float64Histogram
	messagesProcessed metric.Int64Counter
	emailDuration     metric.Float64Histogram
)

func initMetrics(q *Queue) error {
	var err error

	// Counter: only goes up. Datadog shows it as orders.created.
	if ordersCreated, err = meter.Int64Counter("orders.created",
		metric.WithDescription("Orders successfully placed"),
		metric.WithUnit("{order}")); err != nil {
		return err
	}
	if ordersFailed, err = meter.Int64Counter("orders.failed",
		metric.WithDescription("Orders that could not be placed, by error.type"),
		metric.WithUnit("{order}")); err != nil {
		return err
	}

	// Histogram: distribution of values, so you can ask for p50/p95/p99.
	if orderAmount, err = meter.Float64Histogram("orders.amount",
		metric.WithDescription("Order value"),
		metric.WithUnit("USD"),
		metric.WithExplicitBucketBoundaries(5, 10, 25, 50, 100, 250, 500, 1000)); err != nil {
		return err
	}
	if messagesProcessed, err = meter.Int64Counter("messaging.process.messages",
		metric.WithDescription("Queue messages handled by the worker, by outcome"),
		metric.WithUnit("{message}")); err != nil {
		return err
	}
	if emailDuration, err = meter.Float64Histogram("email.send.duration",
		metric.WithDescription("Time spent calling the email provider"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(0.025, 0.05, 0.075, 0.1, 0.15, 0.2, 0.5)); err != nil {
		return err
	}

	// Observable gauge: not recorded by our code, but read by the SDK every
	// collection cycle via this callback.
	_, err = meter.Int64ObservableGauge("queue.depth",
		metric.WithDescription("Messages waiting in the in-memory queue"),
		metric.WithUnit("{message}"),
		metric.WithInt64Callback(func(_ context.Context, o metric.Int64Observer) error {
			o.Observe(int64(len(q.ch)))
			return nil
		}))
	return err
}
