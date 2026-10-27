package run

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/jrgomesdev/miner/internal/task"
)

// ErrUnavailable means the run could not be enqueued; nothing was kept.
var ErrUnavailable = errors.New("run queue unavailable")

// RunPublisher enqueues a run for the workers.
type RunPublisher interface {
	Publish(ctx context.Context, runID uuid.UUID) error
}

// Service starts, reads and cancels runs.
type Service struct {
	repo  Repository
	tasks *task.Service
	pub   RunPublisher
	now   func() time.Time
	newID func() uuid.UUID
}

// NewService returns a service that stores runs in repo, reads tasks through
// tasks and enqueues runs with pub.
func NewService(repo Repository, tasks *task.Service, pub RunPublisher) *Service {
	return &Service{
		repo:  repo,
		tasks: tasks,
		pub:   pub,
		now:   func() time.Time { return time.Now().UTC().Truncate(time.Microsecond) },
		newID: func() uuid.UUID { return uuid.Must(uuid.NewV7()) },
	}
}

// Start creates a queued run of a ready task, moves the task to in_progress
// and enqueues the run. The configuration is checked again, since the catalog
// or the category may have changed after the task became ready.
func (s *Service) Start(ctx context.Context, taskID uuid.UUID) (Run, error) {
	t, err := s.tasks.Get(ctx, taskID)
	if err != nil {
		return Run{}, err
	}
	if _, active, err := s.repo.Active(ctx, taskID); err != nil {
		return Run{}, err
	} else if active {
		return Run{}, fmt.Errorf("%w: task already has an active run", task.ErrConflict)
	}
	if err := s.tasks.CheckRunnable(t); err != nil {
		return Run{}, err
	}
	// The summary embedded in the task has no base branch.
	cat, err := s.tasks.GetCategory(ctx, t.ProjectCategory.ID)
	if err != nil {
		return Run{}, err
	}
	if cat.RepoURL == nil {
		return Run{}, &task.ValidationError{Field: "status", Msg: "task cannot run: missing project_category.repo_url"}
	}

	id := s.newID()
	e := t.Execution
	r := Run{
		ID:     id,
		TaskID: t.ID,
		Status: StatusQueued,
		Snapshot: Snapshot{
			Agent:      *e.Agent,
			Model:      *e.Model,
			Effort:     e.Effort,
			Runner:     *e.Runner,
			RepoURL:    *cat.RepoURL,
			BaseBranch: cat.BaseBranch,
			Branch:     BranchName(t.ID, id),
		},
		CreatedAt: s.now(),
	}
	if err := s.repo.Start(ctx, r); err != nil {
		return Run{}, err
	}
	if err := s.pub.Publish(ctx, id); err != nil {
		if abortErr := s.repo.Abort(context.WithoutCancel(ctx), id); abortErr != nil {
			return Run{}, fmt.Errorf("%w: %w (abort: %w)", ErrUnavailable, err, abortErr)
		}
		return Run{}, fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	return r, nil
}

// Get returns the run with id.
func (s *Service) Get(ctx context.Context, id uuid.UUID) (Run, error) {
	return s.repo.Get(ctx, id)
}

// ListByTask returns the runs of an existing task, newest first.
func (s *Service) ListByTask(ctx context.Context, taskID uuid.UUID) ([]Run, error) {
	if _, err := s.tasks.Get(ctx, taskID); err != nil {
		return nil, err
	}
	return s.repo.ListByTask(ctx, taskID)
}

// Cancel cancels an active run: a queued run ends now and its task goes back
// to ready; a running run is stopped by its worker, which then finishes it.
func (s *Service) Cancel(ctx context.Context, id uuid.UUID) (Run, error) {
	r, err := s.repo.Get(ctx, id)
	if err != nil {
		return Run{}, err
	}
	if !r.Status.Active() {
		return Run{}, fmt.Errorf("%w: run is %s", task.ErrInvalidTransition, r.Status)
	}
	if err := cancel(ctx, s.repo, r, s.now()); err != nil {
		return Run{}, err
	}
	return s.repo.Get(ctx, id)
}

// BranchName is the branch a run commits to.
func BranchName(taskID, runID uuid.UUID) string {
	return "miner/" + taskID.String() + "/" + runID.String()
}

// noCommitInstruction keeps the agent away from git history: the job commits
// the working tree itself when the agent finishes.
const noCommitInstruction = "Do not commit, push or create branches. Leave your changes in the working tree; " +
	"they are committed for you when you finish."

// Prompt is what the agent receives for t: the title, the description and
// the additional instructions in order.
func Prompt(t task.Task) string {
	var b strings.Builder
	b.WriteString("# " + t.Title + "\n\n" + t.Description + "\n")
	if len(t.AdditionalInstructions) > 0 {
		b.WriteString("\n## Additional instructions\n\n")
		for i, in := range t.AdditionalInstructions {
			b.WriteString(strconv.Itoa(i+1) + ". " + in + "\n")
		}
	}
	b.WriteString("\n---\n\n" + noCommitInstruction + "\n")
	return b.String()
}
