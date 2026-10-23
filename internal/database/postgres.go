// Package database manages the PostgreSQL connection pool.
package database

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	defaultConnectTimeout = 30 * time.Second
	defaultRetryInterval  = time.Second
	pingTimeout           = 2 * time.Second
)

// Pinger reports whether a dependency is reachable.
type Pinger interface {
	Ping(ctx context.Context) error
}

// Connect creates a connection pool for databaseURL and waits until the
// database answers a ping, retrying for up to 30 seconds.
func Connect(ctx context.Context, databaseURL string, log *slog.Logger) (*pgxpool.Pool, error) {
	return connect(ctx, databaseURL, log, defaultConnectTimeout, defaultRetryInterval)
}

func connect(ctx context.Context, databaseURL string, log *slog.Logger, timeout, interval time.Duration) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		// The parse error may echo parts of the URL, so it is not wrapped.
		return nil, fmt.Errorf("database: invalid DATABASE_URL")
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	for attempt := 1; ; attempt++ {
		pingCtx, pingCancel := context.WithTimeout(ctx, pingTimeout)
		err = pool.Ping(pingCtx)
		pingCancel()
		if err == nil {
			log.Info("database connected", "attempt", attempt)
			return pool, nil
		}

		log.Warn("database not ready, retrying", "attempt", attempt, "error", err)

		select {
		case <-ctx.Done():
			pool.Close()
			return nil, fmt.Errorf("database: could not connect within %s: %w", timeout, err)
		case <-time.After(interval):
		}
	}
}
