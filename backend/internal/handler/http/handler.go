package http

import (
	"net/http"

	"github.com/Ravwvil/order-service/backend/internal/config"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

type OrderHandler struct {
	// order service
}

func NewOrderHandler( /* order service */ ) *OrderHandler {
	return &OrderHandler{
		// order service
	}
}

func (h *OrderHandler) GetOrderByUID(w http.ResponseWriter, r *http.Request) {
	// Implementation to get order by UID
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
