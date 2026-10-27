package tasktest

import (
	"context"

	"github.com/google/uuid"

	"github.com/jrgomesdev/miner/internal/task"
)

// NoRuns returns a task.RunGuard for tests that do not involve runs: no task
// ever has an active run.
func NoRuns() task.RunGuard { return noRuns{} }

type noRuns struct{}

func (noRuns) HasActiveRun(context.Context, uuid.UUID) (bool, error) { return false, nil }

func (noRuns) CancelActiveRun(context.Context, uuid.UUID) error { return nil }
