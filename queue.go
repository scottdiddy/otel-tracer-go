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

type Message struct {
	Topic   string
	Body    []byte
	Headers map[string]string // like Kafka headers / SQS message attributes
}

type Queue struct{ ch chan Message }

func NewQueue() *Queue { return &Queue{ch: make(chan Message, 100)} }

func (q *Queue) Publish(ctx context.Context, topic string, v any) error {
	ctx, span := tracer.Start(ctx, topic+" publish",
		trace.WithSpanKind(trace.SpanKindProducer))
	defer span.End()

	span.SetAttributes(
		attribute.String("messaging.system", "inmemory"),
		attribute.String("messaging.destination.name", topic),
		attribute.String("messaging.operation", "publish"),
	)

	body, err := json.Marshal(v)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	// Write traceparent (and baggage) into the message headers.
	headers := map[string]string{}
	otel.GetTextMapPropagator().Inject(ctx, propagation.MapCarrier(headers))

	select {
	case q.ch <- Message{Topic: topic, Body: body, Headers: headers}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
