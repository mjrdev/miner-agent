package run_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/jrgomesdev/miner/internal/execution/run"
	"github.com/jrgomesdev/miner/internal/execution/run/runtest"
	"github.com/jrgomesdev/miner/internal/task"
	"github.com/jrgomesdev/miner/internal/task/tasktest"
)

type fakePublisher struct {
	err  error
	sent []uuid.UUID
}

func (p *fakePublisher) Publish(_ context.Context, id uuid.UUID) error {
	if p.err != nil {
		return p.err
	}
	p.sent = append(p.sent, id)
	return nil
}

type env struct {
	tasks    *task.Service
	taskRepo *tasktest.Repository
	runs     *runtest.Repository
	svc      *run.Service
	pub      *fakePublisher
}

func newEnv(t *testing.T) *env {
	t.Helper()
	taskRepo := tasktest.NewRepository()
	runs := runtest.NewRepository(taskRepo)
	tasks := task.NewService(taskRepo, tasktest.Catalog(), run.NewGuard(runs))
	pub := &fakePublisher{}
	return &env{tasks: tasks, taskRepo: taskRepo, runs: runs, svc: run.NewService(runs, tasks, pub), pub: pub}
}

// readyTask creates a fully configured task and moves it to ready.
func (e *env) readyTask(t *testing.T) task.Task {
	t.Helper()
	ctx := context.Background()
	cat, err := e.tasks.CreateCategory(ctx, task.CategoryInput{
		Name: "Repo " + uuid.NewString(), RepoURL: new("https://github.com/org/repo"), BaseBranch: "develop",
	})
	if err != nil {
		t.Fatal(err)
	}
	created, err := e.tasks.CreateLocal(ctx, task.CreateInput{
		Title: "T", Description: "D", ProjectCategoryID: &cat.ID,
		Execution: task.Execution{Agent: new("claude"), Model: new("opus"), Effort: new("high"), Runner: new("host")},
	})
	if err != nil {
		t.Fatal(err)
	}
	ready, err := e.tasks.Update(ctx, created.ID, task.UpdateInput{Status: new(task.StatusReady)})
	if err != nil {
		t.Fatal(err)
	}
	return ready
}

func (e *env) taskStatus(t *testing.T, id uuid.UUID) task.Status {
	t.Helper()
	got, err := e.tasks.Get(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return got.Status
}

func TestStart(t *testing.T) {
	ctx := context.Background()

	t.Run("ready task", func(t *testing.T) {
		e := newEnv(t)
		tk := e.readyTask(t)
		r, err := e.svc.Start(ctx, tk.ID)
		if err != nil {
			t.Fatal(err)
		}
		want := run.Snapshot{
			Agent: "claude", Model: "opus", Effort: r.Effort, Runner: "host",
			RepoURL: "https://github.com/org/repo", BaseBranch: "develop",
			Branch: "miner/" + tk.ID.String() + "/" + r.ID.String(),
		}
		if r.Status != run.StatusQueued || r.Snapshot != want || *r.Effort != "high" {
			t.Errorf("run = %+v", r)
		}
		if len(e.pub.sent) != 1 || e.pub.sent[0] != r.ID {
			t.Errorf("published %v", e.pub.sent)
		}
		if s := e.taskStatus(t, tk.ID); s != task.StatusInProgress {
			t.Errorf("task status = %s", s)
		}
	})

	t.Run("task not ready", func(t *testing.T) {
		e := newEnv(t)
		tk, _ := e.tasks.CreateLocal(ctx, task.CreateInput{Title: "T", Description: "D"})
		if _, err := e.svc.Start(ctx, tk.ID); !errors.Is(err, task.ErrInvalidTransition) {
			t.Errorf("draft task = %v, want ErrInvalidTransition", err)
		}
		if _, err := e.svc.Start(ctx, uuid.New()); !errors.Is(err, task.ErrNotFound) {
			t.Errorf("missing task = %v, want ErrNotFound", err)
		}
	})

	t.Run("model removed from the catalog", func(t *testing.T) {
		e := newEnv(t)
		tk := e.readyTask(t)
		if _, err := e.taskRepo.UpdateTask(ctx, tk.ID, func(t *task.Task) error { t.Execution.Model = new("retired"); return nil }); err != nil {
			t.Fatal(err)
		}
		_, err := e.svc.Start(ctx, tk.ID)
		var ve *task.ValidationError
		if !errors.As(err, &ve) || ve.Field != "model" {
			t.Errorf("error = %v, want validation error on model", err)
		}
		if s := e.taskStatus(t, tk.ID); s != task.StatusReady {
			t.Errorf("task status = %s", s)
		}
	})

	t.Run("active run conflicts", func(t *testing.T) {
		e := newEnv(t)
		tk := e.readyTask(t)
		if _, err := e.svc.Start(ctx, tk.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := e.svc.Start(ctx, tk.ID); !errors.Is(err, task.ErrConflict) {
			t.Errorf("second start = %v, want ErrConflict", err)
		}
	})

	t.Run("queue unavailable", func(t *testing.T) {
		e := newEnv(t)
		tk := e.readyTask(t)
		e.pub.err = errors.New("broker: not connected")
		if _, err := e.svc.Start(ctx, tk.ID); !errors.Is(err, run.ErrUnavailable) {
			t.Errorf("error = %v, want ErrUnavailable", err)
		}
		if runs, _ := e.svc.ListByTask(ctx, tk.ID); len(runs) != 0 {
			t.Errorf("runs kept: %+v", runs)
		}
		if s := e.taskStatus(t, tk.ID); s != task.StatusReady {
			t.Errorf("task status = %s", s)
		}
	})
}

func TestCancel(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)

	queued, _ := e.svc.Start(ctx, e.readyTask(t).ID)
	got, err := e.svc.Cancel(ctx, queued.ID)
	if err != nil || got.Status != run.StatusCancelled || got.FinishedAt == nil {
		t.Fatalf("cancel queued = %+v, %v", got, err)
	}
	if s := e.taskStatus(t, queued.TaskID); s != task.StatusReady {
		t.Errorf("task status = %s", s)
	}
	if _, err := e.svc.Cancel(ctx, queued.ID); !errors.Is(err, task.ErrInvalidTransition) {
		t.Errorf("cancel finished run = %v, want ErrInvalidTransition", err)
	}

	running, _ := e.svc.Start(ctx, e.readyTask(t).ID)
	if _, err := e.runs.MarkRunning(ctx, running.ID, running.CreatedAt, "oauth"); err != nil {
		t.Fatal(err)
	}
	got, err = e.svc.Cancel(ctx, running.ID)
	if err != nil || got.Status != run.StatusRunning || got.CancelRequestedAt == nil {
		t.Errorf("cancel running = %+v, %v", got, err)
	}

	if _, err := e.svc.Cancel(ctx, uuid.New()); !errors.Is(err, task.ErrNotFound) {
		t.Errorf("cancel missing run = %v", err)
	}
}

func TestPrompt(t *testing.T) {
	p := run.Prompt(task.Task{Title: "Corrigir login", Description: "Implemente ...", AdditionalInstructions: []string{"passo 2", "passo 1"}})
	desc, second, first := strings.Index(p, "Implemente ..."), strings.Index(p, "1. passo 2"), strings.Index(p, "2. passo 1")
	if !strings.HasPrefix(p, "# Corrigir login") || desc < 0 || desc > second || second > first {
		t.Errorf("prompt order:\n%s", p)
	}
	if !strings.Contains(p, "Do not commit, push") {
		t.Errorf("prompt without the no-commit instruction:\n%s", p)
	}
	if strings.Contains(run.Prompt(task.Task{Title: "T", Description: "D"}), "Additional instructions") {
		t.Error("empty instructions section")
	}
}
