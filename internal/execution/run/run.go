// Package run executes tasks: a run is one attempt of an agent at a task, in
// a disposable environment provided by a runner. It joins the task domain and
// the execution catalog, so it imports both.
package run

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/jrgomesdev/miner/internal/task"
)

// Status is the lifecycle state of a run.
type Status string

const (
	StatusQueued    Status = "queued"
	StatusRunning   Status = "running"
	StatusSucceeded Status = "succeeded"
	StatusFailed    Status = "failed"
	StatusCancelled Status = "cancelled"
)

// transitions lists, for each status, the statuses it may move to.
var transitions = map[Status][]Status{
	StatusQueued:    {StatusRunning, StatusFailed, StatusCancelled},
	StatusRunning:   {StatusSucceeded, StatusFailed, StatusCancelled},
	StatusSucceeded: nil,
	StatusFailed:    nil,
	StatusCancelled: nil,
}

// CanTransitionTo reports whether a run may move from s to to.
func (s Status) CanTransitionTo(to Status) bool {
	for _, allowed := range transitions[s] {
		if allowed == to {
			return true
		}
	}
	return false
}

// Active reports whether a run in s still holds its task.
func (s Status) Active() bool { return s == StatusQueued || s == StatusRunning }

// TaskStatusAfter is the status a task in progress moves to when its run ends
// in s.
func TaskStatusAfter(s Status) task.Status {
	switch s {
	case StatusSucceeded:
		return task.StatusInReview
	case StatusCancelled:
		return task.StatusReady
	default:
		return task.StatusFailed
	}
}

// Snapshot is the configuration a run was started with. Later edits of the
// task or its category do not change it.
type Snapshot struct {
	Agent      string
	Model      string
	Effort     *string
	Runner     string
	RepoURL    string
	BaseBranch string
	Branch     string
}

// Result is what a finished run reports; nil fields are unknown.
type Result struct {
	EffectiveModel *string
	CostUSD        *float64
	NumTurns       *int
	ExitCode       *int
	Error          *string
	Diff           *string
}

// Run is one execution of a task.
type Run struct {
	ID     uuid.UUID
	TaskID uuid.UUID
	Status Status
	Snapshot
	// AuthMethod is how the agent was authenticated, never the credential.
	AuthMethod        *string
	Result            Result
	CancelRequestedAt *time.Time
	CreatedAt         time.Time
	StartedAt         *time.Time
	FinishedAt        *time.Time
}

// Repository persists runs. Writes that change a run's status also move its
// task in the same transaction, so a run and its task never disagree.
//
// Implementations return task.ErrNotFound for a missing run or task,
// task.ErrConflict when the task already has an active run and
// task.ErrInvalidTransition when the task is not ready.
type Repository interface {
	// Start stores a queued run and moves its task from ready to in_progress.
	Start(ctx context.Context, r Run) error
	// Abort deletes a queued run that could not be enqueued and moves its
	// task back to ready.
	Abort(ctx context.Context, id uuid.UUID) error
	Get(ctx context.Context, id uuid.UUID) (Run, error)
	// ListByTask returns the runs of a task, newest first, without the diff.
	ListByTask(ctx context.Context, taskID uuid.UUID) ([]Run, error)
	// Active returns the active run of a task, if any.
	Active(ctx context.Context, taskID uuid.UUID) (Run, bool, error)
	// MarkRunning moves a queued run to running, reporting false when the run
	// is no longer queued.
	MarkRunning(ctx context.Context, id uuid.UUID, at time.Time, authMethod string) (bool, error)
	// RequestCancel records a cancellation request on a running run,
	// reporting false when the run is not running.
	RequestCancel(ctx context.Context, id uuid.UUID, at time.Time) (bool, error)
	// Finish ends an active run with status and result, reporting false when
	// the run had already ended. A task still in progress moves to
	// TaskStatusAfter(status).
	Finish(ctx context.Context, id uuid.UUID, status Status, res Result, at time.Time) (bool, error)
	// ListRunning returns every run in running.
	ListRunning(ctx context.Context) ([]Run, error)
}
