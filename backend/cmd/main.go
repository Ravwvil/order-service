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
	"github.com/Ravwvil/order-service/backend/internal/config"
)

func main() {
	// Инициализация конфига
	cfg, err := config.New()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
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

	a, err := app.New(ctx, cfg, logger)
	if err != nil {
		logger.Error("failed to create app", slog.Any("error", err))
		os.Exit(1)
	}

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