package main

import (
	"context"
	"fmt"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"time"
)

type OrderRepo struct{}

func (r *OrderRepo) Insert(ctx context.Context, userID string, amount float64) (Order, error) {
	// Client kind: we are calling out to another system (the database).
	_, span := tracer.Start(ctx, "OrderRepo.Insert",
		trace.WithSpanKind(trace.SpanKindClient))
	defer span.End()

	span.SetAttributes(
		attribute.String("db.system", "postgresql"),
		attribute.String("db.operation", "INSERT"),
		attribute.String("db.sql.table", "orders"),
	)

	time.Sleep(30 * time.Millisecond) // pretend DB work; use otelsql / pgx tracer for real

	return Order{
		ID:     fmt.Sprintf("ord_%d", time.Now().UnixNano()),
		UserID: userID,
		Amount: amount,
	}, nil
}
