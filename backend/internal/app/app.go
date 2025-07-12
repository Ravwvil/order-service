package app

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/Ravwvil/order-service/backend/internal/config"
	customhttp "github.com/Ravwvil/order-service/backend/internal/handler/http"
)

type App struct {
	cfg    *config.Config
	log    *slog.Logger
	server *http.Server
}

func New(ctx context.Context, cfg *config.Config, log *slog.Logger) (*App, error) {
	orderHandler := customhttp.NewOrderHandler(nil)
	server := customhttp.NewServer(cfg.HTTP, orderHandler)

	return &App{
		cfg:    cfg,
		log:    log,
		server: server,
	}, nil
}

func (a *App) Run() error {
	a.log.Info("starting http server", slog.String("addr", a.cfg.HTTP.Addr))
	return a.server.ListenAndServe()
}

func (a *App) Stop(ctx context.Context) error {
	return a.server.Shutdown(ctx)
}
