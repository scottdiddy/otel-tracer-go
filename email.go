package main

import (
	"context"
	"math/rand/v2"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

func sendConfirmationEmail(ctx context.Context, o Order) {
	ctx, span := tracer.Start(ctx, "EmailService.Send",
		trace.WithSpanKind(trace.SpanKindClient))
	defer span.End()

	span.SetAttributes(attribute.String("order.id", o.ID))

	start := time.Now()
	// Pretend third-party call; jitter gives the histogram something to show.
	time.Sleep(time.Duration(50+rand.IntN(100)) * time.Millisecond)
	emailDuration.Record(ctx, time.Since(start).Seconds())
}
