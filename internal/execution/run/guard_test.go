package run_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/jrgomesdev/miner/internal/execution/run"
	"github.com/jrgomesdev/miner/internal/execution/run/runtest"
	"github.com/jrgomesdev/miner/internal/task"
	"github.com/jrgomesdev/miner/internal/task/tasktest"
)

// readyTask stores a ready task directly in the repository.
func readyTask(t *testing.T, tasks *tasktest.Repository) uuid.UUID {
	t.Helper()
	id := uuid.Must(uuid.NewV7())
	now := time.Now().UTC()
	if err := tasks.InsertTask(context.Background(), task.Task{
		ID: id, Provider: task.ProviderLocal, ExternalID: id.String(), Title: "T", Description: "D",
		Status: task.StatusReady, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestGuard(t *testing.T) {
	ctx := context.Background()
	tasks := tasktest.NewRepository()
	runs := runtest.NewRepository(tasks)
	g := run.NewGuard(runs)

	queuedTask, runningTask := readyTask(t, tasks), readyTask(t, tasks)
	queued, running := runtest.NewRun(queuedTask, time.Now()), runtest.NewRun(runningTask, time.Now())
	for _, r := range []run.Run{queued, running} {
		if err := runs.Start(ctx, r); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := runs.MarkRunning(ctx, running.ID, time.Now(), "oauth"); err != nil {
		t.Fatal(err)
	}

	if ok, err := g.HasActiveRun(ctx, queuedTask); err != nil || !ok {
		t.Errorf("HasActiveRun(queued) = %v, %v", ok, err)
	}
	if ok, _ := g.HasActiveRun(ctx, uuid.New()); ok {
		t.Error("HasActiveRun(no runs) = true")
	}

	for _, id := range []uuid.UUID{queuedTask, runningTask, uuid.New()} {
		if err := g.CancelActiveRun(ctx, id); err != nil {
			t.Fatal(err)
		}
	}
	if got, _ := runs.Get(ctx, queued.ID); got.Status != run.StatusCancelled {
		t.Errorf("queued run = %s, want cancelled", got.Status)
	}
	got, _ := runs.Get(ctx, running.ID)
	if got.Status != run.StatusRunning || got.CancelRequestedAt == nil {
		t.Errorf("running run = %s, cancel requested %v", got.Status, got.CancelRequestedAt)
	}
}
