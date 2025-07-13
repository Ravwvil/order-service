package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/Ravwvil/order-service/backend/internal/domain"
	"github.com/Ravwvil/order-service/backend/internal/service"
	"github.com/segmentio/kafka-go"
)

// Kafka consumer для обработки заказов
type Consumer struct {
	reader       *kafka.Reader
	orderService *service.OrderService
	logger       *slog.Logger
	wg           *sync.WaitGroup

	// Конфигурация
	brokers    []string
	topic      string
	groupID    string
	maxRetries int
	retryDelay time.Duration
}

// Config для Kafka consumer
type Config struct {
	Brokers    []string
	Topic      string
	GroupID    string
	MaxRetries int
	RetryDelay time.Duration
}

func NewConsumer(cfg Config, orderService *service.OrderService, logger *slog.Logger) *Consumer {
	logger.Debug("creating new kafka consumer",
		slog.String("topic", cfg.Topic),
		slog.String("group_id", cfg.GroupID),
		slog.Any("brokers", cfg.Brokers))

	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers: cfg.Brokers,
		Topic:   cfg.Topic,
		GroupID: cfg.GroupID,

		// Настройки производительности
		MinBytes: 1e3,  // 1KB
		MaxBytes: 10e6, // 10MB
		MaxWait:  1 * time.Second,

		// Автоматическое управление offset'ами
		StartOffset: kafka.FirstOffset,

		// Обработка ошибок
		ErrorLogger: kafka.LoggerFunc(func(msg string, args ...interface{}) {
			logger.Error("kafka reader error", slog.String("message", fmt.Sprintf(msg, args...)))
		}),

		// Настройки коммитов
		CommitInterval: 1 * time.Second,
	})

	consumer := &Consumer{
		reader:       reader,
		orderService: orderService,
		logger:       logger,
		wg:           &sync.WaitGroup{},
		brokers:      cfg.Brokers,
		topic:        cfg.Topic,
		groupID:      cfg.GroupID,
		maxRetries:   cfg.MaxRetries,
		retryDelay:   cfg.RetryDelay,
	}

	logger.Info("kafka consumer created successfully",
		slog.String("topic", cfg.Topic),
		slog.String("group_id", cfg.GroupID))

	return consumer
}

// Start запускает consumer для чтения сообщений из Kafka
func (c *Consumer) Start(ctx context.Context) error {
	c.logger.Info("starting kafka consumer",
		slog.String("topic", c.topic),
		slog.String("group_id", c.groupID),
		slog.Any("brokers", c.brokers),
		slog.Int("max_retries", c.maxRetries),
		slog.Duration("retry_delay", c.retryDelay))

	c.wg.Add(1)
	go c.consumeMessages(ctx)

	c.logger.Info("kafka consumer started successfully")
	return nil
}

// Stop останавливает consumer и ждет завершения обработки
func (c *Consumer) Stop(ctx context.Context) error {
	c.logger.Info("stopping kafka consumer")

	done := make(chan struct{})
	go func() {
		c.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		c.logger.Info("kafka consumer stopped gracefully")
	case <-ctx.Done():
		c.logger.Warn("kafka consumer stop timeout")
	}

	// Закрываем reader
	if err := c.reader.Close(); err != nil {
		c.logger.Error("error closing kafka reader", slog.String("error", err.Error()))
		return fmt.Errorf("failed to close kafka reader: %w", err)
	}

	c.logger.Info("kafka consumer stopped successfully")
	return nil
}

// consumeMessages основной цикл чтения сообщений
func (c *Consumer) consumeMessages(ctx context.Context) {
	defer c.wg.Done()

	for {
		select {
		case <-ctx.Done():
			c.logger.Info("consumer context cancelled, stopping")
			return
		default:
			// Читаем сообщение с контекстом и таймаутом
			msgCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			msg, err := c.reader.FetchMessage(msgCtx)
			cancel()

			if err != nil {
				if ctx.Err() != nil {
					// Контекст отменен, выходим
					c.logger.Debug("context cancelled while fetching message")
					return
				}
				c.logger.Error("error fetching message", slog.String("error", err.Error()))
				continue
			}

			c.logger.Debug("message fetched successfully",
				slog.Int64("offset", msg.Offset),
				slog.Int("partition", msg.Partition))

			// Обрабатываем сообщение
			if err := c.processMessage(ctx, msg); err != nil {
				c.logger.Error("error processing message",
					slog.String("error", err.Error()),
					slog.Int64("offset", msg.Offset),
					slog.Int("partition", msg.Partition))
				continue
			}

			// Коммитим сообщение после успешной обработки
			if err := c.reader.CommitMessages(ctx, msg); err != nil {
				c.logger.Error("error committing message",
					slog.String("error", err.Error()),
					slog.Int64("offset", msg.Offset),
					slog.Int("partition", msg.Partition))
			} else {
				c.logger.Debug("message committed successfully",
					slog.Int64("offset", msg.Offset),
					slog.Int("partition", msg.Partition))
			}
		}
	}
}

// processMessage обрабатывает отдельное сообщение
func (c *Consumer) processMessage(ctx context.Context, msg kafka.Message) error {
	c.logger.Debug("processing message",
		slog.Int64("offset", msg.Offset),
		slog.Int("partition", msg.Partition),
		slog.String("key", string(msg.Key)))

	// Парсим JSON сообщение
	var order domain.Order
	if err := json.Unmarshal(msg.Value, &order); err != nil {
		c.logger.Error("error unmarshaling order",
			slog.String("error", err.Error()),
			slog.String("value", string(msg.Value)))
		return fmt.Errorf("unmarshal order: %w", err)
	}

	c.logger.Debug("order unmarshaled successfully",
		slog.String("order_uid", order.OrderUID))

	// Обрабатываем заказ с повторными попытками
	return c.processOrderWithRetry(ctx, &order)
}

// processOrderWithRetry обрабатывает заказ с механизмом повторных попыток
func (c *Consumer) processOrderWithRetry(ctx context.Context, order *domain.Order) error {
	c.logger.Debug("starting order processing with retry",
		slog.String("order_uid", order.OrderUID),
		slog.Int("max_retries", c.maxRetries))

	var lastErr error

	for attempt := 1; attempt <= c.maxRetries; attempt++ {
		c.logger.Debug("attempting to process order",
			slog.String("order_uid", order.OrderUID),
			slog.Int("attempt", attempt))

		err := c.orderService.ProcessOrderMessage(ctx, order)
		if err == nil {
			c.logger.Info("order processed successfully",
				slog.String("order_uid", order.OrderUID),
				slog.Int("attempt", attempt))
			return nil
		}

		lastErr = err
		c.logger.Warn("order processing failed",
			slog.String("order_uid", order.OrderUID),
			slog.Int("attempt", attempt),
			slog.Int("max_retries", c.maxRetries),
			slog.String("error", err.Error()))

		if attempt < c.maxRetries {
			retryDelay := c.retryDelay
			c.logger.Debug("waiting before retry",
				slog.String("order_uid", order.OrderUID),
				slog.Duration("delay", retryDelay))

			select {
			case <-ctx.Done():
				c.logger.Debug("context cancelled during retry wait",
					slog.String("order_uid", order.OrderUID))
				return ctx.Err()
			case <-time.After(retryDelay): // Продолжение после задержки
			}
		}
	}

	c.logger.Error("failed to process order after all retry attempts",
		slog.String("order_uid", order.OrderUID),
		slog.Int("max_retries", c.maxRetries),
		slog.String("error", lastErr.Error()))

	return fmt.Errorf("failed to process order after %d attempts: %w", c.maxRetries, lastErr)
}

// Consumer Health проверяет состояние Kafka consumer
func (c *Consumer) Health(ctx context.Context) error {
	// Проверяем подключение к Kafka через Dialer с контекстом
	dialer := &kafka.Dialer{}
	conn, err := dialer.DialContext(ctx, "tcp", c.brokers[0])
	if err != nil {
		return fmt.Errorf("kafka dial error: %w", err)
	}
	defer conn.Close()

	// Проверяем lag на отрицательные значениях
	stats := c.reader.Stats()
	if stats.Lag < 0 {
		return fmt.Errorf("kafka reader lag negative: %d", stats.Lag)
	}

	return nil
}
