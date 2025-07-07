package postgres

import (
	"context"

	"github.com/Ravwvil/order-service/backend/internal/domain"
	"github.com/jmoiron/sqlx"
)

type OrderRepository struct {
	db *sqlx.DB
}

func NewOrderRepository(db *sqlx.DB) *OrderRepository {
	return &OrderRepository{db: db}
}

func (r *OrderRepository) Create(ctx context.Context, order *domain.Order) error {
	// Implementation to create order in postgres
	return nil
}

func (r *OrderRepository) GetByUID(ctx context.Context, uid string) (*domain.Order, error) {
	// Implementation to get order by UID from postgres
	return nil, nil
}

func (r *OrderRepository) GetAll(ctx context.Context) ([]*domain.Order, error) {
	// Implementation to get all orders from postgres
	return nil, nil
}
