package http

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/Ravwvil/order-service/backend/internal/config"
	"github.com/Ravwvil/order-service/backend/internal/domain"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// OrderServicer defines the interface for the order service.
type OrderServicer interface {
	GetOrderByUID(ctx context.Context, uid string) (*domain.Order, error)
}

type OrderHandler struct {
	orderService OrderServicer
}

func NewOrderHandler(orderService OrderServicer) *OrderHandler {
	return &OrderHandler{
		orderService: orderService,
	}
}

func (h *OrderHandler) GetOrderByUID(w http.ResponseWriter, r *http.Request) {
	uid := chi.URLParam(r, "order_uid")
	if uid == "" {
		http.Error(w, "order_uid is required", http.StatusBadRequest)
		return
	}

	order, err := h.orderService.GetOrderByUID(r.Context(), uid)
	if err != nil {
		http.Error(w, "Order not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(order); err != nil {
		http.Error(w, "Failed to encode order", http.StatusInternalServerError)
	}
}

func NewRouter(orderHandler *OrderHandler, healthCheck func(ctx context.Context) error) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		if err := healthCheck(r.Context()); err != nil {
			http.Error(w, "health check failed", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	r.Get("/order/{order_uid}", orderHandler.GetOrderByUID)

	return r
}

func NewServer(cfg config.HTTPConfig, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:    cfg.Addr,
		Handler: handler,
	}
}
