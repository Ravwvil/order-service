package main

import (
	"context"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Ravwvil/order-service/backend/internal/app"
	"github.com/Ravwvil/order-service/backend/internal/broker/kafka"
	"github.com/Ravwvil/order-service/backend/internal/cache/redis"
	"github.com/Ravwvil/order-service/backend/internal/config"
	customhttp "github.com/Ravwvil/order-service/backend/internal/handler/http"
	"github.com/Ravwvil/order-service/backend/internal/repository/postgres"
	"github.com/Ravwvil/order-service/backend/internal/service"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	redisClient "github.com/redis/go-redis/v9"
)

func main() {
	// Инициализация конфига
	cfg, err := config.New()
	if err != nil {
		log.Printf("failed to load config: %v", err)
		os.Exit(1)
	}

	// Инициализация логгера
	var lvl slog.Level
	switch cfg.LogLevel {
	case "debug":
		lvl = slog.LevelDebug
	case "info":
		lvl = slog.LevelInfo
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: lvl}))

	ctx := context.Background()

	// Инициализация базы данных
	db, err := sqlx.Connect("postgres", cfg.Postgres.DSN())
	if err != nil {
		logger.Error("failed to connect to postgres", slog.Any("error", err))
		os.Exit(1)
	}

	// Инициализация Redis
	rdb := redisClient.NewClient(&redisClient.Options{
		Addr:     cfg.Redis.Addr,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})

	// Проверка подключения к Redis
	if err := rdb.Ping(ctx).Err(); err != nil {
		logger.Error("failed to connect to redis", slog.Any("error", err))
		os.Exit(1)
	}

	// Инициализация репозиториев
	orderRepo := postgres.NewOrderRepository(db, logger)

	// Инициализация кэша
	cache := redis.New(cfg.Redis.Addr, cfg.Redis.Password, cfg.Redis.DB, time.Duration(cfg.Redis.TTL)*time.Second, logger)

	// Инициализация сервисов
	orderService := service.NewOrderService(orderRepo, cache, logger)

	// Инициализация Kafka consumer
	consumerCfg := kafka.Config{
		Brokers:           cfg.Kafka.Brokers,
		Topic:             cfg.Kafka.Topic,
		GroupID:           cfg.Kafka.GroupID,
		MaxRetries:        cfg.Kafka.MaxRetries,
		InitialRetryDelay: time.Duration(cfg.Kafka.InitialRetryDelay) * time.Second,
		MaxRetryDelay:     time.Duration(cfg.Kafka.MaxRetryDelay) * time.Second,
		BackoffFactor:     cfg.Kafka.BackoffFactor,
		DLQTopic:          cfg.Kafka.DLQTopic,
		Concurrency:       cfg.Kafka.Concurrency,
	}
	consumer := kafka.NewConsumer(consumerCfg, orderService, logger)

	// Инициализация HTTP обработчиков и сервера
	orderHandler := customhttp.NewOrderHandler(orderService)

	a := app.NewApp(logger, nil, orderService, db, rdb, consumer, cfg)

	// Теперь, когда у нас есть `a` с методом Health, мы можем создать роутер
	router := customhttp.NewRouter(orderHandler, a.Health)
	server := customhttp.NewServer(cfg.HTTP, router)
	a.SetServer(server)

	// Настройка graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	// Запуск приложения в горутине
	appErr := make(chan error, 1)
	go func() {
		logger.Info("starting application")
		if err := a.Run(ctx); err != nil {
			logger.Error("error running app", slog.Any("error", err))
			appErr <- err
		}
	}()

	select {
	case <-quit:
		logger.Info("received shutdown signal")
	case err := <-appErr:
		logger.Error("application error", slog.Any("error", err))
	}

	logger.Info("shutting down server...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := a.Stop(shutdownCtx); err != nil {
		logger.Error("error stopping app", slog.Any("error", err))
		os.Exit(1)
	}

	logger.Info("server gracefully stopped")
}