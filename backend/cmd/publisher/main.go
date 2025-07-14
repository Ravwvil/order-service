package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"os"
	"runtime"
	"sync"
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
		Async:        true,
		ErrorLogger: kafka.LoggerFunc(func(msg string, args ...interface{}) {
			log.Printf("KAFKA WRITER ERROR: "+msg, args...)
		}),
	}
	defer func() {
		if err := writer.Close(); err != nil {
			log.Printf("ERROR: failed to close kafka writer: %v", err)
		}
	}()

	log.Println("Generating mock orders...")
	orders := generateMockOrders(50)
	log.Printf("%d mock orders generated.", len(orders))

	fmt.Println("--- Published Order UIDs ---")

	var wg sync.WaitGroup
	numPublishWorkers := runtime.NumCPU()
	if len(orders) < numPublishWorkers {
		numPublishWorkers = len(orders)
	}
	if numPublishWorkers == 0 {
		numPublishWorkers = 1
	}

	orderChan := make(chan domain.Order, len(orders))

	for i := 0; i < numPublishWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for order := range orderChan {
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
		}()
	}

	for _, order := range orders {
		orderChan <- order
	}
	close(orderChan)

	wg.Wait()

	fmt.Println("--------------------------")
	log.Println("Finished sending mock orders.")
	return nil
}

func generateMockOrders(count int) []domain.Order {
	if count <= 0 {
		return []domain.Order{}
	}

	numWorkers := runtime.NumCPU()
	if count < numWorkers {
		numWorkers = count
	}

	jobs := make(chan int, count)
	results := make(chan domain.Order, count)
	cities := []string{"Moscow", "Kazan", "Innopolis", "Penza", "Krasnodar", "St. Petersburg", "Novosibirsk"}
	names := []string{"Ravil Kazeev", "Dmitriy Kuznetsov", "Vladimir Base", "Alexey Ivanov ", "Anna Petrova"}

	worker := func(jobs <-chan int, results chan<- domain.Order, workerID int) {
		r := rand.New(rand.NewSource(int64(workerID))) // устанавливаем разный seed для каждого инстанса

		for range jobs {
			orderUID := generateRandomString(r, 19)
			trackNumber := generateRandomString(r, 13)
			now := time.Now()

			goodsTotal := r.Intn(15000) + 500
			deliveryCost := r.Intn(2000) + 500
			customFee := 0

			itemsCount := r.Intn(4) + 1
			var items []domain.Item
			for j := 0; j < itemsCount; j++ {
				itemPrice := r.Intn(4000) + 200
				item := domain.Item{
					ChrtID:      r.Intn(1000000),
					TrackNumber: trackNumber,
					Price:       itemPrice,
					Rid:         generateRandomString(r, 21),
					Name:        fmt.Sprintf("Item-%d", j+1),
					Sale:        r.Intn(60),
					Size:        "0",
					TotalPrice:  itemPrice - (itemPrice * r.Intn(30) / 100),
					NmID:        r.Intn(5000000),
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
					Name:    names[r.Intn(len(names))],
					Phone:   fmt.Sprintf("+79%09d", r.Intn(1000000000)),
					Zip:     fmt.Sprintf("%06d", r.Intn(1000000)),
					City:    cities[r.Intn(len(cities))],
					Address: fmt.Sprintf("Some Street %d", r.Intn(100)+1),
					Region:  "Some Region",
					Email:   fmt.Sprintf("user%d@example.com", r.Intn(10000)),
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
				CustomerID:        generateRandomString(r, 10),
				DeliveryService:   "meest",
				ShardKey:          fmt.Sprintf("%d", r.Intn(10)),
				SmID:              r.Intn(100),
				DateCreated:       now,
				OofShard:          "1",
			}
			results <- order
		}
	}

	for w := 0; w < numWorkers; w++ {
		go worker(jobs, results, w)
	}

	for j := 0; j < count; j++ {
		jobs <- j
	}
	close(jobs)

	orders := make([]domain.Order, count)
	for a := 0; a < count; a++ {
		orders[a] = <-results
	}
	return orders
}

func generateRandomString(r *rand.Rand, length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[r.Intn(len(charset))]
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
