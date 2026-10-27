package postgres_test

import (
	"testing"

	"github.com/jrgomesdev/miner/internal/execution/run"
	runpg "github.com/jrgomesdev/miner/internal/execution/run/postgres"
	"github.com/jrgomesdev/miner/internal/execution/run/runtest"
	"github.com/jrgomesdev/miner/internal/task"
	taskpg "github.com/jrgomesdev/miner/internal/task/postgres"
	"github.com/jrgomesdev/miner/internal/testdb"
)

func TestRepositoryContract(t *testing.T) {
	runtest.Contract(t, func(t *testing.T) (task.Repository, run.Repository) {
		pool := testdb.Pool(t)
		return taskpg.NewRepository(pool), runpg.NewRepository(pool)
	})
}
