package main

import (
	"context"
	"log/slog"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

type OrderService struct {
	repo  *OrderRepo
	queue *Queue
}

func (s *OrderService) PlaceOrder(ctx context.Context, userID string, amount float64) (Order, error) {
	// Child span of the HTTP server span, because ctx carries the parent.
	ctx, span := tracer.Start(ctx, "OrderService.PlaceOrder")
	defer span.End()

	span.SetAttributes(
		attribute.String("user.id", userID),
		attribute.Float64("order.amount", amount),
	)
	// Passing ctx is what links this log line to the span above.
	slog.InfoContext(ctx, "placing order", "user_id", userID, "amount", amount)

	order, err := s.repo.Insert(ctx, userID, amount)
	if err != nil {
		return Order{}, s.fail(ctx, "db_insert", err)
	}

	if err := s.queue.Publish(ctx, "order.created", order); err != nil {
		return Order{}, s.fail(ctx, "queue_publish", err)
	}

	// Recorded with ctx, so these carry the request's trace context.
	ordersCreated.Add(ctx, 1)
	orderAmount.Record(ctx, amount)
	slog.InfoContext(ctx, "order placed", "order_id", order.ID)
	return order, nil
}

// fail reports one error on all three signals.
func (s *OrderService) fail(ctx context.Context, errType string, err error) error {
	span := trace.SpanFromContext(ctx)
	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())
	ordersFailed.Add(ctx, 1, metric.WithAttributes(attribute.String("error.type", errType)))
	slog.ErrorContext(ctx, "order failed", "error.type", errType, "error", err)
	return err
}
