package redis

import (
	"context"
	"encoding/json"
	"log"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/Ravwvil/order-service/backend/internal/domain"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

var (
	redisClient *redis.Client
	redisCache  *Cache
)

// loadOrderFromJSON вспомогательная функция для загрузки заказа из JSON-файла.
func loadOrderFromJSON(t *testing.T, path string) *domain.Order {
	t.Helper()
	data, err := os.ReadFile("../../../internal/service/testdata/valid_order.json")
	if err != nil {
		t.Fatalf("failed to read file %s: %v", path, err)
	}
	var order domain.Order
	if err := json.Unmarshal(data, &order); err != nil {
		t.Fatalf("failed to unmarshal order: %v", err)
	}
	return &order
}

// TestMain настраивает контейнер Redis для тестов.
func TestMain(m *testing.M) {
	ctx := context.Background()

	req := testcontainers.ContainerRequest{
		Image:        "redis:7.2.5-alpine",
		ExposedPorts: []string{"6379/tcp"},
		WaitingFor:   wait.ForLog("Ready to accept connections"),
	}

	redisContainer, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		log.Fatalf("could not start redis container: %v", err)
	}
	defer func() {
		if err := redisContainer.Terminate(ctx); err != nil {
			log.Fatalf("failed to terminate container: %s", err.Error())
		}
	}()

	endpoint, err := redisContainer.Endpoint(ctx, "")
	if err != nil {
		log.Fatalf("failed to get container endpoint: %v", err)
	}

	redisClient = redis.NewClient(&redis.Options{
		Addr: endpoint,
	})
	if err := redisClient.Ping(ctx).Err(); err != nil {
		log.Fatalf("could not connect to redis: %v", err)
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	redisCache = New(endpoint, "", 0, 1*time.Hour, logger)

	code := m.Run()

	os.Exit(code)
}

// TestCache_SetAndGet тестирует установку и получение заказа из кэша.
func TestCache_SetAndGet(t *testing.T) {
	ctx := context.Background()
	order := loadOrderFromJSON(t, "testdata/valid_order.json")
	key := order.OrderUID

	redisClient.Del(ctx, "order:"+key)

	t.Run("Get miss", func(t *testing.T) {
		result, found := redisCache.Get(ctx, key)
		assert.False(t, found)
		assert.Nil(t, result)
	})

	t.Run("Set and Get hit", func(t *testing.T) {
		redisCache.Set(ctx, key, order)

		cachedOrder, found := redisCache.Get(ctx, key)
		assert.True(t, found)
		assert.NotNil(t, cachedOrder)

		expectedJSON, _ := json.Marshal(order)
		actualJSON, _ := json.Marshal(cachedOrder)
		assert.JSONEq(t, string(expectedJSON), string(actualJSON))

		ttl := redisClient.TTL(ctx, "order:"+key).Val()
		assert.Greater(t, ttl.Seconds(), float64(0))
		assert.LessOrEqual(t, ttl.Seconds(), float64(3600))
	})
}

func TestCache_LoadFromDB(t *testing.T) {
	ctx := context.Background()
	order1 := loadOrderFromJSON(t, "testdata/valid_order.json")
	order2 := loadOrderFromJSON(t, "testdata/valid_order.json") // Create another order
	order2.OrderUID = "another-test-uid"
	order2.Items[0].ChrtID = 12345

	ordersMap := map[string]*domain.Order{
		order1.OrderUID: order1,
		order2.OrderUID: order2,
	}

	redisClient.Del(ctx, "order:"+order1.OrderUID, "order:"+order2.OrderUID)

	redisCache.LoadFromDB(ctx, ordersMap)

	t.Run("verify order 1", func(t *testing.T) {
		cachedOrder, found := redisCache.Get(ctx, order1.OrderUID)
		assert.True(t, found)
		expectedJSON, _ := json.Marshal(order1)
		actualJSON, _ := json.Marshal(cachedOrder)
		assert.JSONEq(t, string(expectedJSON), string(actualJSON))
	})

	t.Run("verify order 2", func(t *testing.T) {
		cachedOrder, found := redisCache.Get(ctx, order2.OrderUID)
		assert.True(t, found)
		expectedJSON, _ := json.Marshal(order2)
		actualJSON, _ := json.Marshal(cachedOrder)
		assert.JSONEq(t, string(expectedJSON), string(actualJSON))
	})

	t.Run("load empty map", func(t *testing.T) {
		redisCache.LoadFromDB(ctx, make(map[string]*domain.Order))
	})
}

// TestCache_Close тестирует метод Close кэша.
func TestCache_Close(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	tempCache := New("localhost:6379", "", 0, 1*time.Hour, logger)
	err := tempCache.Close()
	assert.NoError(t, err)
}

// TestCache_Get_UnmarshalError тестирует поведение Get при столкновении с поврежденными данными в Redis.
func TestCache_Get_UnmarshalError(t *testing.T) {
	ctx := context.Background()
	key := "unmarshal-error-key"

	err := redisClient.Set(ctx, "order:"+key, "{invalid-json}", 1*time.Hour).Err()
	assert.NoError(t, err)

	order, found := redisCache.Get(ctx, key)
	assert.False(t, found)
	assert.Nil(t, order)
}
