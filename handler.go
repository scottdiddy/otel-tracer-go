package main

import (
	"encoding/json"
	"net/http"
)

type Handler struct {
	svc *OrderService
}

func (h *Handler) CreateOrder(w http.ResponseWriter, r *http.Request) {
	// otelhttp (wired in main) already created the server span and put it
	// in r.Context(). It also read any incoming traceparent header, so if a
	// caller upstream is traced, this span joins THEIR trace.
	ctx := r.Context()

	var req struct {
		UserID string  `json:"user_id"`
		Amount float64 `json:"amount"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	order, err := h.svc.PlaceOrder(ctx, req.UserID, req.Amount)
	if err != nil {
		http.Error(w, "could not place order", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(order) // the "back" part: response returns here
}
