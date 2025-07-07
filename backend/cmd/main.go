package main

import (
	"context"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/Ravwvil/order-service/backend/internal/app"
	"github.com/Ravwvil/order-service/backend/internal/config"
)

func main() {
	// Initialize configuration
	cfg, err := config.New()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	// Initialize logger
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

	// Create a new application
	a, err := app.New(context.Background(), cfg, logger)
	if err != nil {
		logger.Error("failed to create app", slog.Any("error", err))
		return
	}

	// Run the application
	go func() {
		if err := a.Run(); err != nil {
			logger.Error("error running app", slog.Any("error", err))
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("shutting down server...")

	if err := a.Stop(context.Background()); err != nil {
		logger.Error("error stopping app", slog.Any("error", err))
	}

	logger.Info("server gracefully stopped")
}
