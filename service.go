package main

import (
	"context"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
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
	logger(ctx).Info("placing order", "user_id", userID)

	order, err := s.repo.Insert(ctx, userID, amount)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return Order{}, err
	}

	if err := s.queue.Publish(ctx, "order.created", order); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return Order{}, err
	}

	return order, nil
}
