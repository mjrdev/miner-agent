package httpapi

import (
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"

	"github.com/jrgomesdev/miner/internal/execution/run"
)

// Runs serves the runs of tasks: starting, reading, cancelling and the
// agent's events.
type Runs struct {
	svc *run.Service
	// runsDir holds a directory per run, written by the worker.
	runsDir string
	log     *slog.Logger
}

// NewRuns returns the run handlers backed by svc, reading the run events from
// runsDir.
func NewRuns(svc *run.Service, runsDir string, log *slog.Logger) *Runs {
	return &Runs{svc: svc, runsDir: runsDir, log: log}
}

// Register adds the run routes to mux.
func (h *Runs) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /tasks/{id}/runs", h.start)
	mux.HandleFunc("GET /tasks/{id}/runs", h.list)
	mux.HandleFunc("GET /runs/{id}", h.get)
	mux.HandleFunc("GET /runs/{id}/logs", h.logs)
	mux.HandleFunc("POST /runs/{id}/cancel", h.cancel)
}

type runResponse struct {
	ID             uuid.UUID  `json:"id"`
	TaskID         uuid.UUID  `json:"task_id"`
	Status         run.Status `json:"status"`
	Agent          string     `json:"agent"`
	Model          string     `json:"model"`
	Effort         *string    `json:"effort"`
	Runner         string     `json:"runner"`
	RepoURL        string     `json:"repo_url"`
	BaseBranch     string     `json:"base_branch"`
	Branch         string     `json:"branch"`
	AuthMethod     *string    `json:"auth_method"`
	EffectiveModel *string    `json:"effective_model"`
	CostUSD        *float64   `json:"cost_usd"`
	NumTurns       *int       `json:"num_turns"`
	ExitCode       *int       `json:"exit_code"`
	Error          *string    `json:"error"`
	CreatedAt      time.Time  `json:"created_at"`
	StartedAt      *time.Time `json:"started_at"`
	FinishedAt     *time.Time `json:"finished_at"`
}

// runDetailResponse is a single run, the only place the diff appears.
type runDetailResponse struct {
	runResponse
	Diff *string `json:"diff"`
}

func toRunResponse(r run.Run) runResponse {
	return runResponse{
		ID:             r.ID,
		TaskID:         r.TaskID,
		Status:         r.Status,
		Agent:          r.Agent,
		Model:          r.Model,
		Effort:         r.Effort,
		Runner:         r.Runner,
		RepoURL:        r.RepoURL,
		BaseBranch:     r.BaseBranch,
		Branch:         r.Branch,
		AuthMethod:     r.AuthMethod,
		EffectiveModel: r.Result.EffectiveModel,
		CostUSD:        r.Result.CostUSD,
		NumTurns:       r.Result.NumTurns,
		ExitCode:       r.Result.ExitCode,
		Error:          r.Result.Error,
		CreatedAt:      r.CreatedAt,
		StartedAt:      r.StartedAt,
		FinishedAt:     r.FinishedAt,
	}
}

func (h *Runs) start(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	started, err := h.svc.Start(r.Context(), id)
	if err != nil {
		writeDomainError(w, r, h.log, err)
		return
	}
	writeJSON(w, http.StatusAccepted, toRunResponse(started))
}

type runListResponse struct {
	Items []runResponse `json:"items"`
}

func (h *Runs) list(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	runs, err := h.svc.ListByTask(r.Context(), id)
	if err != nil {
		writeDomainError(w, r, h.log, err)
		return
	}
	items := make([]runResponse, 0, len(runs))
	for _, rn := range runs {
		items = append(items, toRunResponse(rn))
	}
	writeJSON(w, http.StatusOK, runListResponse{Items: items})
}

func (h *Runs) get(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	rn, err := h.svc.Get(r.Context(), id)
	if err != nil {
		writeDomainError(w, r, h.log, err)
		return
	}
	writeJSON(w, http.StatusOK, runDetailResponse{runResponse: toRunResponse(rn), Diff: rn.Result.Diff})
}

// logs streams the events the agent wrote, one JSON object per line. A run
// that has not written any yet answers with an empty body.
func (h *Runs) logs(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if _, err := h.svc.Get(r.Context(), id); err != nil {
		writeDomainError(w, r, h.log, err)
		return
	}
	f, err := os.Open(filepath.Join(h.runsDir, id.String(), "events.jsonl"))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		writeDomainError(w, r, h.log, err)
		return
	}
	w.Header().Set("Content-Type", "application/x-ndjson")
	w.WriteHeader(http.StatusOK)
	if f == nil {
		return
	}
	defer func() { _ = f.Close() }()
	if _, err := io.Copy(w, f); err != nil {
		h.log.WarnContext(r.Context(), "streaming run events interrupted", "run_id", id, "error", err)
	}
}

func (h *Runs) cancel(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	rn, err := h.svc.Cancel(r.Context(), id)
	if err != nil {
		writeDomainError(w, r, h.log, err)
		return
	}
	writeJSON(w, http.StatusAccepted, toRunResponse(rn))
}
