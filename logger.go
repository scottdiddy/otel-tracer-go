package main

import (
	"context"
	"log/slog"

	"go.opentelemetry.io/otel/trace"
)

// logger returns a slog logger stamped with the current trace and span IDs,
// so every log line can be matched to its trace (and the reverse).
func logger(ctx context.Context) *slog.Logger {
	sc := trace.SpanContextFromContext(ctx)
	return slog.With(
		"trace_id", sc.TraceID().String(),
		"span_id", sc.SpanID().String(),
	)
}
