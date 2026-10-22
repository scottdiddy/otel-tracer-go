package main

import (
	"context"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"time"
)

func sendConfirmationEmail(ctx context.Context, o Order) {
	_, span := tracer.Start(ctx, "EmailService.Send",
		trace.WithSpanKind(trace.SpanKindClient))
	defer span.End()

	span.SetAttributes(attribute.String("order.id", o.ID))
	time.Sleep(80 * time.Millisecond) // pretend third-party call
}
