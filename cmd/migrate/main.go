// Command migrate applies the SQL migrations in migrations/ to DATABASE_URL.
//
// Usage: migrate up | down | status
package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/jrgomesdev/miner/internal/logger"
	"github.com/jrgomesdev/miner/migrations"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	log := logger.New(os.Stdout, slog.LevelInfo)
	if err := run(ctx, os.Args[1:], log); err != nil {
		log.Error("migration failed", "error", err)
		stop()
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, log *slog.Logger) error {
	if len(args) != 1 {
		return errors.New("usage: migrate up | down | status")
	}

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		return errors.New("DATABASE_URL: required")
	}

	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		// The parse error may echo parts of the URL, so it is not wrapped.
		return errors.New("invalid DATABASE_URL")
	}
	defer db.Close()

	provider, err := goose.NewProvider(goose.DialectPostgres, db, migrations.FS)
	if err != nil {
		return fmt.Errorf("load migrations: %w", err)
	}

	switch args[0] {
	case "up":
		results, err := provider.Up(ctx)
		for _, r := range results {
			log.Info("migration applied", "version", r.Source.Version, "file", r.Source.Path, "duration", r.Duration)
		}
		if err != nil {
			return err
		}
		if len(results) == 0 {
			log.Info("no pending migrations")
		}
	case "down":
		r, err := provider.Down(ctx)
		if err != nil {
			return err
		}
		log.Info("migration rolled back", "version", r.Source.Version, "file", r.Source.Path)
	case "status":
		statuses, err := provider.Status(ctx)
		if err != nil {
			return err
		}
		for _, s := range statuses {
			log.Info("migration status", "version", s.Source.Version, "file", s.Source.Path, "state", s.State)
		}
	default:
		return fmt.Errorf("unknown command %q (use up, down or status)", args[0])
	}
	return nil
}
