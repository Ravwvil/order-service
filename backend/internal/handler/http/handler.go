package http

import (
	"encoding/json"
	"net/http"

	"github.com/Ravwvil/order-service/backend/internal/config"
	"github.com/Ravwvil/order-service/backend/internal/service"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

type OrderHandler struct {
	orderService *service.OrderService
}

func NewOrderHandler(orderService *service.OrderService) *OrderHandler {
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

func NewServer(cfg config.HTTPConfig, orderHandler *OrderHandler) *http.Server {
	r := chi.NewRouter()

	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Get("/order/{order_uid}", orderHandler.GetOrderByUID)

	return &http.Server{
		Addr:    cfg.Addr,
		Handler: r,
	}
}
