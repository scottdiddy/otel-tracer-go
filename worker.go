package main

import (
	"context"
	"encoding/json"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
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

	var order Order
	if err := json.Unmarshal(msg.Body, &order); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return
	}

	logger(ctx).Info("processing order event", "order_id", order.ID)
	sendConfirmationEmail(ctx, order)
}
