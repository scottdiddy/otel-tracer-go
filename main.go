// Demo: traces, metrics and logs flowing handler -> service -> repo -> queue -> worker.
//
// Setup (Go 1.26+):
//
//	docker compose up -d   # Collector, Jaeger, Prometheus, Loki, Grafana, Postgres
//	go run .
//	curl -X POST localhost:8081/orders -d '{"user_id":"u_42","amount":59.9}'
//	curl -X POST localhost:8081/orders -d '{"user_id":"u_42","amount":-1}'  # DB error
//
// Then open:
//
//	http://localhost:16686  Jaeger      traces for service "checkout_servicename"
//	http://localhost:9090   Prometheus  e.g. orders_created_total
//	http://localhost:3000   Grafana     Explore: Loki logs <-> Jaeger traces <-> metrics
package main

import (
	"cmp"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	shutdown, err := initTelemetry(ctx)
	if err != nil {
		fatal("init telemetry", err)
	}
	defer func() {
		// Flush buffered spans, metrics and logs before exiting, or the last ones are lost.
		c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = shutdown(c)
	}()

	// After initTelemetry, so the otelslog bridge sees the real LoggerProvider.
	slog.SetDefault(newLogger())

	repo, err := NewOrderRepo(ctx, cmp.Or(os.Getenv("DATABASE_URL"),
		"postgres://otel:otel@localhost:5434/orders?sslmode=disable"))
	if err != nil {
		fatal("connect db", err)
	}
	defer repo.Close()

	q := NewQueue()
	if err := initMetrics(q); err != nil {
		fatal("init metrics", err)
	}
	go (&Worker{q: q}).Run(ctx)

	h := &Handler{svc: &OrderService{repo: repo, queue: q}}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /orders", h.CreateOrder) // method patterns need Go 1.22+

	addr := cmp.Or(os.Getenv("ADDR"), ":8081")
	srv := &http.Server{
		Addr: addr,
		// Creates the root server span AND records http.server.request.duration
		// etc., since a MeterProvider is now registered.
		Handler: otelhttp.NewHandler(mux, "http.server"),
	}

	go func() {
		<-ctx.Done()
		sc, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(sc)
	}()

	slog.Info("listening", "addr", addr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		fatal("serve", err)
	}
}

// fatal logs and exits. os.Exit skips deferred calls, so telemetry still
// buffered at this point is not flushed.
func fatal(msg string, err error) {
	slog.Error(msg, "error", err)
	os.Exit(1)
}
