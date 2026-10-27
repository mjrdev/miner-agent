package runtest

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/jrgomesdev/miner/internal/execution/run"
	"github.com/jrgomesdev/miner/internal/task"
)

// Contract checks the behavior every run.Repository must share. newRepos
// returns an empty task repository and the run repository backed by it.
func Contract(t *testing.T, newRepos func(t *testing.T) (task.Repository, run.Repository)) {
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	setup := func(t *testing.T, status task.Status) (task.Repository, run.Repository, run.Run) {
		t.Helper()
		tasks, runs := newRepos(t)
		id := uuid.Must(uuid.NewV7())
		err := tasks.InsertTask(ctx, task.Task{
			ID: id, Provider: task.ProviderLocal, ExternalID: id.String(), Title: "T", Description: "D",
			AdditionalInstructions: []string{}, Labels: []string{}, Status: status, CreatedAt: now, UpdatedAt: now,
		})
		if err != nil {
			t.Fatal(err)
		}
		return tasks, runs, NewRun(id, now)
	}
	taskStatus := func(t *testing.T, tasks task.Repository, id uuid.UUID) task.Status {
		t.Helper()
		got, err := tasks.GetTask(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		return got.Status
	}

	t.Run("start moves the task to in_progress", func(t *testing.T) {
		tasks, runs, r := setup(t, task.StatusReady)
		if err := runs.Start(ctx, r); err != nil {
			t.Fatal(err)
		}
		if s := taskStatus(t, tasks, r.TaskID); s != task.StatusInProgress {
			t.Errorf("task status = %s", s)
		}
		got, err := runs.Get(ctx, r.ID)
		if err != nil || got.Status != run.StatusQueued || got.Branch != r.Branch || *got.Effort != "high" {
			t.Errorf("stored run = %+v, %v", got, err)
		}
		if active, ok, err := runs.Active(ctx, r.TaskID); err != nil || !ok || active.ID != r.ID {
			t.Errorf("Active() = %v, %v, %v", active.ID, ok, err)
		}
	})

	t.Run("second active run conflicts", func(t *testing.T) {
		tasks, runs, r := setup(t, task.StatusReady)
		if err := runs.Start(ctx, r); err != nil {
			t.Fatal(err)
		}
		// Put the task back in ready, so only the active run blocks the start.
		if _, err := tasks.UpdateTask(ctx, r.TaskID, func(tk *task.Task) error { tk.Status = task.StatusReady; return nil }); err != nil {
			t.Fatal(err)
		}
		if err := runs.Start(ctx, NewRun(r.TaskID, now)); !errors.Is(err, task.ErrConflict) {
			t.Errorf("second start = %v, want ErrConflict", err)
		}
	})

	t.Run("start requires a ready task", func(t *testing.T) {
		tasks, runs, r := setup(t, task.StatusDraft)
		if err := runs.Start(ctx, r); !errors.Is(err, task.ErrInvalidTransition) {
			t.Errorf("start on draft = %v, want ErrInvalidTransition", err)
		}
		if _, err := runs.Get(ctx, r.ID); !errors.Is(err, task.ErrNotFound) {
			t.Errorf("run stored: %v", err)
		}
		if s := taskStatus(t, tasks, r.TaskID); s != task.StatusDraft {
			t.Errorf("task status = %s", s)
		}
		if err := runs.Start(ctx, NewRun(uuid.New(), now)); !errors.Is(err, task.ErrNotFound) {
			t.Errorf("start on missing task = %v, want ErrNotFound", err)
		}
	})

	t.Run("abort puts the task back in ready", func(t *testing.T) {
		tasks, runs, r := setup(t, task.StatusReady)
		if err := runs.Start(ctx, r); err != nil {
			t.Fatal(err)
		}
		if err := runs.Abort(ctx, r.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := runs.Get(ctx, r.ID); !errors.Is(err, task.ErrNotFound) {
			t.Errorf("aborted run still stored: %v", err)
		}
		if s := taskStatus(t, tasks, r.TaskID); s != task.StatusReady {
			t.Errorf("task status = %s", s)
		}
	})

	t.Run("finish follows the task status map", func(t *testing.T) {
		for _, st := range []run.Status{run.StatusSucceeded, run.StatusFailed, run.StatusCancelled} {
			tasks, runs, r := setup(t, task.StatusReady)
			if err := runs.Start(ctx, r); err != nil {
				t.Fatal(err)
			}
			if ok, err := runs.MarkRunning(ctx, r.ID, now, "oauth"); err != nil || !ok {
				t.Fatalf("MarkRunning = %v, %v", ok, err)
			}
			res := run.Result{Error: new("boom"), ExitCode: new(3), CostUSD: new(0.25), NumTurns: new(4), Diff: new("diff")}
			if ok, err := runs.Finish(ctx, r.ID, st, res, now); err != nil || !ok {
				t.Fatalf("Finish(%s) = %v, %v", st, ok, err)
			}
			if s := taskStatus(t, tasks, r.TaskID); s != run.TaskStatusAfter(st) {
				t.Errorf("%s: task status = %s", st, s)
			}
			got, _ := runs.Get(ctx, r.ID)
			if got.Status != st || *got.AuthMethod != "oauth" || *got.Result.CostUSD != 0.25 || *got.Result.Diff != "diff" || got.FinishedAt == nil {
				t.Errorf("%s: finished run = %+v", st, got)
			}
			if ok, _ := runs.Finish(ctx, r.ID, run.StatusFailed, run.Result{}, now); ok {
				t.Errorf("%s: finishing twice reported true", st)
			}
			if list, _ := runs.ListByTask(ctx, r.TaskID); len(list) != 1 || list[0].Result.Diff != nil {
				t.Errorf("%s: listing = %+v", st, list)
			}
		}
	})

	t.Run("finish keeps a task that left in_progress", func(t *testing.T) {
		tasks, runs, r := setup(t, task.StatusReady)
		if err := runs.Start(ctx, r); err != nil {
			t.Fatal(err)
		}
		if _, err := tasks.UpdateTask(ctx, r.TaskID, func(tk *task.Task) error { tk.Status = task.StatusCancelled; return nil }); err != nil {
			t.Fatal(err)
		}
		if ok, err := runs.Finish(ctx, r.ID, run.StatusCancelled, run.Result{}, now); err != nil || !ok {
			t.Fatalf("Finish = %v, %v", ok, err)
		}
		if s := taskStatus(t, tasks, r.TaskID); s != task.StatusCancelled {
			t.Errorf("task status = %s", s)
		}
	})

	t.Run("runs go away with their task", func(t *testing.T) {
		tasks, runs, r := setup(t, task.StatusReady)
		if err := runs.Start(ctx, r); err != nil {
			t.Fatal(err)
		}
		if _, err := runs.Finish(ctx, r.ID, run.StatusFailed, run.Result{}, now); err != nil {
			t.Fatal(err)
		}
		if err := tasks.DeleteTask(ctx, r.TaskID); err != nil {
			t.Fatal(err)
		}
		if _, err := runs.Get(ctx, r.ID); !errors.Is(err, task.ErrNotFound) {
			t.Errorf("run of a deleted task = %v, want ErrNotFound", err)
		}
	})

	t.Run("running state and cancellation requests", func(t *testing.T) {
		_, runs, r := setup(t, task.StatusReady)
		if err := runs.Start(ctx, r); err != nil {
			t.Fatal(err)
		}
		if ok, _ := runs.RequestCancel(ctx, r.ID, now); ok {
			t.Error("RequestCancel on a queued run reported true")
		}
		if ok, _ := runs.MarkRunning(ctx, r.ID, now, "api_key"); !ok {
			t.Fatal("MarkRunning = false")
		}
		if ok, _ := runs.MarkRunning(ctx, r.ID, now, "api_key"); ok {
			t.Error("MarkRunning twice reported true")
		}
		if ok, err := runs.RequestCancel(ctx, r.ID, now); err != nil || !ok {
			t.Errorf("RequestCancel = %v, %v", ok, err)
		}
		running, err := runs.ListRunning(ctx)
		if err != nil || len(running) != 1 || running[0].CancelRequestedAt == nil {
			t.Errorf("ListRunning = %+v, %v", running, err)
		}
	})
}

// NewRun returns a queued run of taskID with a complete snapshot.
func NewRun(taskID uuid.UUID, at time.Time) run.Run {
	id := uuid.Must(uuid.NewV7())
	return run.Run{
		ID:     id,
		TaskID: taskID,
		Status: run.StatusQueued,
		Snapshot: run.Snapshot{
			Agent: "claude", Model: "opus", Effort: new("high"), Runner: "host",
			RepoURL: "https://github.com/org/repo", BaseBranch: "main",
			Branch: "miner/" + taskID.String() + "/" + id.String(),
		},
		CreatedAt: at,
	}
}
