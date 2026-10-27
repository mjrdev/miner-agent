// Package runtest provides an in-memory run.Repository for tests and the
// contract every run repository must satisfy.
package runtest

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/jrgomesdev/miner/internal/execution/run"
	"github.com/jrgomesdev/miner/internal/task"
)

// Repository is an in-memory run.Repository that moves tasks through a
// task.Repository, as the SQL implementation does in its transactions.
type Repository struct {
	mu    sync.Mutex
	tasks task.Repository
	runs  map[uuid.UUID]run.Run
}

// NewRepository returns an empty repository whose runs belong to the tasks
// of tasks.
func NewRepository(tasks task.Repository) *Repository {
	return &Repository{tasks: tasks, runs: map[uuid.UUID]run.Run{}}
}

var _ run.Repository = (*Repository)(nil)

func (r *Repository) Start(ctx context.Context, rn run.Run) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	t, err := r.tasks.GetTask(ctx, rn.TaskID)
	if err != nil {
		return err
	}
	if t.Status != task.StatusReady {
		return fmt.Errorf("%w: task is %s, not ready", task.ErrInvalidTransition, t.Status)
	}
	if _, ok := r.active(rn.TaskID); ok {
		return fmt.Errorf("%w: task already has an active run", task.ErrConflict)
	}
	if err := r.moveTask(ctx, rn.TaskID, task.StatusReady, task.StatusInProgress, rn.CreatedAt); err != nil {
		return err
	}
	rn.Status = run.StatusQueued
	r.runs[rn.ID] = rn
	return nil
}

func (r *Repository) Abort(ctx context.Context, id uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	rn, ok := r.runs[id]
	if !ok || rn.Status != run.StatusQueued {
		return task.ErrNotFound
	}
	delete(r.runs, id)
	return r.moveTask(ctx, rn.TaskID, task.StatusInProgress, task.StatusReady, time.Now().UTC())
}

func (r *Repository) Get(ctx context.Context, id uuid.UUID) (run.Run, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rn, ok := r.runs[id]
	if !ok {
		return run.Run{}, task.ErrNotFound
	}
	// Runs go away with their task, as with ON DELETE CASCADE.
	if _, err := r.tasks.GetTask(ctx, rn.TaskID); err != nil {
		delete(r.runs, id)
		return run.Run{}, err
	}
	return rn, nil
}

func (r *Repository) ListByTask(_ context.Context, taskID uuid.UUID) ([]run.Run, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []run.Run{}
	for _, rn := range r.runs {
		if rn.TaskID == taskID {
			rn.Result.Diff = nil
			out = append(out, rn)
		}
	}
	slices.SortFunc(out, func(a, b run.Run) int { return b.CreatedAt.Compare(a.CreatedAt) })
	return out, nil
}

func (r *Repository) Active(_ context.Context, taskID uuid.UUID) (run.Run, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rn, ok := r.active(taskID)
	return rn, ok, nil
}

func (r *Repository) MarkRunning(_ context.Context, id uuid.UUID, at time.Time, authMethod string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rn, ok := r.runs[id]
	if !ok || rn.Status != run.StatusQueued {
		return false, nil
	}
	rn.Status, rn.StartedAt, rn.AuthMethod = run.StatusRunning, &at, &authMethod
	r.runs[id] = rn
	return true, nil
}

func (r *Repository) RequestCancel(_ context.Context, id uuid.UUID, at time.Time) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rn, ok := r.runs[id]
	if !ok || rn.Status != run.StatusRunning {
		return false, nil
	}
	if rn.CancelRequestedAt == nil {
		rn.CancelRequestedAt = &at
	}
	r.runs[id] = rn
	return true, nil
}

func (r *Repository) Finish(ctx context.Context, id uuid.UUID, status run.Status, res run.Result, at time.Time) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rn, ok := r.runs[id]
	if !ok || !rn.Status.Active() {
		return false, nil
	}
	rn.Status, rn.Result, rn.FinishedAt = status, res, &at
	r.runs[id] = rn
	t, err := r.tasks.GetTask(ctx, rn.TaskID)
	if err != nil {
		return true, err
	}
	if t.Status != task.StatusInProgress {
		return true, nil
	}
	return true, r.moveTask(ctx, rn.TaskID, task.StatusInProgress, run.TaskStatusAfter(status), at)
}

func (r *Repository) ListRunning(context.Context) ([]run.Run, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []run.Run{}
	for _, rn := range r.runs {
		if rn.Status == run.StatusRunning {
			out = append(out, rn)
		}
	}
	return out, nil
}

func (r *Repository) active(taskID uuid.UUID) (run.Run, bool) {
	for _, rn := range r.runs {
		if rn.TaskID == taskID && rn.Status.Active() {
			return rn, true
		}
	}
	return run.Run{}, false
}

// moveTask changes the status of a task that must be in from, bypassing the
// task rules as the SQL implementation does.
func (r *Repository) moveTask(ctx context.Context, id uuid.UUID, from, to task.Status, at time.Time) error {
	_, err := r.tasks.UpdateTask(ctx, id, func(t *task.Task) error {
		if t.Status != from {
			return fmt.Errorf("%w: task is not %s", task.ErrInvalidTransition, from)
		}
		t.Status, t.UpdatedAt = to, at
		return nil
	})
	return err
}
