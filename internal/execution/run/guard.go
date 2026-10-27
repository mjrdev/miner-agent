package run

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/jrgomesdev/miner/internal/task"
)

// Guard implements task.RunGuard on top of a Repository.
type Guard struct {
	repo Repository
	now  func() time.Time
}

// NewGuard returns the task.RunGuard backed by repo.
func NewGuard(repo Repository) *Guard {
	return &Guard{repo: repo, now: func() time.Time { return time.Now().UTC().Truncate(time.Microsecond) }}
}

var _ task.RunGuard = (*Guard)(nil)

func (g *Guard) HasActiveRun(ctx context.Context, taskID uuid.UUID) (bool, error) {
	_, ok, err := g.repo.Active(ctx, taskID)
	return ok, err
}

func (g *Guard) CancelActiveRun(ctx context.Context, taskID uuid.UUID) error {
	r, ok, err := g.repo.Active(ctx, taskID)
	if err != nil || !ok {
		return err
	}
	return cancel(ctx, g.repo, r, g.now())
}

// cancel ends a queued run right away and asks the worker to stop a running
// one; the worker then finishes it.
func cancel(ctx context.Context, repo Repository, r Run, at time.Time) error {
	if r.Status == StatusQueued {
		_, err := repo.Finish(ctx, r.ID, StatusCancelled, Result{}, at)
		return err
	}
	_, err := repo.RequestCancel(ctx, r.ID, at)
	return err
}
