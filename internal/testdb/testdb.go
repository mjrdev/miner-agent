// Package testdb prepares the disposable PostgreSQL database used by
// integration tests.
package testdb

import (
	"context"
	"database/sql"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/jrgomesdev/miner/migrations"
)

// Pool connects to TEST_DATABASE_URL, applies the migrations and empties every
// table. The test is skipped when the variable is not set. The database must
// be disposable: its data is deleted. Packages that use it share the database,
// so run them one at a time (go test -p 1).
func Pool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()

	db, err := sql.Open("pgx", url)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	provider, err := goose.NewProvider(goose.DialectPostgres, db, migrations.FS)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.Up(ctx); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	if _, err := pool.Exec(ctx, "TRUNCATE task_labels, labels, tasks, project_categories"); err != nil {
		t.Fatalf("truncate tables: %v", err)
	}
	return pool
}
