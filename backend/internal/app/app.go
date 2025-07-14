package app

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/Ravwvil/order-service/backend/internal/broker/kafka"
	"github.com/Ravwvil/order-service/backend/internal/config"
	"github.com/Ravwvil/order-service/backend/internal/service"
	redisClient "github.com/redis/go-redis/v9"
)

// DBer defines the interface for database operations needed by App.
type DBer interface {
	PingContext(ctx context.Context) error
	Close() error
}

// Rediser defines the interface for Redis operations needed by App.
type Rediser interface {
	Ping(ctx context.Context) *redisClient.StatusCmd
	Close() error
}

type App struct {
	logger        *slog.Logger
	server        *http.Server
	consumer      kafka.ConsumerInterface
	db            DBer
	redis         Rediser
	orderService  service.OrderServicer
	cfg           *config.Config
}

func NewApp(
	logger *slog.Logger,
	server *http.Server,
	orderService service.OrderServicer,
	db DBer,
	redis Rediser,
	consumer kafka.ConsumerInterface,
	cfg *config.Config,
) *App {
	return &App{
		logger:        logger,
		server:        server,
		orderService:  orderService,
		db:            db,
		redis:         redis,
		consumer:      consumer,
		cfg:           cfg,
	}
}

func (a *App) SetServer(server *http.Server) {
	a.server = server
}

func (a *App) Run(ctx context.Context) error {
	// Восстанавливаем кэш из базы данных при запуске
	if err := a.orderService.RestoreCache(ctx); err != nil {
		a.logger.Warn("failed to restore cache from database", "error", err)
		// Не возвращаем ошибку, чтобы приложение могло запуститься
	}

	// Запускаем Kafka consumer
	if err := a.consumer.Start(ctx); err != nil {
		return err
	}

	a.logger.Info("starting http server", slog.String("addr", a.server.Addr))
	return a.server.ListenAndServe()
}

func (a *App) Stop(ctx context.Context) error {
	a.logger.Info("shutting down application")

	// Останавливаем consumer
	if err := a.consumer.Stop(ctx); err != nil {
		a.logger.Error("error stopping kafka consumer", "error", err)
	}

	// Закрываем подключения
	if err := a.db.Close(); err != nil {
		a.logger.Error("error closing database connection", "error", err)
	}

	if err := a.redis.Close(); err != nil {
		a.logger.Error("error closing redis connection", "error", err)
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
