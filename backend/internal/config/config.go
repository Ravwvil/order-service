package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	LogLevel string
	HTTP     HTTPConfig
	Postgres PostgresConfig
	Kafka    KafkaConfig
	Redis    RedisConfig
}

type HTTPConfig struct {
	Addr string
}

type PostgresConfig struct {
	Host     string
	Port     int
	Database string
	User     string
	Password string
	SSLMode  string
}

func (c PostgresConfig) DSN() string {
	return fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		c.Host, c.Port, c.User, c.Password, c.Database, c.SSLMode,
	)
}

type KafkaConfig struct {
	Brokers           []string
	Topic             string
	GroupID           string
	MaxRetries        int
	InitialRetryDelay int     // в секундах
	MaxRetryDelay     int     // в секундах
	BackoffFactor     float64
	DLQTopic          string
}

type RedisConfig struct {
	Addr     string
	Password string
	DB       int
	TTL      int // в секундах
}

func New() (*Config, error) {
	cfg := &Config{
		LogLevel: getEnv("LOG_LEVEL", "info"),
		HTTP: HTTPConfig{
			Addr: getEnv("HTTP_ADDR", ":8081"),
		},
		Postgres: PostgresConfig{
			Host:     getEnv("POSTGRES_HOST", "localhost"),
			Port:     getEnvInt("POSTGRES_PORT", 5432),
			Database: getEnv("POSTGRES_DATABASE", "order_service"),
			User:     getEnv("POSTGRES_USER", "user"),
			Password: getEnv("POSTGRES_PASSWORD", "password"),
			SSLMode:  getEnv("POSTGRES_SSL_MODE", "disable"),
		},
		Kafka: KafkaConfig{
			Brokers:           getEnvSlice("KAFKA_BROKERS", []string{"localhost:9092"}),
			Topic:             getEnv("KAFKA_TOPIC", "orders"),
			GroupID:           getEnv("KAFKA_GROUP_ID", "order-service-consumer"),
			MaxRetries:        getEnvInt("KAFKA_MAX_RETRIES", 5),
			InitialRetryDelay: getEnvInt("KAFKA_INITIAL_RETRY_DELAY_S", 2),
			MaxRetryDelay:     getEnvInt("KAFKA_MAX_RETRY_DELAY_S", 60),
			BackoffFactor:     getEnvFloat("KAFKA_BACKOFF_FACTOR", 2.0),
			DLQTopic:          getEnv("KAFKA_DLQ_TOPIC", "orders-dlq"),
		},
		Redis: RedisConfig{
			Addr:     getEnv("REDIS_ADDR", "localhost:6379"),
			Password: getEnv("REDIS_PASSWORD", ""),
			DB:       getEnvInt("REDIS_DB", 0),
			TTL:      getEnvInt("REDIS_TTL", 3600),
		},
	}
	
	return cfg, nil
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

func getEnvFloat(key string, defaultValue float64) float64 {
	if value := os.Getenv(key); value != "" {
		if floatValue, err := strconv.ParseFloat(value, 64); err == nil {
			return floatValue
		}
	}
	return defaultValue
}

func getEnvSlice(key string, defaultValue []string) []string {
	if value := os.Getenv(key); value != "" {
		return strings.Split(value, ",")
	}
	return defaultValue
}
