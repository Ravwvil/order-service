package service

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"testing"

	"github.com/Ravwvil/order-service/backend/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

const (
	validOrderPath = "testdata/valid_order.json"
)

// Mocks
// MockOrderRepository мок для интерфейса OrderRepository.
type MockOrderRepository struct {
	mock.Mock
}

// Create мок для метода Create.
func (m *MockOrderRepository) Create(ctx context.Context, order *domain.Order) error {
	args := m.Called(ctx, order)
	return args.Error(0)
}

// GetByUID мок для метода GetByUID.
func (m *MockOrderRepository) GetByUID(ctx context.Context, uid string) (*domain.Order, error) {
	args := m.Called(ctx, uid)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Order), args.Error(1)
}

// GetAll мок для метода GetAll.
func (m *MockOrderRepository) GetAll(ctx context.Context) ([]*domain.Order, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*domain.Order), args.Error(1)
}

// MockOrderCache мок для интерфейса OrderCache.
type MockOrderCache struct {
	mock.Mock
}

// Set мок для метода Set.
func (m *MockOrderCache) Set(ctx context.Context, key string, order *domain.Order) {
	m.Called(ctx, key, order)
}

// Get мок для метода Get.
func (m *MockOrderCache) Get(ctx context.Context, key string) (*domain.Order, bool) {
	args := m.Called(ctx, key)
	if args.Get(0) == nil {
		return nil, args.Bool(1)
	}
	return args.Get(0).(*domain.Order), args.Bool(1)
}

// LoadFromDB мок для метода LoadFromDB.
func (m *MockOrderCache) LoadFromDB(ctx context.Context, orders map[string]*domain.Order) {
	m.Called(ctx, orders)
}

// Test Helpers
// loadOrderFromJSON вспомогательная функция для загрузки заказа из JSON-файла.
func loadOrderFromJSON(t *testing.T, path string) *domain.Order {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read file %s: %v", path, err)
	}
	var order domain.Order
	if err := json.Unmarshal(data, &order); err != nil {
		t.Fatalf("failed to unmarshal order: %v", err)
	}
	return &order
}

// newTestService вспомогательная функция для создания нового OrderService с мок-зависимостями.
func newTestService(repo *MockOrderRepository, cache *MockOrderCache) *OrderService {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	return NewOrderService(repo, cache, logger)
}

// Tests
// TestOrderService_GetOrderByUID тестирует метод GetOrderByUID.
func TestOrderService_GetOrderByUID(t *testing.T) {
	validOrder := loadOrderFromJSON(t, "validOrderPath")
	uid := validOrder.OrderUID

	t.Run("found in cache", func(t *testing.T) {
		repo := new(MockOrderRepository)
		cache := new(MockOrderCache)
		service := newTestService(repo, cache)

		cache.On("Get", mock.Anything, uid).Return(validOrder, true).Once()

		order, err := service.GetOrderByUID(context.Background(), uid)

		assert.NoError(t, err)
		assert.Equal(t, validOrder, order)
		cache.AssertExpectations(t)
		repo.AssertNotCalled(t, "GetByUID")
	})

	t.Run("found in repo", func(t *testing.T) {
		repo := new(MockOrderRepository)
		cache := new(MockOrderCache)
		service := newTestService(repo, cache)

		cache.On("Get", mock.Anything, uid).Return(nil, false).Once()
		repo.On("GetByUID", mock.Anything, uid).Return(validOrder, nil).Once()
		cache.On("Set", mock.Anything, uid, validOrder).Once()

		order, err := service.GetOrderByUID(context.Background(), uid)

		assert.NoError(t, err)
		assert.Equal(t, validOrder, order)
		cache.AssertExpectations(t)
		repo.AssertExpectations(t)
	})

	t.Run("not found", func(t *testing.T) {
		repo := new(MockOrderRepository)
		cache := new(MockOrderCache)
		service := newTestService(repo, cache)

		notFoundErr := errors.New("not found")
		cache.On("Get", mock.Anything, uid).Return(nil, false).Once()
		repo.On("GetByUID", mock.Anything, uid).Return(nil, notFoundErr).Once()

		_, err := service.GetOrderByUID(context.Background(), uid)

		assert.Error(t, err)
		assert.ErrorIs(t, err, notFoundErr)
		cache.AssertExpectations(t)
		repo.AssertExpectations(t)
		cache.AssertNotCalled(t, "Set")
	})
}

// TestOrderService_ProcessOrderMessage тестирует метод ProcessOrderMessage.
func TestOrderService_ProcessOrderMessage(t *testing.T) {
	validOrder := loadOrderFromJSON(t, "validOrderPath")

	t.Run("success", func(t *testing.T) {
		repo := new(MockOrderRepository)
		cache := new(MockOrderCache)
		service := newTestService(repo, cache)

		repo.On("Create", mock.Anything, validOrder).Return(nil).Once()
		cache.On("Set", mock.Anything, validOrder.OrderUID, validOrder).Once()

		err := service.ProcessOrderMessage(context.Background(), validOrder)

		assert.NoError(t, err)
		repo.AssertExpectations(t)
		cache.AssertExpectations(t)
	})

	t.Run("validation failed", func(t *testing.T) {
		repo := new(MockOrderRepository)
		cache := new(MockOrderCache)
		service := newTestService(repo, cache)
		invalidOrder := loadOrderFromJSON(t, "validOrderPath")
		invalidOrder.OrderUID = "" // Make it invalid

		err := service.ProcessOrderMessage(context.Background(), invalidOrder)

		assert.Error(t, err)
		repo.AssertNotCalled(t, "Create")
		cache.AssertNotCalled(t, "Set")
	})

	t.Run("repo create failed", func(t *testing.T) {
		repo := new(MockOrderRepository)
		cache := new(MockOrderCache)
		service := newTestService(repo, cache)
		repoErr := errors.New("db error")

		repo.On("Create", mock.Anything, validOrder).Return(repoErr).Once()

		err := service.ProcessOrderMessage(context.Background(), validOrder)

		assert.Error(t, err)
		assert.ErrorIs(t, err, repoErr)
		repo.AssertExpectations(t)
		cache.AssertNotCalled(t, "Set")
	})
}

// TestOrderService_RestoreCache тестирует метод RestoreCache.
func TestOrderService_RestoreCache(t *testing.T) {
	validOrder := loadOrderFromJSON(t, "validOrderPath")
	orders := []*domain.Order{validOrder}
	orderMap := map[string]*domain.Order{validOrder.OrderUID: validOrder}

	t.Run("success", func(t *testing.T) {
		repo := new(MockOrderRepository)
		cache := new(MockOrderCache)
		service := newTestService(repo, cache)

		repo.On("GetAll", mock.Anything).Return(orders, nil).Once()
		cache.On("LoadFromDB", mock.Anything, orderMap).Once()

		err := service.RestoreCache(context.Background())

		assert.NoError(t, err)
		repo.AssertExpectations(t)
		cache.AssertExpectations(t)
	})

	t.Run("get all from repo failed", func(t *testing.T) {
		repo := new(MockOrderRepository)
		cache := new(MockOrderCache)
		service := newTestService(repo, cache)
		repoErr := errors.New("db error")

		repo.On("GetAll", mock.Anything).Return(nil, repoErr).Once()

		err := service.RestoreCache(context.Background())

		assert.Error(t, err)
		assert.ErrorIs(t, err, repoErr)
		repo.AssertExpectations(t)
		cache.AssertNotCalled(t, "LoadFromDB")
	})
} 