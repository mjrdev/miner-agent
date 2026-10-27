package runtest_test

import (
	"testing"

	"github.com/jrgomesdev/miner/internal/execution/run"
	"github.com/jrgomesdev/miner/internal/execution/run/runtest"
	"github.com/jrgomesdev/miner/internal/task"
	"github.com/jrgomesdev/miner/internal/task/tasktest"
)

func TestRepositoryContract(t *testing.T) {
	runtest.Contract(t, func(*testing.T) (task.Repository, run.Repository) {
		tasks := tasktest.NewRepository()
		return tasks, runtest.NewRepository(tasks)
	})
}
