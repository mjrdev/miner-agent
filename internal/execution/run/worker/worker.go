// Package worker executes the queued runs: it takes them from the queue,
// runs each job on its runner and records the outcome. The database holds
// the runs, so the queue message is acknowledged as soon as a run starts and
// a restart recovers the runs it left behind.
package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/jrgomesdev/miner/internal/execution"
	"github.com/jrgomesdev/miner/internal/execution/run"
	"github.com/jrgomesdev/miner/internal/task"
)

// Exit codes of the job entrypoint (build/agent/miner-job).
const (
	exitClone     = 10
	exitNoChanges = 11
	exitAgent     = 12
)

const (
	// maxDiffBytes bounds the diff kept on the run; the workspace keeps all.
	maxDiffBytes = 1 << 20
	// writeTimeout bounds the database writes that finish a run.
	writeTimeout = 30 * time.Second
)

var (
	errCancelled = errors.New("run cancelled")
	errTimedOut  = errors.New("run timed out")
	errStopped   = errors.New("worker stopped")
)

// Config tunes the worker.
type Config struct {
	// MaxConcurrent is how many runs execute at the same time.
	MaxConcurrent int
	// Timeout ends a run that takes longer.
	Timeout time.Duration
	// Image is the environment every run executes in.
	Image string
	// RunsDir is where this process writes the run directories, and
	// RunsHostDir the same directory as the runner's engine sees it.
	RunsDir     string
	RunsHostDir string
	// GitToken, when set, is used to clone the repository.
	GitToken string
	// Getenv reads the agents' credentials.
	Getenv func(string) string
	// CancelPoll is how often a running run checks for a cancellation.
	CancelPoll time.Duration
	// RetryDelay is the pause before a message is returned to the queue.
	RetryDelay time.Duration
}

type subscriber interface {
	Consume(ctx context.Context, queue string, prefetch int, handle func(amqp.Delivery)) error
}

// Worker takes runs from the queue and executes them.
type Worker struct {
	cfg     Config
	broker  subscriber
	runs    run.Repository
	tasks   task.Repository
	catalog *execution.Catalog
	runners map[string]execution.Runner
	log     *slog.Logger
	now     func() time.Time

	slots chan struct{}
	jobs  sync.WaitGroup
	// stopJobs ends every run in progress with errStopped.
	jobsCtx  context.Context
	stopJobs context.CancelCauseFunc
	cancel   context.CancelFunc
	done     chan struct{}
}

// New returns a worker that executes the runs of runs on runners, reading
// their tasks from tasks.
func New(
	cfg Config,
	broker subscriber,
	runs run.Repository,
	tasks task.Repository,
	catalog *execution.Catalog,
	runners []execution.Runner,
	log *slog.Logger,
) *Worker {
	byName := make(map[string]execution.Runner, len(runners))
	for _, r := range runners {
		byName[r.Name()] = r
	}
	jobsCtx, stopJobs := context.WithCancelCause(context.Background())
	return &Worker{
		cfg:      cfg,
		broker:   broker,
		runs:     runs,
		tasks:    tasks,
		catalog:  catalog,
		runners:  byName,
		log:      log,
		now:      func() time.Time { return time.Now().UTC().Truncate(time.Microsecond) },
		slots:    make(chan struct{}, max(cfg.MaxConcurrent, 1)),
		jobsCtx:  jobsCtx,
		stopJobs: stopJobs,
	}
}

// Recover removes the environments left by a previous process and fails the
// runs it was executing; queued runs stay in the queue.
func (w *Worker) Recover(ctx context.Context) error {
	var errs []error
	for _, r := range w.runners {
		if err := r.Cleanup(ctx); err != nil {
			errs = append(errs, fmt.Errorf("clean up runner %s: %w", r.Name(), err))
		}
	}
	running, err := w.runs.ListRunning(ctx)
	if err != nil {
		return errors.Join(append(errs, err)...)
	}
	for _, r := range running {
		status, res := run.StatusFailed, run.Result{Error: new("worker restarted")}
		if r.CancelRequestedAt != nil {
			status, res = run.StatusCancelled, run.Result{}
		}
		if _, err := w.runs.Finish(ctx, r.ID, status, res, w.now()); err != nil {
			errs = append(errs, fmt.Errorf("recover run %s: %w", r.ID, err))
			continue
		}
		w.log.Warn("recovered run left by a previous worker", "run_id", r.ID, "status", status)
	}
	return errors.Join(errs...)
}

// Start consumes the run queue in the background.
func (w *Worker) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	w.cancel = cancel
	w.done = make(chan struct{})
	go func() {
		defer close(w.done)
		// prefetch 1: the handler blocks while every slot is busy, so the
		// next runs wait in the queue in order.
		_ = w.broker.Consume(ctx, run.Queue, 1, func(d amqp.Delivery) { w.handle(ctx, d) })
		w.log.Info("worker stopped consuming")
	}()
	w.log.Info("worker started", "queue", run.Queue, "max_concurrent_runs", cap(w.slots))
}

// Stop stops taking runs, ends the runs in progress as failed and waits for
// them to be recorded, up to ctx.
func (w *Worker) Stop(ctx context.Context) error {
	w.cancel()
	w.stopJobs(errStopped)
	finished := make(chan struct{})
	go func() {
		<-w.done
		w.jobs.Wait()
		close(finished)
	}()
	select {
	case <-finished:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("worker: stop: %w", ctx.Err())
	}
}

// handle starts the run of d or settles d when it cannot start.
func (w *Worker) handle(stopping context.Context, d amqp.Delivery) {
	settle := func(err error, what string) {
		if err != nil {
			w.log.Error("could not settle run message", "message_id", d.MessageId, "action", what, "error", err)
		}
	}
	requeue := func(reason string, err error) {
		w.log.Warn("returning run message to the queue", "message_id", d.MessageId, "reason", reason, "error", err)
		select {
		case <-time.After(w.cfg.RetryDelay):
		case <-stopping.Done():
		}
		settle(d.Nack(false, true), "requeue")
	}

	var msg run.Message
	if err := json.Unmarshal(d.Body, &msg); err != nil || msg.RunID == uuid.Nil {
		w.log.Error("dead-lettering run message: invalid body", "message_id", d.MessageId, "error", err)
		settle(d.Nack(false, false), "reject")
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), writeTimeout)
	defer cancel()

	r, err := w.runs.Get(ctx, msg.RunID)
	switch {
	case errors.Is(err, task.ErrNotFound):
		w.log.Warn("dropping message of a missing run", "run_id", msg.RunID)
		settle(d.Ack(false), "ack")
		return
	case err != nil:
		requeue("load run", err)
		return
	case r.Status != run.StatusQueued:
		w.log.Info("dropping message of a run that is no longer queued", "run_id", r.ID, "status", r.Status)
		settle(d.Ack(false), "ack")
		return
	}

	select {
	case w.slots <- struct{}{}:
	case <-stopping.Done():
		settle(d.Nack(false, true), "requeue")
		return
	}
	release := func() { <-w.slots }

	agent, cred, runner, failure := w.prepare(r)
	if failure != "" {
		release()
		if _, err := w.runs.Finish(ctx, r.ID, run.StatusFailed, run.Result{Error: &failure}, w.now()); err != nil {
			requeue("fail run", err)
			return
		}
		w.log.Warn("run failed before starting", "run_id", r.ID, "error", failure)
		settle(d.Ack(false), "ack")
		return
	}

	started, err := w.runs.MarkRunning(ctx, r.ID, w.now(), cred.Method)
	if err != nil {
		release()
		requeue("mark run running", err)
		return
	}
	if !started {
		release()
		settle(d.Ack(false), "ack")
		return
	}
	// From here the database holds the run: a lost worker is recovered on
	// the next start, so the message is no longer needed.
	settle(d.Ack(false), "ack")

	w.jobs.Add(1)
	go func() {
		defer w.jobs.Done()
		defer release()
		w.execute(r, agent, cred, runner)
	}()
}

// prepare resolves what r needs to run, or the reason it cannot.
func (w *Worker) prepare(r run.Run) (execution.Agent, execution.Credential, execution.Runner, string) {
	agent, ok := w.catalog.Agent(r.Agent)
	if !ok {
		return nil, execution.Credential{}, nil, fmt.Sprintf("unknown agent %q", r.Agent)
	}
	runner, ok := w.runners[r.Runner]
	if !ok {
		return nil, execution.Credential{}, nil, fmt.Sprintf("runner %q is not available", r.Runner)
	}
	cred, ok := agent.Credential(w.cfg.Getenv)
	if !ok {
		return nil, execution.Credential{}, nil, "no agent credentials configured"
	}
	return agent, cred, runner, ""
}

// execute runs the job of r and records its outcome.
func (w *Worker) execute(r run.Run, agent execution.Agent, cred execution.Credential, runner execution.Runner) {
	log := w.log.With("run_id", r.ID, "task_id", r.TaskID)
	log.Info("run started", "agent", r.Agent, "model", r.Model, "runner", r.Runner, "auth_method", cred.Method)

	status, res := w.runJob(r, agent, cred, runner)

	ctx, cancel := context.WithTimeout(context.Background(), writeTimeout)
	defer cancel()
	if _, err := w.runs.Finish(ctx, r.ID, status, res, w.now()); err != nil {
		log.Error("could not record the run outcome", "status", status, "error", err)
		return
	}
	log.Info("run finished", "status", status, "error", deref(res.Error))
}

func (w *Worker) runJob(r run.Run, agent execution.Agent, cred execution.Credential, runner execution.Runner) (run.Status, run.Result) {
	failed := func(msg string) (run.Status, run.Result) {
		return run.StatusFailed, run.Result{Error: &msg}
	}
	dir := filepath.Join(w.cfg.RunsDir, r.ID.String())
	tk, err := w.tasks.GetTask(context.Background(), r.TaskID)
	if err != nil {
		return failed("load task: " + err.Error())
	}
	if err := w.writeJobFiles(dir, r, tk, agent); err != nil {
		return failed("prepare run directory: " + err.Error())
	}
	// The token file is deleted by the job before the agent starts; this
	// covers a job that never got that far.
	defer func() { _ = os.Remove(filepath.Join(dir, "git-token")) }()

	ctx, cancel := context.WithTimeoutCause(w.jobsCtx, w.cfg.Timeout, errTimedOut)
	defer cancel()
	ctx, cancelRun := context.WithCancelCause(ctx)
	defer cancelRun(nil)
	go w.watchCancel(ctx, r.ID, cancelRun)

	code, runErr := runner.Run(ctx, execution.Job{
		RunID: r.ID,
		Dir:   filepath.Join(w.cfg.RunsHostDir, r.ID.String()),
		Image: w.cfg.Image,
		Env: map[string]string{
			"REPO_URL":       r.RepoURL,
			"BASE_BRANCH":    r.BaseBranch,
			"BRANCH":         r.Branch,
			"COMMIT_MESSAGE": tk.Title,
			cred.Env:         cred.Value,
		},
	})

	res := summarize(dir)
	switch cause := context.Cause(ctx); {
	case runErr != nil && errors.Is(cause, errCancelled):
		return run.StatusCancelled, res
	case runErr != nil && (errors.Is(cause, errTimedOut) || errors.Is(cause, errStopped)):
		res.Error = new(cause.Error())
		return run.StatusFailed, res
	case runErr != nil:
		res.Error = new(runErr.Error())
		return run.StatusFailed, res
	}

	switch code {
	case 0:
		res.Diff = readDiff(dir)
		return run.StatusSucceeded, res
	case exitClone:
		res.Error = new("clone failed: " + lastLine(filepath.Join(dir, "clone.log")))
	case exitNoChanges:
		res.Error = new("agent made no changes")
	case exitAgent:
		msg := fmt.Sprintf("agent exited with code %d", deref(res.ExitCode))
		if reason := agentError(dir); reason != "" {
			msg += ": " + reason
		}
		res.Error = &msg
	default:
		res.Error = new(fmt.Sprintf("job exited with code %d", code))
	}
	return run.StatusFailed, res
}

// writeJobFiles fills the run directory the job reads.
func (w *Worker) writeJobFiles(dir string, r run.Run, tk task.Task, agent execution.Agent) error {
	// World-writable so the job's user can write to it whatever this
	// process runs as.
	if err := os.MkdirAll(dir, 0o777); err != nil {
		return err
	}
	if err := os.Chmod(dir, 0o777); err != nil {
		return err
	}
	command, err := json.Marshal(agent.Command(execution.Invocation{
		Prompt: run.Prompt(tk), Model: r.Model, Effort: r.Effort,
	}))
	if err != nil {
		return err
	}
	files := map[string][]byte{"prompt.md": []byte(run.Prompt(tk)), "command.json": command}
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
			return err
		}
	}
	if w.cfg.GitToken != "" {
		// Readable by the job's user only; the job deletes it after the clone.
		path := filepath.Join(dir, "git-token")
		if err := os.WriteFile(path, []byte(w.cfg.GitToken), 0o600); err != nil {
			return err
		}
		if err := os.Chown(path, 1000, 1000); err != nil && !errors.Is(err, os.ErrPermission) {
			return err
		}
	}
	return nil
}

// watchCancel stops the run when a cancellation is requested.
func (w *Worker) watchCancel(ctx context.Context, id uuid.UUID, cancel context.CancelCauseFunc) {
	ticker := time.NewTicker(w.cfg.CancelPoll)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r, err := w.runs.Get(ctx, id)
			if err != nil {
				continue
			}
			if r.CancelRequestedAt != nil {
				cancel(errCancelled)
				return
			}
		}
	}
}

// summarize reads what the agent reported in its events and the agent's
// exit code. Unknown event formats leave the fields unset.
func summarize(dir string) run.Result {
	var res run.Result
	if data, err := os.ReadFile(filepath.Join(dir, "agent_exit")); err == nil {
		if code, err := strconv.Atoi(strings.TrimSpace(string(data))); err == nil {
			res.ExitCode = &code
		}
	}
	data, err := os.ReadFile(filepath.Join(dir, "events.jsonl"))
	if err != nil {
		return res
	}
	for line := range strings.Lines(string(data)) {
		var ev struct {
			Type     string   `json:"type"`
			Subtype  string   `json:"subtype"`
			Model    *string  `json:"model"`
			CostUSD  *float64 `json:"total_cost_usd"`
			NumTurns *int     `json:"num_turns"`
		}
		if json.Unmarshal([]byte(line), &ev) != nil {
			continue
		}
		switch {
		case ev.Type == "system" && ev.Subtype == "init" && ev.Model != nil:
			res.EffectiveModel = ev.Model
		case ev.Type == "result":
			res.CostUSD, res.NumTurns = ev.CostUSD, ev.NumTurns
		}
	}
	return res
}

// agentError returns the error the agent reported in its last result event,
// if any.
func agentError(dir string) string {
	data, err := os.ReadFile(filepath.Join(dir, "events.jsonl"))
	if err != nil {
		return ""
	}
	reason := ""
	for line := range strings.Lines(string(data)) {
		var ev struct {
			Type    string `json:"type"`
			IsError bool   `json:"is_error"`
			Result  string `json:"result"`
		}
		if json.Unmarshal([]byte(line), &ev) == nil && ev.Type == "result" && ev.IsError {
			reason = strings.TrimSpace(ev.Result)
		}
	}
	const maxReason = 500
	if len(reason) > maxReason {
		reason = reason[:maxReason] + "..."
	}
	return reason
}

func readDiff(dir string) *string {
	data, err := os.ReadFile(filepath.Join(dir, "diff.patch"))
	if err != nil {
		return nil
	}
	diff := string(data)
	if len(diff) > maxDiffBytes {
		diff = diff[:maxDiffBytes] + "\n... diff truncated, see the run workspace\n"
	}
	return &diff
}

func lastLine(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return "no details"
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	return lines[len(lines)-1]
}

func deref[T any](p *T) T {
	var zero T
	if p == nil {
		return zero
	}
	return *p
}
