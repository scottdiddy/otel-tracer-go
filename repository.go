package main

import (
	"context"

	"github.com/exaring/otelpgx"
	"github.com/jackc/pgx/v5/pgxpool"
)

type OrderRepo struct {
	db *pgxpool.Pool
}

// NewOrderRepo connects to Postgres with otelpgx wired in:
//   - the tracer makes a client span for every query (db.query.text, etc.),
//     as a child of whatever span is in ctx
//   - RecordStats exports pool metrics (connections in use, idle, waits...)
func NewOrderRepo(ctx context.Context, dsn string) (*OrderRepo, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, err
	}
	cfg.ConnConfig.Tracer = otelpgx.NewTracer()

	db, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	if err := otelpgx.RecordStats(db); err != nil {
		db.Close()
		return nil, err
	}

	// The CHECK constraint gives us a real DB error to look at:
	// POST a negative amount and watch the red span, error log and
	// orders.failed counter all line up.
	_, err = db.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS orders (
			id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
			user_id    text NOT NULL,
			amount     numeric(12,2) NOT NULL CHECK (amount > 0),
			created_at timestamptz NOT NULL DEFAULT now()
		)`)
	if err != nil {
		db.Close()
		return nil, err
	}
	return &OrderRepo{db: db}, nil
}

func (r *OrderRepo) Close() { r.db.Close() }

func (r *OrderRepo) Insert(ctx context.Context, userID string, amount float64) (Order, error) {
	// No manual span needed: otelpgx creates the client span for this query.
	o := Order{UserID: userID, Amount: amount}
	err := r.db.QueryRow(ctx,
		`INSERT INTO orders (user_id, amount) VALUES ($1, $2) RETURNING id::text`,
		userID, amount,
	).Scan(&o.ID)
	return o, err
}
