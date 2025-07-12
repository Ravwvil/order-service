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
	Brokers []string
	Topic   string
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
			Brokers: getEnvSlice("KAFKA_BROKERS", []string{"localhost:9092"}),
			Topic:   getEnv("KAFKA_TOPIC", "orders"),
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

func getEnvSlice(key string, defaultValue []string) []string {
	if value := os.Getenv(key); value != "" {
		return strings.Split(value, ",")
	}
	return defaultValue
}
