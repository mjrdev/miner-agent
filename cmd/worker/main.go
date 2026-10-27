// Command worker executes the queued runs of tasks on the runners of the
// execution catalog. It is the only process with access to the Docker engine.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jrgomesdev/miner/internal/broker"
	"github.com/jrgomesdev/miner/internal/config"
	"github.com/jrgomesdev/miner/internal/database"
	"github.com/jrgomesdev/miner/internal/execution"
	"github.com/jrgomesdev/miner/internal/execution/claude"
	"github.com/jrgomesdev/miner/internal/execution/host"
	execrun "github.com/jrgomesdev/miner/internal/execution/run"
	runpg "github.com/jrgomesdev/miner/internal/execution/run/postgres"
	"github.com/jrgomesdev/miner/internal/execution/run/worker"
	"github.com/jrgomesdev/miner/internal/logger"
	"github.com/jrgomesdev/miner/internal/task/ingest"
	"github.com/jrgomesdev/miner/internal/task/postgres"
)

const (
	cancelPoll = 2 * time.Second
	retryDelay = time.Second
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
	cfg, err := config.LoadWorker()
	if err != nil {
		logger.New(os.Stdout, slog.LevelInfo).Error("failed to load configuration", "error", err)
		return err
	}
	log := logger.New(os.Stdout, cfg.LogLevel)
	slog.SetDefault(log)
	log.Info("starting worker", "max_concurrent_runs", cfg.MaxConcurrentRuns, "run_timeout", cfg.RunTimeout.String())
	defer log.Info("worker stopped")

	catalog := execution.NewCatalog([]execution.Agent{claude.New()}, []string{host.Name})
	for _, a := range catalog.Options().Agents {
		agent, _ := catalog.Agent(a.Name)
		if _, ok := agent.Credential(os.Getenv); !ok {
			log.Warn("no credentials configured for agent: its runs will fail", "agent", a.Name)
		}
	}

	docker, err := host.NewClient()
	if err != nil {
		log.Error("failed to create Docker client", "error", err)
		return err
	}
	defer func() { _ = docker.Close() }()
	runners := []execution.Runner{host.New(docker)}
	if err := checkRunners(catalog, runners); err != nil {
		log.Error("invalid runner setup", "error", err)
		return err
	}

	hostDir := cfg.RunsHostDir
	if hostDir == "" {
		if hostDir, err = host.RunsDirOnHost(ctx, docker, cfg.RunsDir); err != nil {
			log.Error("could not find the runs directory on the host; set RUNS_HOST_DIR", "error", err)
			return err
		}
	}
	log.Info("runs directory", "path", cfg.RunsDir, "host_path", hostDir)

	pool, err := database.Connect(ctx, cfg.DatabaseURL, log)
	if err != nil {
		log.Error("failed to connect to database", "error", err)
		return err
	}
	defer pool.Close()

	mq, err := broker.Connect(ctx, cfg.RabbitMQURL, broker.Setups(ingest.DeclareTopology, execrun.DeclareTopology), log)
	if err != nil {
		log.Error("failed to connect to message broker", "error", err)
		return err
	}
	defer func() { _ = mq.Close() }()

	w := worker.New(worker.Config{
		MaxConcurrent: cfg.MaxConcurrentRuns,
		Timeout:       cfg.RunTimeout,
		Image:         cfg.AgentImage,
		RunsDir:       cfg.RunsDir,
		RunsHostDir:   hostDir,
		GitToken:      cfg.GitToken,
		Getenv:        os.Getenv,
		CancelPoll:    cancelPoll,
		RetryDelay:    retryDelay,
	}, mq, runpg.NewRepository(pool), postgres.NewRepository(pool), catalog, runners, log)

	if err := w.Recover(ctx); err != nil {
		log.Error("failed to recover runs left by a previous worker", "error", err)
		return err
	}
	w.Start()
	<-ctx.Done()

	stopCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	if err := w.Stop(stopCtx); err != nil {
		log.Warn("runs still recording when the worker stopped; the next start recovers them", "error", err)
	}
	return nil
}

// checkRunners reports a catalog runner without an implementation.
func checkRunners(catalog *execution.Catalog, runners []execution.Runner) error {
	for _, name := range catalog.Options().Runners {
		found := false
		for _, r := range runners {
			found = found || r.Name() == name
		}
		if !found {
			return fmt.Errorf("runner %q of the catalog has no implementation", name)
		}
	}
	return nil
}
