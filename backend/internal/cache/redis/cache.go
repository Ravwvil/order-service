package redis

import (
	"context"
	"encoding/json"
	"time"
	"log/slog"

	"github.com/Ravwvil/order-service/backend/internal/domain"
	"github.com/redis/go-redis/v9"
)

// Redis кэш для заказов
type Cache struct {
	client *redis.Client
	ttl    time.Duration
	logger *slog.Logger
}

// New создает новый экземпляр Redis кэша
func New(addr, password string, db int, ttl time.Duration, log *logger.Logger) *Cache {
	rdb := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	})

	return &Cache{
		client: rdb,
		ttl:    ttl,
		logger: log,
	}
}

// Set сохраняет заказ в кэше
func (c *Cache) Set(ctx context.Context, key string, order *domain.Order) {
	data, err := json.Marshal(order)
	if err != nil {
		c.logger.Error("Failed to marshal order for Redis cache",
			logger.String("key", key),
			logger.Error(err),
		)
		return
	}

	// Используем переданный контекст с таймаутом
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	err = c.client.Set(ctx, "order:"+key, data, c.ttl).Err()
	if err != nil {
		c.logger.Error("Failed to set order in Redis cache",
			logger.String("key", key),
			logger.Error(err),
		)
	} else {
		c.logger.Debug("Order saved to Redis cache",
			logger.String("key", key),
		)
	}
}

// Get получает заказ из кэша
func (c *Cache) Get(ctx context.Context, key string) (*domain.Order, bool) {
	data, err := c.client.Get(ctx, "order:"+key).Result()
	if err != nil {
		if err == redis.Nil {
			c.logger.Debug("Order not found in Redis cache",
				logger.String("key", key),
			)
		} else {
			c.logger.Error("Failed to get order from Redis cache",
				logger.String("key", key),
				logger.Error(err),
			)
		}
		return nil, false
	}

	var order domain.Order
	if err := json.Unmarshal([]byte(data), &order); err != nil {
		c.logger.Error("Failed to unmarshal order from Redis cache",
			logger.String("key", key),
			logger.Error(err),
		)
		return nil, false
	}

	c.logger.Debug("Order retrieved from Redis cache",
		logger.String("key", key),
	)
	return &order, true
}

// LoadFromDB загружает данные из БД в кэш
func (c *Cache) LoadFromDB(ctx context.Context, orders map[string]*domain.Order) {
	c.logger.Info("Loading orders from database to Redis cache",
		logger.Int("count", len(orders)),
	)
	
	for key, order := range orders {
		c.Set(ctx, key, order)
	}
	
	c.logger.Info("Finished loading orders from database to Redis cache",
		logger.Int("count", len(orders)),
	)
}

// Close закрывает соединение
func (c *Cache) Close() error {
	return c.client.Close()
}
