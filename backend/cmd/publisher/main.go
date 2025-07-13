package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"os"
	"time"

	"github.com/Ravwvil/order-service/backend/internal/domain"
	"github.com/segmentio/kafka-go"
)

func getKafkaBroker() (string, error) {
	broker := os.Getenv("KAFKA_BROKERS")
	if broker == "" {
		return "", errors.New("KAFKA_BROKERS environment variable must be set")
	}
	return broker, nil
}

func main() {
	if err := run(); err != nil {
		log.Printf("ERROR: Publisher failed: %v", err)
		os.Exit(1)
	}
}

func run() error {
	broker, err := getKafkaBroker()
	if err != nil {
		return err
	}
	topic := "orders"

	log.Printf("Connecting to Kafka broker at %s", broker)

	conn, err := kafka.Dial("tcp", broker)
	if err != nil {
		return fmt.Errorf("failed to connect to Kafka broker: %w", err)
	}
	defer func() {
		if err := conn.Close(); err != nil {
			log.Printf("WARN: failed to close Kafka check connection: %v", err)
		}
	}()
	log.Println("Successfully connected to Kafka broker.")

	if err := createTopic(broker, topic); err != nil {
		return err
	}
	if err := createTopic(broker, "orders-dlq"); err != nil {
		return err
	}

	writer := &kafka.Writer{
		Addr:         kafka.TCP(broker),
		Topic:        topic,
		Balancer:     &kafka.LeastBytes{},
		RequiredAcks: kafka.RequireOne,
		Async:        false,
	}
	defer func() {
		if err := writer.Close(); err != nil {
			log.Printf("ERROR: failed to close kafka writer: %v", err)
		}
	}()

	log.Println("Generating mock orders...")
	orders := generateMockOrders(15)
	log.Printf("%d mock orders generated.", len(orders))

	fmt.Println("--- Published Order UIDs ---")
	for _, order := range orders {
		orderJSON, err := json.Marshal(order)
		if err != nil {
			log.Printf("ERROR: Failed to marshal order %s to JSON: %v", order.OrderUID, err)
			continue
		}

		err = writer.WriteMessages(context.Background(),
			kafka.Message{
				Key:   []byte(order.OrderUID),
				Value: orderJSON,
			},
		)
		if err != nil {
			log.Printf("ERROR: Failed to write message for order %s: %v", order.OrderUID, err)
		} else {
			fmt.Println(order.OrderUID)
		}
	}
	fmt.Println("--------------------------")
	log.Println("Finished sending mock orders.")
	return nil
}

func generateMockOrders(count int) []domain.Order {
	var orders []domain.Order
	cities := []string{"Moscow", "Kazan", "Innopolis", "Penza", "Krasnodar", "St. Petersburg", "Novosibirsk"}
	names := []string{"Ravil Kazeev", "Dmitriy Kuznetsov", "Vladimir Base", "Alexey Ivanov ", "Anna Petrova"}

	for i := 0; i < count; i++ {
		orderUID := generateRandomString(19)
		trackNumber := generateRandomString(13)
		now := time.Now()

		goodsTotal := rand.Intn(15000) + 500
		deliveryCost := rand.Intn(2000) + 500
		customFee := 0

		itemsCount := rand.Intn(4) + 1
		var items []domain.Item
		for j := 0; j < itemsCount; j++ {
			itemPrice := rand.Intn(4000) + 200
			item := domain.Item{
				ChrtID:      rand.Intn(1000000),
				TrackNumber: trackNumber,
				Price:       itemPrice,
				Rid:         generateRandomString(21),
				Name:        fmt.Sprintf("Item-%d", j+1),
				Sale:        rand.Intn(60),
				Size:        "0",
				TotalPrice:  itemPrice - (itemPrice * rand.Intn(30) / 100),
				NmID:        rand.Intn(5000000),
				Brand:       "Some Brand",
				Status:      202,
			}
			items = append(items, item)
		}

		order := domain.Order{
			OrderUID:    orderUID,
			TrackNumber: trackNumber,
			Entry:       "WBIL",
			Delivery: domain.Delivery{
				Name:    names[rand.Intn(len(names))],
				Phone:   fmt.Sprintf("+79%09d", rand.Intn(1000000000)),
				Zip:     fmt.Sprintf("%06d", rand.Intn(1000000)),
				City:    cities[rand.Intn(len(cities))],
				Address: fmt.Sprintf("Some Street %d", rand.Intn(100)+1),
				Region:  "Some Region",
				Email:   fmt.Sprintf("user%d@example.com", rand.Intn(10000)),
			},
			Payment: domain.Payment{
				Transaction:  orderUID,
				RequestID:    "",
				Currency:     "RUB",
				Provider:     "wbpay",
				Amount:       goodsTotal + deliveryCost + customFee,
				PaymentDt:    now.Unix(),
				Bank:         "sber",
				DeliveryCost: deliveryCost,
				GoodsTotal:   goodsTotal,
				CustomFee:    customFee,
			},
			Items:             items,
			Locale:            "ru",
			InternalSignature: "",
			CustomerID:        generateRandomString(10),
			DeliveryService:   "meest",
			ShardKey:          fmt.Sprintf("%d", rand.Intn(10)),
			SmID:              rand.Intn(100),
			DateCreated:       now,
			OofShard:          "1",
		}
		orders = append(orders, order)
	}
	return orders
}

func generateRandomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[rand.Intn(len(charset))]
	}
	return string(b)
}

func createTopic(broker, topicName string) error {
	conn, err := kafka.Dial("tcp", broker)
	if err != nil {
		return fmt.Errorf("failed to dial Kafka for topic creation: %w", err)
	}
	defer func() {
		if err := conn.Close(); err != nil {
			log.Printf("WARN: failed to close Kafka connection for topic creation: %v", err)
		}
	}()

	controller, err := conn.Controller()
	if err != nil {
		return fmt.Errorf("failed to get controller: %w", err)
	}

	var controllerConn *kafka.Conn
	controllerConn, err = kafka.Dial("tcp", fmt.Sprintf("%s:%d", controller.Host, controller.Port))
	if err != nil {
		return fmt.Errorf("failed to dial controller: %w", err)
	}
	defer func() {
		if err := controllerConn.Close(); err != nil {
			log.Printf("WARN: failed to close Kafka controller connection: %v", err)
		}
	}()

	topicConfigs := []kafka.TopicConfig{
		{
			Topic:             topicName,
			NumPartitions:     1,
			ReplicationFactor: 1,
		},
	}

	err = controllerConn.CreateTopics(topicConfigs...)
	if err != nil {
		if errors.Is(err, kafka.TopicAlreadyExists) {
			log.Printf("Topic '%s' already exists. Skipping creation.", topicName)
		} else {
			return fmt.Errorf("failed to create topic '%s': %w", topicName, err)
		}
	} else {
		log.Printf("Topic '%s' created successfully.", topicName)
	}
	return nil
}
