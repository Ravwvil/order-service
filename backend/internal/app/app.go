package app

import (
	"context"
	"log/slog"
	"net/http"
	"time"

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

type App struct {
	cfg          *config.Config
	log          *slog.Logger
	server       *http.Server
	consumer     *kafka.Consumer
	db           *sqlx.DB
	redis        *redisClient.Client
	orderService *service.OrderService
}

func New(ctx context.Context, cfg *config.Config, log *slog.Logger) (*App, error) {
	// Инициализация базы данных
	db, err := sqlx.Connect("postgres", cfg.Postgres.DSN())
	if err != nil {
		return nil, err
	}

	// Инициализация Redis
	rdb := redisClient.NewClient(&redisClient.Options{
		Addr:     cfg.Redis.Addr,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})

	// Проверка подключения к Redis
	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, err
	}

	// Инициализация репозиториев
	orderRepo := postgres.NewOrderRepository(db, log)

	// Инициализация кэша
	cache := redis.New(cfg.Redis.Addr, cfg.Redis.Password, cfg.Redis.DB, time.Duration(cfg.Redis.TTL)*time.Second, log)

	// Инициализация сервисов
	orderService := service.NewOrderService(orderRepo, cache, log)

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
	}
	consumer := kafka.NewConsumer(consumerCfg, orderService, log)

	app := &App{
		cfg:          cfg,
		log:          log,
		consumer:     consumer,
		db:           db,
		redis:        rdb,
		orderService: orderService,
	}

	// Инициализация HTTP обработчиков
	orderHandler := customhttp.NewOrderHandler(orderService)
	server := customhttp.NewServer(cfg.HTTP, orderHandler, app.Health)
	app.server = server

	return app, nil
}

func (a *App) Run(ctx context.Context) error {
	// Восстанавливаем кэш из базы данных при запуске
	if err := a.orderService.RestoreCache(ctx); err != nil {
		a.log.Warn("failed to restore cache from database", "error", err)
		// Не возвращаем ошибку, чтобы приложение могло запуститься
	}

	// Запускаем Kafka consumer
	if err := a.consumer.Start(ctx); err != nil {
		return err
	}

	a.log.Info("starting http server", slog.String("addr", a.cfg.HTTP.Addr))
	return a.server.ListenAndServe()
}

func (a *App) Stop(ctx context.Context) error {
	a.log.Info("shutting down application")

	// Останавливаем consumer
	if err := a.consumer.Stop(ctx); err != nil {
		a.log.Error("error stopping kafka consumer", "error", err)
	}

	// Закрываем подключения
	if err := a.db.Close(); err != nil {
		a.log.Error("error closing database connection", "error", err)
	}

	if err := a.redis.Close(); err != nil {
		a.log.Error("error closing redis connection", "error", err)
	}

	// Останавливаем HTTP сервер
	return a.server.Shutdown(ctx)
}

// Health проверяет состояние приложения
func (a *App) Health(ctx context.Context) error {
	// Проверяем подключение к базе данных
	if err := a.db.PingContext(ctx); err != nil {
		return err
	}

	// Проверяем подключение к Redis
	if err := a.redis.Ping(ctx).Err(); err != nil {
		return err
	}

	// Проверяем состояние consumer
	if err := a.consumer.Health(ctx); err != nil {
		return err
	}

	return nil
}
