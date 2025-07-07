package kafka

import (
	"context"
	"github.com/Ravwvil/order-service/backend/internal/service"
)

type Consumer struct {
	orderService *service.OrderService
}

func NewConsumer(orderService *service.OrderService) *Consumer {
	return &Consumer{
		orderService: orderService,
	}
}

func (c *Consumer) Start(ctx context.Context) {
	// Implementation to start kafka consumer
}
