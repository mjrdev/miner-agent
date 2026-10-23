// Command api runs the miner HTTP service.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/jrgomesdev/miner/internal/config"
	"github.com/jrgomesdev/miner/internal/database"
	"github.com/jrgomesdev/miner/internal/handler"
	"github.com/jrgomesdev/miner/internal/logger"
	"github.com/jrgomesdev/miner/internal/server"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx); err != nil {
		stop()
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	cfg, err := config.Load()
	if err != nil {
		// The configured level is unknown here, so report with the default logger.
		logger.New(os.Stdout, slog.LevelInfo).Error("failed to load configuration", "error", err)
		return err
	}

	log := logger.New(os.Stdout, cfg.LogLevel)
	slog.SetDefault(log)
	log.Info("starting service", "env", cfg.AppEnv, "port", cfg.HTTPPort)
	defer log.Info("service stopped")

	pool, err := database.Connect(ctx, cfg.DatabaseURL, log)
	if err != nil {
		log.Error("failed to connect to database", "error", err)
		return err
	}
	defer func() {
		pool.Close()
		log.Info("database pool closed")
	}()

	router := server.NewRouter(handler.NewHealth(pool, log), log)
	srv := server.New(cfg.HTTPPort, router, log, cfg.ShutdownTimeout)

	if err := srv.Run(ctx); err != nil {
		log.Error("http server failed", "error", err)
		return err
	}
	return nil
}
