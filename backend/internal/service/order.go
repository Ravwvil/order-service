package service

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/Ravwvil/order-service/backend/internal/domain"
)

type OrderRepository interface {
	Create(ctx context.Context, order *domain.Order) error
	GetByUID(ctx context.Context, uid string) (*domain.Order, error)
	GetAll(ctx context.Context) ([]*domain.Order, error)
}

type OrderCache interface {
	Set(ctx context.Context, key string, order *domain.Order)
	Get(ctx context.Context, key string) (*domain.Order, bool)
	LoadFromDB(ctx context.Context, orders map[string]*domain.Order)
}

type OrderService struct {
	repo   OrderRepository
	cache  OrderCache
	logger *slog.Logger
}

func NewOrderService(repo OrderRepository, cache OrderCache, logger *slog.Logger) *OrderService {
	return &OrderService{
		repo:   repo,
		cache:  cache,
		logger: logger,
	}
}

func (s *OrderService) GetOrderByUID(ctx context.Context, uid string) (*domain.Order, error) {
	s.logger.Debug("getting order by UID", slog.String("uid", uid))

	// Пытаемся получить из кэша
	if order, found := s.cache.Get(ctx, uid); found {
		s.logger.Debug("order found in cache", slog.String("uid", uid))
		return order, nil
	}

	// Если не найден в кэше, получаем из базы данных
	s.logger.Debug("order not found in cache, fetching from database", slog.String("uid", uid))
	order, err := s.repo.GetByUID(ctx, uid)
	if err != nil {
		s.logger.Error("failed to get order from database",
			slog.String("uid", uid),
			slog.String("error", err.Error()))
		return nil, fmt.Errorf("failed to get order: %w", err)
	}

	// Сохраняем в кэш для последующих запросов
	s.cache.Set(ctx, uid, order)
	s.logger.Debug("order cached successfully", slog.String("uid", uid))

	return order, nil
}

func (s *OrderService) ProcessOrderMessage(ctx context.Context, order *domain.Order) error {
	s.logger.Info("processing order message", slog.String("order_uid", order.OrderUID))

	// Валидируем заказ
	validationResult := order.Validate()
	if validationResult.HasErrors() {
		s.logger.Error("order validation failed",
			slog.String("order_uid", order.OrderUID),
			slog.Any("errors", validationResult.Errors))
		return fmt.Errorf("order validation failed: %w", validationResult.GetFirstError())
	}

	// Сохраняем в базу данных
	if err := s.repo.Create(ctx, order); err != nil {
		s.logger.Error("failed to save order to database",
			slog.String("order_uid", order.OrderUID),
			slog.String("error", err.Error()))
		return fmt.Errorf("failed to save order: %w", err)
	}

	// Сохраняем в кэш
	s.cache.Set(ctx, order.OrderUID, order)
	s.logger.Info("order processed successfully", slog.String("order_uid", order.OrderUID))

	return nil
}

func (s *OrderService) RestoreCache(ctx context.Context) error {
	s.logger.Info("starting cache restoration from database")

	// Получаем все заказы из базы данных
	orders, err := s.repo.GetAll(ctx)
	if err != nil {
		s.logger.Error("failed to get all orders from database", slog.String("error", err.Error()))
		return fmt.Errorf("failed to get orders from database: %w", err)
	}

	// Преобразуем в map для загрузки в кэш
	orderMap := make(map[string]*domain.Order, len(orders))
	for _, order := range orders {
		orderMap[order.OrderUID] = order
	}

	// Загружаем в кэш
	s.cache.LoadFromDB(ctx, orderMap)

	s.logger.Info("cache restoration completed", slog.Int("orders_count", len(orders)))
	return nil
}
