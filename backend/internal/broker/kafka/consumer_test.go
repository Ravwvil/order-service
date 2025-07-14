package kafka

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/Ravwvil/order-service/backend/internal/domain"
	"github.com/Ravwvil/order-service/backend/internal/service"
	"github.com/segmentio/kafka-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/network"
	"github.com/testcontainers/testcontainers-go/wait"
)

// MockOrderService мок для интерфейса OrderServicer.
type MockOrderService struct {
	mock.Mock
}

// ProcessOrderMessage мок для метода ProcessOrderMessage.
func (m *MockOrderService) ProcessOrderMessage(ctx context.Context, order *domain.Order) error {
	args := m.Called(ctx, order)
	return args.Error(0)
}

var (
	kafkaBroker string
	logger      *slog.Logger
)

const (
	testTopic    = "test-orders"
	dlqTopic     = "test-orders-dlq"
	testGroupID  = "test-consumer-group"
	networkName  = "kafka-test-network"
	zookeeperImg = "confluentinc/cp-zookeeper:7.2.1"
	kafkaImg     = "confluentinc/cp-kafka:7.2.1"
)

// loadOrderFromJSON вспомогательная функция для загрузки заказа из JSON-файла.
func loadOrderFromJSON(t *testing.T, path string) *domain.Order {
	t.Helper()
	data, err := os.ReadFile("../../service/testdata/valid_order.json")
	require.NoError(t, err, "failed to read order json")
	var order domain.Order
	require.NoError(t, json.Unmarshal(data, &order), "failed to unmarshal order")
	return &order
}

// createTopic создает топик Kafka для теста.
func createTopic(t *testing.T, broker, topicName string) {
	conn, err := kafka.Dial("tcp", broker)
	require.NoError(t, err)
	defer conn.Close()

	err = conn.CreateTopics(kafka.TopicConfig{
		Topic:             topicName,
		NumPartitions:     1,
		ReplicationFactor: 1,
	})
	require.NoError(t, err)
}

// produceMessage отправляет сообщение в топик Kafka для теста.
func produceMessage(t *testing.T, broker, topic string, order *domain.Order) {
	writer := &kafka.Writer{
		Addr:  kafka.TCP(broker),
		Topic: topic,
	}
	defer writer.Close()

	msgValue, err := json.Marshal(order)
	require.NoError(t, err)

	err = writer.WriteMessages(context.Background(), kafka.Message{
		Value: msgValue,
	})
	require.NoError(t, err)
}

// consumeDLQ читает сообщение из DLQ топика для теста.
func consumeDLQ(t *testing.T, broker, topic string) (*kafka.Message, error) {
	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:   []string{broker},
		Topic:     topic,
		Partition: 0,
		MaxBytes:  10e6, // 10MB
	})
	defer reader.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	msg, err := reader.ReadMessage(ctx)
	if err != nil {
		return nil, err
	}
	return &msg, nil
}

// TestMain настраивает тестовое окружение с контейнерами Kafka и Zookeeper.
func TestMain(m *testing.M) {
	ctx := context.Background()
	logger = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	net, err := network.New(ctx, network.WithCheckDuplicate())
	if err != nil {
		log.Fatalf("failed to create network: %s", err)
	}
	defer net.Remove(ctx)

	zookeeper, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        zookeeperImg,
			Hostname:     "zookeeper",
			ExposedPorts: []string{"2181/tcp"},
			Env: map[string]string{
				"ZOOKEEPER_CLIENT_PORT": "2181",
				"ZOOKEEPER_TICK_TIME":   "2000",
			},
			Networks: []string{net.Name},
		},
		Started: true,
	})
	if err != nil {
		log.Fatalf("failed to start zookeeper: %s", err)
	}
	defer zookeeper.Terminate(ctx)

	kafkaContainer, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        kafkaImg,
			Hostname:     "kafka",
			ExposedPorts: []string{"9093:9093"},
			Env: map[string]string{
				"KAFKA_BROKER_ID":                        "1",
				"KAFKA_ZOOKEEPER_CONNECT":                "zookeeper:2181",
				"KAFKA_LISTENER_SECURITY_PROTOCOL_MAP":   "PLAINTEXT:PLAINTEXT,PLAINTEXT_HOST:PLAINTEXT",
				"KAFKA_ADVERTISED_LISTENERS":             "PLAINTEXT://kafka:9092,PLAINTEXT_HOST://localhost:9093",
				"KAFKA_OFFSETS_TOPIC_REPLICATION_FACTOR": "1",
				"KAFKA_GROUP_INITIAL_REBALANCE_DELAY_MS": "0",
			},
			Networks:   []string{net.Name},
			WaitingFor: wait.ForLog("started"),
		},
		Started: true,
	})
	if err != nil {
		log.Fatalf("failed to start kafka: %s", err)
	}
	defer kafkaContainer.Terminate(ctx)

	kafkaBroker = "localhost:9093"

	os.Exit(m.Run())
}

// TestKafkaConsumer_Success тестирует успешную обработку сообщения Kafka.
func TestKafkaConsumer_Success(t *testing.T) {
	createTopic(t, kafkaBroker, testTopic)

	order := loadOrderFromJSON(t, "testdata/valid_order.json")
	var orderService service.OrderServicer = &MockOrderService{}
	mockOrderService := orderService.(*MockOrderService)
	mockOrderService.On("ProcessOrderMessage", mock.Anything, mock.AnythingOfType("*domain.Order")).Return(nil).Once()

	cfg := Config{
		Brokers:     []string{kafkaBroker},
		Topic:       testTopic,
		GroupID:     "success-group",
		Concurrency: 1,
	}
	consumer := NewConsumer(cfg, orderService, logger)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := consumer.Start(ctx)
	require.NoError(t, err)

	produceMessage(t, kafkaBroker, testTopic, order)

	assert.Eventually(t, func() bool {
		return len(mockOrderService.Calls) > 0
	}, 15*time.Second, 1*time.Second)

	mockOrderService.AssertExpectations(t)
	err = consumer.Stop(context.Background())
	assert.NoError(t, err)
}

// TestKafkaConsumer_RetryAndDLQ тестирует логику повторных попыток и DLQ в консьюмере Kafka.
func TestKafkaConsumer_RetryAndDLQ(t *testing.T) {
	createTopic(t, kafkaBroker, testTopic)
	createTopic(t, kafkaBroker, dlqTopic)

	order := loadOrderFromJSON(t, "testdata/valid_order.json")
	processingError := errors.New("processing failed")
	var orderService service.OrderServicer = &MockOrderService{}
	mockOrderService := orderService.(*MockOrderService)
	mockOrderService.On("ProcessOrderMessage", mock.Anything, mock.AnythingOfType("*domain.Order")).Return(processingError)

	cfg := Config{
		Brokers:           []string{kafkaBroker},
		Topic:             testTopic,
		GroupID:           "retry-group",
		Concurrency:       1,
		MaxRetries:        3,
		InitialRetryDelay: 100 * time.Millisecond,
		BackoffFactor:     1.5,
		MaxRetryDelay:     1 * time.Second,
		DLQTopic:          dlqTopic,
	}
	consumer := NewConsumer(cfg, orderService, logger)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := consumer.Start(ctx)
	require.NoError(t, err)

	produceMessage(t, kafkaBroker, testTopic, order)

	assert.Eventually(t, func() bool {
		return len(mockOrderService.Calls) >= cfg.MaxRetries
	}, 15*time.Second, 1*time.Second, "consumer did not retry expected number of times")

	mockOrderService.AssertExpectations(t)
	err = consumer.Stop(context.Background())
	assert.NoError(t, err)

	dlqMsg, err := consumeDLQ(t, kafkaBroker, dlqTopic)
	require.NoError(t, err)
	require.NotNil(t, dlqMsg)

	headers := make(map[string]string)
	for _, h := range dlqMsg.Headers {
		headers[h.Key] = string(h.Value)
	}
	assert.Equal(t, testTopic, headers["x-original-topic"])
	assert.Contains(t, headers["x-failure-reason"], "processing failed")
}
