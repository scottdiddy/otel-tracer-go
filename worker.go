package main

import (
	"context"
	"encoding/json"
	"log/slog"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

type Worker struct{ q *Queue }

func (w *Worker) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-w.q.ch:
			w.handle(msg)
		}
	}
}

func (w *Worker) handle(msg Message) {
	// No request context exists here (this runs after the HTTP response),
	// so we rebuild it from the headers the producer wrote.
	ctx := otel.GetTextMapPropagator().Extract(
		context.Background(), propagation.MapCarrier(msg.Headers))

	ctx, span := tracer.Start(ctx, msg.Topic+" process",
		trace.WithSpanKind(trace.SpanKindConsumer))
	defer span.End()

	span.SetAttributes(
		attribute.String("messaging.system", "inmemory"),
		attribute.String("messaging.destination.name", msg.Topic),
		attribute.String("messaging.operation", "process"),
	)

	outcome := "success"
	defer func() {
		messagesProcessed.Add(ctx, 1, metric.WithAttributes(
			attribute.String("messaging.destination.name", msg.Topic),
			attribute.String("outcome", outcome),
		))
	}()

	var order Order
	if err := json.Unmarshal(msg.Body, &order); err != nil {
		outcome = "error"
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		slog.ErrorContext(ctx, "bad order event", "error", err)
		return
	}

	// Same trace_id as the HTTP request's logs, even though this runs later
	// in another goroutine: ctx was rebuilt from the message headers.
	slog.InfoContext(ctx, "processing order event", "order_id", order.ID)
	sendConfirmationEmail(ctx, order)
}
