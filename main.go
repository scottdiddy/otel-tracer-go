// Demo: one trace flowing handler -> service -> repo -> queue -> worker.
//
// Setup (Go 1.22+):
//
//	docker run --rm -p 16686:16686 -p 4317:4317 jaegertracing/all-in-one:latest
//	go mod tidy
//	go run .
//	curl -X POST localhost:8080/orders -d '{"user_id":"u_42","amount":59.9}'
//
// Then open http://localhost:16686 and look at service "checkout_v2".
package main

import (
	"context"
	"errors"
	"log"
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

	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	shutdown, err := initTracer(ctx)
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		// Flush buffered spans before exiting, or the last ones are lost.
		c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = shutdown(c)
	}()

	q := NewQueue()
	go (&Worker{q: q}).Run(ctx)

	h := &Handler{svc: &OrderService{repo: &OrderRepo{}, queue: q}}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /orders", h.CreateOrder) // method patterns need Go 1.22+

	srv := &http.Server{
		Addr:    ":8080",
		Handler: otelhttp.NewHandler(mux, "http.server"), // creates the root server span
	}

	go func() {
		<-ctx.Done()
		sc, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(sc)
	}()

	log.Println("listening on :8080")
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}
