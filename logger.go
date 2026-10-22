package main

import (
	"context"
	"log/slog"
	"os"

	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/otel/trace"
)

// newLogger sends every slog record to two places:
//   - stdout as JSON, for humans tailing the process
//   - the OTel log pipeline (otelslog bridge -> Collector -> Loki)
//
// Use the *Context methods (slog.InfoContext(ctx, ...)). The bridge reads the
// active span from ctx and stamps the record with its trace and span IDs, so
// Grafana can jump from a log line to its trace in Jaeger and back.
func newLogger() *slog.Logger {
	return slog.New(slog.NewMultiHandler(
		traceIDHandler{slog.NewJSONHandler(os.Stdout, nil)},
		otelslog.NewHandler("checkout_loggername"),
	))
}

// traceIDHandler adds trace_id/span_id to stdout lines. The OTel bridge
// doesn't need this: it carries trace context as first-class fields.
type traceIDHandler struct {
	slog.Handler
}

func (h traceIDHandler) Handle(ctx context.Context, r slog.Record) error {
	if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
		r.AddAttrs(
			slog.String("trace_id", sc.TraceID().String()),
			slog.String("span_id", sc.SpanID().String()),
		)
	}
	return h.Handler.Handle(ctx, r)
}

func (h traceIDHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return traceIDHandler{h.Handler.WithAttrs(attrs)}
}

func (h traceIDHandler) WithGroup(name string) slog.Handler {
	return traceIDHandler{h.Handler.WithGroup(name)}
}
