// Command api runs the miner HTTP service.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/jrgomesdev/miner/internal/broker"
	"github.com/jrgomesdev/miner/internal/config"
	"github.com/jrgomesdev/miner/internal/database"
	"github.com/jrgomesdev/miner/internal/handler"
	"github.com/jrgomesdev/miner/internal/logger"
	"github.com/jrgomesdev/miner/internal/server"
	"github.com/jrgomesdev/miner/internal/task"
	"github.com/jrgomesdev/miner/internal/task/adapter"
	"github.com/jrgomesdev/miner/internal/task/adapter/local"
	"github.com/jrgomesdev/miner/internal/task/httpapi"
	"github.com/jrgomesdev/miner/internal/task/ingest"
	"github.com/jrgomesdev/miner/internal/task/postgres"
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

	mq, err := broker.Connect(ctx, cfg.RabbitMQURL, ingest.DeclareTopology, log)
	if err != nil {
		log.Error("failed to connect to message broker", "error", err)
		return err
	}
	defer func() {
		if err := mq.Close(); err != nil {
			log.Warn("closing message broker connection", "error", err)
		}
		log.Info("message broker connection closed")
	}()

	if cfg.LocalWebhookToken == "" {
		log.Warn("LOCAL_WEBHOOK_TOKEN is empty: the local webhook will reject every request")
	}
	registry := adapter.NewRegistry(local.New(cfg.LocalWebhookToken))
	tasks := task.NewService(postgres.NewRepository(pool))

	consumer := ingest.NewConsumer(mq, registry, tasks, log)
	consumer.Start()
	// Deferred after the broker and pool closes, so it runs before them: the
	// message in progress finishes while its connections are still open.
	defer func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancel()
		if err := consumer.Stop(stopCtx); err != nil {
			log.Warn("consumer did not stop in time, unacknowledged messages return to the queue", "error", err)
		}
	}()

	router := server.NewRouter(log,
		handler.NewHealth(map[string]database.Pinger{"database": pool, "broker": mq}, log),
		httpapi.NewWebhooks(registry, ingest.NewPublisher(mq), log),
		httpapi.NewTasks(tasks, log),
	)
	srv := server.New(cfg.HTTPPort, router, log, cfg.ShutdownTimeout)

	if err := srv.Run(ctx); err != nil {
		log.Error("http server failed", "error", err)
		return err
	}
	return nil
}
