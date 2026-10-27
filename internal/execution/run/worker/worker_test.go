package worker

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/jrgomesdev/miner/internal/execution"
	"github.com/jrgomesdev/miner/internal/execution/run"
	"github.com/jrgomesdev/miner/internal/execution/run/runtest"
	"github.com/jrgomesdev/miner/internal/task"
	"github.com/jrgomesdev/miner/internal/task/tasktest"
)

type fakeAck struct {
	mu                     sync.Mutex
	acked, nacked, requeue bool
}

func (a *fakeAck) Ack(uint64, bool) error { a.mu.Lock(); a.acked = true; a.mu.Unlock(); return nil }
func (a *fakeAck) Nack(_ uint64, _ bool, requeue bool) error {
	a.mu.Lock()
	a.nacked, a.requeue = true, requeue
	a.mu.Unlock()
	return nil
}
func (a *fakeAck) Reject(uint64, bool) error { return nil }

// fakeRunner plays the job: job writes the run directory and returns the
// exit code. A nil job blocks until the context ends.
type fakeRunner struct {
	mu       sync.Mutex
	job      func(dir string, j execution.Job) int
	err      error
	jobs     []execution.Job
	cleaned  bool
	started  chan uuid.UUID
	inFlight int
	maxSeen  int
}

func (f *fakeRunner) Name() string { return "host" }

func (f *fakeRunner) Run(ctx context.Context, j execution.Job) (int, error) {
	f.mu.Lock()
	f.jobs = append(f.jobs, j)
	f.inFlight++
	f.maxSeen = max(f.maxSeen, f.inFlight)
	f.mu.Unlock()
	defer func() { f.mu.Lock(); f.inFlight--; f.mu.Unlock() }()
	if f.started != nil {
		f.started <- j.RunID
	}
	if f.err != nil {
		return 0, f.err
	}
	if f.job == nil {
		<-ctx.Done()
		return 0, ctx.Err()
	}
	return f.job(j.Dir, j), nil
}

func (f *fakeRunner) Cleanup(context.Context) error { f.cleaned = true; return nil }

type env struct {
	w      *Worker
	runs   *runtest.Repository
	tasks  *tasktest.Repository
	runner *fakeRunner
	dir    string
	envs   map[string]string
}

func newEnv(t *testing.T, runner *fakeRunner, tweak func(*Config)) *env {
	t.Helper()
	tasks := tasktest.NewRepository()
	runs := runtest.NewRepository(tasks)
	dir := t.TempDir()
	e := &env{runs: runs, tasks: tasks, runner: runner, dir: dir, envs: map[string]string{"CLAUDE_CODE_OAUTH_TOKEN": "tok"}}
	cfg := Config{
		MaxConcurrent: 1, Timeout: time.Minute, Image: "miner-agent:default",
		RunsDir: dir, RunsHostDir: dir, Getenv: func(k string) string { return e.envs[k] },
		CancelPoll: 10 * time.Millisecond, RetryDelay: time.Millisecond,
	}
	if tweak != nil {
		tweak(&cfg)
	}
	log := slog.New(slog.NewJSONHandler(io.Discard, nil))
	e.w = New(cfg, nil, runs, tasks, tasktest.Catalog(), []execution.Runner{runner}, log)
	return e
}

// queue creates a ready task, starts a run of it and returns the run.
func (e *env) queue(t *testing.T) run.Run {
	t.Helper()
	ctx := context.Background()
	id := uuid.Must(uuid.NewV7())
	now := time.Now().UTC()
	if err := e.tasks.InsertTask(ctx, task.Task{
		ID: id, Provider: task.ProviderLocal, ExternalID: id.String(), Title: "Corrigir login", Description: "Implemente",
		AdditionalInstructions: []string{"passo 1"}, Status: task.StatusReady, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	r := runtest.NewRun(id, now)
	if err := e.runs.Start(ctx, r); err != nil {
		t.Fatal(err)
	}
	return r
}

// deliver hands the run message to the worker, as the queue would.
func (e *env) deliver(r run.Run) *fakeAck {
	body, _ := json.Marshal(run.Message{RunID: r.ID})
	ack := &fakeAck{}
	e.w.handle(context.Background(), amqp.Delivery{Acknowledger: ack, Body: body, MessageId: r.ID.String()})
	return ack
}

func (e *env) wait(t *testing.T) {
	t.Helper()
	e.w.jobs.Wait()
}

func (e *env) get(t *testing.T, r run.Run) (run.Run, task.Status) {
	t.Helper()
	got, err := e.runs.Get(context.Background(), r.ID)
	if err != nil {
		t.Fatal(err)
	}
	tk, err := e.tasks.GetTask(context.Background(), r.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	return got, tk.Status
}

func write(t *testing.T, dir, name, data string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

const events = `{"type":"system","subtype":"init","model":"claude-opus-5-5"}
not json
{"type":"assistant"}
{"type":"result","total_cost_usd":0.42,"num_turns":7}
`

func TestSuccessfulRun(t *testing.T) {
	var jobDir string
	runner := &fakeRunner{job: func(dir string, j execution.Job) int {
		jobDir = dir
		write(t, dir, "events.jsonl", events)
		write(t, dir, "agent_exit", "0\n")
		write(t, dir, "diff.patch", "+hello\n")
		return 0
	}}
	e := newEnv(t, runner, func(c *Config) { c.GitToken = "git-secret" })
	r := e.queue(t)

	ack := e.deliver(r)
	e.wait(t)
	if !ack.acked {
		t.Error("message not acknowledged")
	}
	got, taskStatus := e.get(t, r)
	if got.Status != run.StatusSucceeded || taskStatus != task.StatusInReview {
		t.Fatalf("run %s, task %s", got.Status, taskStatus)
	}
	res := got.Result
	if *res.Diff != "+hello\n" || *res.EffectiveModel != "claude-opus-5-5" || *res.CostUSD != 0.42 || *res.NumTurns != 7 || *res.ExitCode != 0 {
		t.Errorf("result = %+v", res)
	}
	if *got.AuthMethod != "oauth" {
		t.Errorf("auth method = %v", got.AuthMethod)
	}

	j := runner.jobs[0]
	if j.Env["CLAUDE_CODE_OAUTH_TOKEN"] != "tok" || j.Env["COMMIT_MESSAGE"] != "Corrigir login" || j.Env["BRANCH"] != r.Branch {
		t.Errorf("job env = %v", j.Env)
	}
	for k, v := range j.Env {
		if strings.Contains(v, "git-secret") {
			t.Errorf("git token in the job environment (%s)", k)
		}
	}
	prompt, _ := os.ReadFile(filepath.Join(jobDir, "prompt.md"))
	if !strings.Contains(string(prompt), "Implemente") || !strings.Contains(string(prompt), "1. passo 1") {
		t.Errorf("prompt = %q", prompt)
	}
	var argv []string
	data, _ := os.ReadFile(filepath.Join(jobDir, "command.json"))
	if json.Unmarshal(data, &argv) != nil || argv[0] != "claude" || !strings.Contains(strings.Join(argv, " "), "--model opus --effort high") {
		t.Errorf("command = %s", data)
	}
	if _, err := os.Stat(filepath.Join(jobDir, "git-token")); !os.IsNotExist(err) {
		t.Errorf("git token file left behind: %v", err)
	}
}

func TestFailedRuns(t *testing.T) {
	tests := map[string]struct {
		code int
		job  func(t *testing.T, dir string)
		want string
	}{
		"no changes":   {exitNoChanges, func(*testing.T, string) {}, "agent made no changes"},
		"clone failed": {exitClone, func(t *testing.T, dir string) { write(t, dir, "clone.log", "fatal: repository not found\n") }, "clone failed: fatal: repository not found"},
		"agent failed": {exitAgent, func(t *testing.T, dir string) { write(t, dir, "agent_exit", "2") }, "agent exited with code 2"},
		"agent reported its error": {exitAgent, func(t *testing.T, dir string) {
			write(t, dir, "agent_exit", "1")
			write(t, dir, "events.jsonl", `{"type":"result","is_error":true,"result":"Failed to authenticate. API Error: 401"}`+"\n")
		}, "agent exited with code 1: Failed to authenticate. API Error: 401"},
		"unexpected": {1, func(*testing.T, string) {}, "job exited with code 1"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			e := newEnv(t, &fakeRunner{job: func(dir string, _ execution.Job) int { tt.job(t, dir); return tt.code }}, nil)
			r := e.queue(t)
			e.deliver(r)
			e.wait(t)
			got, taskStatus := e.get(t, r)
			if got.Status != run.StatusFailed || *got.Result.Error != tt.want || taskStatus != task.StatusFailed {
				t.Errorf("run %s %q, task %s", got.Status, deref(got.Result.Error), taskStatus)
			}
		})
	}
}

func TestRunsThatCannotStart(t *testing.T) {
	t.Run("no credentials", func(t *testing.T) {
		runner := &fakeRunner{job: func(string, execution.Job) int { return 0 }}
		e := newEnv(t, runner, nil)
		e.envs = map[string]string{}
		r := e.queue(t)
		ack := e.deliver(r)
		got, taskStatus := e.get(t, r)
		if !ack.acked || got.Status != run.StatusFailed || *got.Result.Error != "no agent credentials configured" || taskStatus != task.StatusFailed {
			t.Errorf("run %s %q, task %s, acked %v", got.Status, deref(got.Result.Error), taskStatus, ack.acked)
		}
		if len(runner.jobs) != 0 {
			t.Error("environment created without credentials")
		}
	})

	t.Run("runner error such as a missing image", func(t *testing.T) {
		e := newEnv(t, &fakeRunner{err: errors.New("agent image not found: miner-agent:default (build it with `make agent-image`)")}, nil)
		r := e.queue(t)
		e.deliver(r)
		e.wait(t)
		if got, _ := e.get(t, r); got.Status != run.StatusFailed || !strings.Contains(*got.Result.Error, "make agent-image") {
			t.Errorf("run %s %q", got.Status, deref(got.Result.Error))
		}
	})

	t.Run("messages that start nothing", func(t *testing.T) {
		e := newEnv(t, &fakeRunner{}, nil)
		ack := &fakeAck{}
		e.w.handle(context.Background(), amqp.Delivery{Acknowledger: ack, Body: []byte("not json")})
		if !ack.nacked || ack.requeue {
			t.Errorf("invalid body settled %+v, want dead-lettered", ack)
		}
		if ack := e.deliver(run.Run{ID: uuid.New()}); !ack.acked {
			t.Error("missing run not acknowledged")
		}
		r := e.queue(t)
		if _, err := e.runs.Finish(context.Background(), r.ID, run.StatusCancelled, run.Result{}, time.Now()); err != nil {
			t.Fatal(err)
		}
		if ack := e.deliver(r); !ack.acked || len(e.runner.jobs) != 0 {
			t.Error("cancelled run started or not acknowledged")
		}
	})
}

func TestConcurrencyLimit(t *testing.T) {
	release := make(chan struct{})
	runner := &fakeRunner{started: make(chan uuid.UUID, 2), job: func(string, execution.Job) int { <-release; return exitNoChanges }}
	e := newEnv(t, runner, nil)
	first, second := e.queue(t), e.queue(t)

	e.deliver(first)
	<-runner.started
	delivered := make(chan struct{})
	go func() { e.deliver(second); close(delivered) }()

	// The second message waits for the slot, so its run stays queued.
	select {
	case <-delivered:
		t.Fatal("second run started while the only slot was busy")
	case <-time.After(50 * time.Millisecond):
	}
	if got, _ := e.get(t, second); got.Status != run.StatusQueued {
		t.Errorf("second run = %s, want queued", got.Status)
	}
	close(release)
	<-delivered
	<-runner.started
	e.wait(t)
	if runner.maxSeen != 1 {
		t.Errorf("%d runs at the same time, want 1", runner.maxSeen)
	}
}

func TestCancellationTimeoutAndStop(t *testing.T) {
	t.Run("cancel requested", func(t *testing.T) {
		runner := &fakeRunner{started: make(chan uuid.UUID, 1)}
		e := newEnv(t, runner, nil)
		r := e.queue(t)
		e.deliver(r)
		<-runner.started
		if _, err := e.runs.RequestCancel(context.Background(), r.ID, time.Now()); err != nil {
			t.Fatal(err)
		}
		e.wait(t)
		if got, taskStatus := e.get(t, r); got.Status != run.StatusCancelled || taskStatus != task.StatusReady {
			t.Errorf("run %s, task %s", got.Status, taskStatus)
		}
	})

	t.Run("timeout", func(t *testing.T) {
		e := newEnv(t, &fakeRunner{}, func(c *Config) { c.Timeout = 30 * time.Millisecond })
		r := e.queue(t)
		e.deliver(r)
		e.wait(t)
		if got, taskStatus := e.get(t, r); got.Status != run.StatusFailed || *got.Result.Error != "run timed out" || taskStatus != task.StatusFailed {
			t.Errorf("run %s %q, task %s", got.Status, deref(got.Result.Error), taskStatus)
		}
	})

	t.Run("stop", func(t *testing.T) {
		runner := &fakeRunner{started: make(chan uuid.UUID, 1)}
		e := newEnv(t, runner, nil)
		e.w.broker = blockingBroker{}
		e.w.Start()
		r := e.queue(t)
		e.deliver(r)
		<-runner.started
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := e.w.Stop(ctx); err != nil {
			t.Fatal(err)
		}
		if got, _ := e.get(t, r); got.Status != run.StatusFailed || *got.Result.Error != "worker stopped" {
			t.Errorf("run %s %q", got.Status, deref(got.Result.Error))
		}
	})
}

type blockingBroker struct{}

func (blockingBroker) Consume(ctx context.Context, _ string, _ int, _ func(amqp.Delivery)) error {
	<-ctx.Done()
	return nil
}

func TestRecover(t *testing.T) {
	ctx := context.Background()
	runner := &fakeRunner{}
	e := newEnv(t, runner, nil)
	lost, cancelled, queued := e.queue(t), e.queue(t), e.queue(t)
	for _, r := range []run.Run{lost, cancelled} {
		if _, err := e.runs.MarkRunning(ctx, r.ID, time.Now(), "oauth"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := e.runs.RequestCancel(ctx, cancelled.ID, time.Now()); err != nil {
		t.Fatal(err)
	}

	if err := e.w.Recover(ctx); err != nil {
		t.Fatal(err)
	}
	if !runner.cleaned {
		t.Error("runner not cleaned up")
	}
	if got, taskStatus := e.get(t, lost); got.Status != run.StatusFailed || *got.Result.Error != "worker restarted" || taskStatus != task.StatusFailed {
		t.Errorf("lost run %s %q, task %s", got.Status, deref(got.Result.Error), taskStatus)
	}
	if got, taskStatus := e.get(t, cancelled); got.Status != run.StatusCancelled || taskStatus != task.StatusReady {
		t.Errorf("cancelled run %s, task %s", got.Status, taskStatus)
	}
	if got, _ := e.get(t, queued); got.Status != run.StatusQueued {
		t.Errorf("queued run = %s", got.Status)
	}
}

func TestDiffLimit(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "diff.patch", strings.Repeat("x", maxDiffBytes+10))
	if d := readDiff(dir); len(*d) > maxDiffBytes+100 || !strings.Contains(*d, "diff truncated") {
		t.Errorf("diff of %d bytes not truncated", len(*d))
	}
}
