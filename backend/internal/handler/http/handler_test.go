package http

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Ravwvil/order-service/backend/internal/domain"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// mockOrderService является моком для интерфейса OrderServicer
type mockOrderService struct {
	mock.Mock
}

// GetOrderByUID мокает метод GetOrderByUID
func (m *mockOrderService) GetOrderByUID(ctx context.Context, uid string) (*domain.Order, error) {
	args := m.Called(ctx, uid)
	var order *domain.Order
	if args.Get(0) != nil {
		order = args.Get(0).(*domain.Order)
	}
	return order, args.Error(1)
}

// getTestOrder возвращает тестовый экземпляр заказа.
func getTestOrder() *domain.Order {
	return &domain.Order{
		OrderUID: "test-uid",
		// Populate other fields if needed for more detailed tests
	}
}

// TestOrderHandler_GetOrderByUID тестирует обработчик GetOrderByUID.
func TestOrderHandler_GetOrderByUID(t *testing.T) {
	testOrder := getTestOrder()
	uid := testOrder.OrderUID

	t.Run("success", func(t *testing.T) {
		orderService := new(mockOrderService)
		orderService.On("GetOrderByUID", mock.Anything, uid).Return(testOrder, nil).Once()
		handler := NewOrderHandler(orderService)

		req := httptest.NewRequest(http.MethodGet, "/order/"+uid, nil)
		w := httptest.NewRecorder()

		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("order_uid", uid)
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		handler.GetOrderByUID(w, req)

		res := w.Result()
		defer res.Body.Close()
		data, err := io.ReadAll(res.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusOK, res.StatusCode)
		var receivedOrder domain.Order
		err = json.Unmarshal(data, &receivedOrder)
		require.NoError(t, err)
		assert.Equal(t, testOrder.OrderUID, receivedOrder.OrderUID)

		orderService.AssertExpectations(t)
	})

	t.Run("not found", func(t *testing.T) {
		orderService := new(mockOrderService)
		orderService.On("GetOrderByUID", mock.Anything, uid).Return(nil, errors.New("not found")).Once()
		handler := NewOrderHandler(orderService)

		req := httptest.NewRequest(http.MethodGet, "/order/"+uid, nil)
		w := httptest.NewRecorder()

		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("order_uid", uid)

		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		handler.GetOrderByUID(w, req)

		res := w.Result()
		defer res.Body.Close()
		assert.Equal(t, http.StatusNotFound, res.StatusCode)
		orderService.AssertExpectations(t)
	})

	t.Run("missing uid", func(t *testing.T) {
		orderService := new(mockOrderService)
		handler := NewOrderHandler(orderService)

		req := httptest.NewRequest(http.MethodGet, "/order/", nil) // No UID
		w := httptest.NewRecorder()

		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, chi.NewRouteContext()))

		handler.GetOrderByUID(w, req)

		res := w.Result()
		defer res.Body.Close()
		assert.Equal(t, http.StatusBadRequest, res.StatusCode)
		orderService.AssertNotCalled(t, "GetOrderByUID")
	})
}

// TestNewRouter_HealthCheck тестирует эндпоинт проверки состояния.
func TestNewRouter_HealthCheck(t *testing.T) {
	t.Run("healthy", func(t *testing.T) {
		healthCheck := func(ctx context.Context) error {
			return nil
		}
		router := NewRouter(nil, healthCheck)

		req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("unhealthy", func(t *testing.T) {
		healthCheck := func(ctx context.Context) error {
			return errors.New("db is down")
		}
		router := NewRouter(nil, healthCheck)

		req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	})
}
