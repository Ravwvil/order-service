package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"math/rand"
	"sync"
	"time"

	"github.com/Ravwvil/order-service/backend/internal/domain"
	"github.com/Ravwvil/order-service/backend/internal/service"
	"github.com/segmentio/kafka-go"
)

// Consumer (Kafka) для обработки заказов
type Consumer struct {
	reader       *kafka.Reader
	producer     *kafka.Writer // Для отправки в DLQ
	orderService *service.OrderService
	logger       *slog.Logger
	wg           *sync.WaitGroup

	// Конфигурация
	brokers           []string
	topic             string
	groupID           string
	maxRetries        int
	initialRetryDelay time.Duration
	maxRetryDelay     time.Duration
	backoffFactor     float64
	dlqTopic          string
}

// Config для Kafka consumer
type Config struct {
	Brokers           []string
	Topic             string
	GroupID           string
	MaxRetries        int
	InitialRetryDelay time.Duration
	MaxRetryDelay     time.Duration
	BackoffFactor     float64
	DLQTopic          string
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

	var producer *kafka.Writer
	if cfg.DLQTopic != "" {
		producer = &kafka.Writer{
			Addr:     kafka.TCP(cfg.Brokers...),
			Topic:    cfg.DLQTopic,
			Balancer: &kafka.LeastBytes{},
		}
	}

	consumer := &Consumer{
		reader:            reader,
		producer:          producer,
		orderService:      orderService,
		logger:            logger,
		wg:                &sync.WaitGroup{},
		brokers:           cfg.Brokers,
		topic:             cfg.Topic,
		groupID:           cfg.GroupID,
		maxRetries:        cfg.MaxRetries,
		initialRetryDelay: cfg.InitialRetryDelay,
		maxRetryDelay:     cfg.MaxRetryDelay,
		backoffFactor:     cfg.BackoffFactor,
		dlqTopic:          cfg.DLQTopic,
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
		slog.Duration("initial_retry_delay", c.initialRetryDelay),
		slog.Duration("max_retry_delay", c.maxRetryDelay),
		slog.Float64("backoff_factor", c.backoffFactor),
		slog.String("dlq_topic", c.dlqTopic))

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

	if c.producer != nil {
		if err := c.producer.Close(); err != nil {
			c.logger.Error("error closing kafka dlq producer", slog.String("error", err.Error()))
			return fmt.Errorf("failed to close kafka dlq producer: %w", err)
		}
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
			processingErr := c.processMessage(ctx, msg)
			if processingErr == nil {
				// Успешная обработка, коммитим
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
				continue
			}

			// Ошибка обработки
			c.logger.Error("error processing message, attempting to send to DLQ",
				slog.String("error", processingErr.Error()),
				slog.Int64("offset", msg.Offset),
				slog.Int("partition", msg.Partition))

			// Пытаемся отправить в DLQ
			if dlqErr := c.handleFailedMessage(ctx, msg, processingErr); dlqErr != nil {
				c.logger.Error("failed to send message to DLQ, message will be re-processed",
					slog.String("dlq_error", dlqErr.Error()),
					slog.Int64("offset", msg.Offset))
				// Не коммитим, позволяем kafka-go повторить доставку
				continue
			}

			// Успешно отправлено в DLQ, коммитим, чтобы не обрабатывать снова
			c.logger.Info("message sent to DLQ, committing offset", slog.Int64("offset", msg.Offset))
			if commitErr := c.reader.CommitMessages(ctx, msg); commitErr != nil {
				c.logger.Error("error committing message after sending to DLQ",
					slog.String("error", commitErr.Error()),
					slog.Int64("offset", msg.Offset))
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

// processOrderWithRetry обрабатывает заказ с механизмом повторных попыток и экспоненциальной задержкой
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
			delay := c.calculateBackoff(attempt)
			c.logger.Debug("waiting before retry",
				slog.String("order_uid", order.OrderUID),
				slog.Duration("delay", delay))

			select {
			case <-ctx.Done():
				c.logger.Debug("context cancelled during retry wait",
					slog.String("order_uid", order.OrderUID))
				return ctx.Err()
			case <-time.After(delay): // Продолжение после задержки
			}
		}
	}

	c.logger.Error("failed to process order after all retry attempts",
		slog.String("order_uid", order.OrderUID),
		slog.Int("max_retries", c.maxRetries),
		slog.String("error", lastErr.Error()))

	return fmt.Errorf("failed to process order after %d attempts: %w", c.maxRetries, lastErr)
}

func (c *Consumer) calculateBackoff(attempt int) time.Duration {
	if c.initialRetryDelay <= 0 || c.backoffFactor <= 1 || c.maxRetryDelay <= 0 {
		return c.initialRetryDelay // Fallback to simple retry delay
	}
	backoff := float64(c.initialRetryDelay) * math.Pow(c.backoffFactor, float64(attempt-1))
	delay := time.Duration(backoff)

	// Добавляем джиттер
	if delay > 0 {
		jitterMax := int64(delay) / 10
		if jitterMax > 0 {
			jitter := time.Duration(rand.Int63n(jitterMax))
			delay += jitter
		}
	}

	if delay > c.maxRetryDelay {
		delay = c.maxRetryDelay
	}

	return delay
}

// handleFailedMessage обрабатывает сообщение, которое не удалось обработать
func (c *Consumer) handleFailedMessage(ctx context.Context, msg kafka.Message, processingErr error) error {
	if c.producer == nil {
		c.logger.Warn("DLQ producer is not configured, message will be re-processed or lost",
			slog.Int64("offset", msg.Offset))
		return nil // Не возвращаем ошибку, чтобы не зацикливаться, если DLQ не настроен
	}

	c.logger.Info("sending message to DLQ",
		slog.String("dlq_topic", c.dlqTopic),
		slog.Int64("offset", msg.Offset))

	dlqMsg := kafka.Message{
		Key:   msg.Key,
		Value: msg.Value,
		Headers: []kafka.Header{
			{Key: "x-original-topic", Value: []byte(c.topic)},
			{Key: "x-original-offset", Value: []byte(fmt.Sprintf("%d", msg.Offset))},
			{Key: "x-original-partition", Value: []byte(fmt.Sprintf("%d", msg.Partition))},
			{Key: "x-failure-reason", Value: []byte(processingErr.Error())},
			{Key: "x-failed-at", Value: []byte(time.Now().UTC().Format(time.RFC3339))},
		},
	}

	err := c.producer.WriteMessages(ctx, dlqMsg)
	if err != nil {
		c.logger.Error("failed to write message to DLQ",
			slog.String("dlq_topic", c.dlqTopic),
			slog.String("error", err.Error()))
		return fmt.Errorf("failed to write to DLQ: %w", err)
	}

	c.logger.Info("message successfully sent to DLQ",
		slog.Int64("offset", msg.Offset),
		slog.String("dlq_topic", c.dlqTopic))
	return nil
}

// Health проверяет состояние Kafka consumer
func (c *Consumer) Health(ctx context.Context) error {
	// Проверяем подключение к Kafka через Dialer с контекстом
	dialer := &kafka.Dialer{}
	conn, err := dialer.DialContext(ctx, "tcp", c.brokers[0])
	if err != nil {
		return fmt.Errorf("kafka dial error: %w", err)
	}
	defer func() {
		if err := conn.Close(); err != nil {
			c.logger.Warn("error closing kafka connection in health check", slog.String("error", err.Error()))
		}
	}()

	// Проверяем lag на отрицательные значениях
	stats := c.reader.Stats()
	if stats.Lag < 0 {
		return fmt.Errorf("kafka reader lag negative: %d", stats.Lag)
	}

	return nil
}
