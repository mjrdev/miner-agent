package httpapi

import (
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/jrgomesdev/miner/internal/execution"
	"github.com/jrgomesdev/miner/internal/task"
)

const (
	defaultLimit = 50
	maxLimit     = 200
)

// Tasks serves the CRUD of tasks, project categories and labels, and the
// execution options tasks may be configured with.
type Tasks struct {
	svc *task.Service
	log *slog.Logger
}

// NewTasks returns the task CRUD handlers backed by svc.
func NewTasks(svc *task.Service, log *slog.Logger) *Tasks {
	return &Tasks{svc: svc, log: log}
}

// Register adds the task, project category, label and execution option
// routes to mux.
func (h *Tasks) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /tasks", h.create)
	mux.HandleFunc("GET /tasks", h.list)
	mux.HandleFunc("GET /tasks/{id}", h.get)
	mux.HandleFunc("PATCH /tasks/{id}", h.update)
	mux.HandleFunc("DELETE /tasks/{id}", h.delete)
	mux.HandleFunc("GET /labels", h.listLabels)
	mux.HandleFunc("GET /execution-options", h.executionOptions)

	mux.HandleFunc("POST /project-categories", h.createCategory)
	mux.HandleFunc("GET /project-categories", h.listCategories)
	mux.HandleFunc("GET /project-categories/{id}", h.getCategory)
	mux.HandleFunc("PATCH /project-categories/{id}", h.updateCategory)
	mux.HandleFunc("DELETE /project-categories/{id}", h.deleteCategory)
}

type taskResponse struct {
	ID                     uuid.UUID        `json:"id"`
	Provider               string           `json:"provider"`
	ExternalID             string           `json:"external_id"`
	ExternalURL            *string          `json:"external_url"`
	Title                  string           `json:"title"`
	Description            string           `json:"description"`
	AdditionalInstructions []string         `json:"additional_instructions"`
	Labels                 []string         `json:"labels"`
	ProjectCategory        *categorySummary `json:"project_category"`
	Status                 task.Status      `json:"status"`
	Agent                  *string          `json:"agent"`
	Model                  *string          `json:"model"`
	Effort                 *string          `json:"effort"`
	Runner                 *string          `json:"runner"`
	CreatedAt              time.Time        `json:"created_at"`
	UpdatedAt              time.Time        `json:"updated_at"`
}

type categorySummary struct {
	ID   uuid.UUID `json:"id"`
	Slug string    `json:"slug"`
	Name string    `json:"name"`
}

func toTaskResponse(t task.Task) taskResponse {
	resp := taskResponse{
		ID:                     t.ID,
		Provider:               t.Provider,
		ExternalID:             t.ExternalID,
		ExternalURL:            t.ExternalURL,
		Title:                  t.Title,
		Description:            t.Description,
		AdditionalInstructions: nonNil(t.AdditionalInstructions),
		Labels:                 nonNil(t.Labels),
		Status:                 t.Status,
		Agent:                  t.Execution.Agent,
		Model:                  t.Execution.Model,
		Effort:                 t.Execution.Effort,
		Runner:                 t.Execution.Runner,
		CreatedAt:              t.CreatedAt,
		UpdatedAt:              t.UpdatedAt,
	}
	if c := t.ProjectCategory; c != nil {
		resp.ProjectCategory = &categorySummary{ID: c.ID, Slug: c.Slug, Name: c.Name}
	}
	return resp
}

type createTaskRequest struct {
	Title                  string     `json:"title"`
	Description            string     `json:"description"`
	AdditionalInstructions []string   `json:"additional_instructions"`
	Labels                 []string   `json:"labels"`
	ProjectCategoryID      *uuid.UUID `json:"project_category_id"`
	Agent                  *string    `json:"agent"`
	Model                  *string    `json:"model"`
	Effort                 *string    `json:"effort"`
	Runner                 *string    `json:"runner"`
}

func (h *Tasks) create(w http.ResponseWriter, r *http.Request) {
	var req createTaskRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	t, err := h.svc.CreateLocal(r.Context(), task.CreateInput{
		Title:                  req.Title,
		Description:            req.Description,
		AdditionalInstructions: req.AdditionalInstructions,
		Labels:                 req.Labels,
		ProjectCategoryID:      req.ProjectCategoryID,
		Execution: task.Execution{
			Agent:  req.Agent,
			Model:  req.Model,
			Effort: req.Effort,
			Runner: req.Runner,
		},
	})
	if err != nil {
		writeDomainError(w, r, h.log, err)
		return
	}
	writeJSON(w, http.StatusCreated, toTaskResponse(t))
}

type taskListResponse struct {
	Items []taskResponse `json:"items"`
	Total int            `json:"total"`
}

func (h *Tasks) list(w http.ResponseWriter, r *http.Request) {
	f, msg := parseListFilter(r)
	if msg != "" {
		writeError(w, http.StatusBadRequest, codeValidationFailed, msg)
		return
	}
	tasks, total, err := h.svc.List(r.Context(), f)
	if err != nil {
		writeDomainError(w, r, h.log, err)
		return
	}
	items := make([]taskResponse, 0, len(tasks))
	for _, t := range tasks {
		items = append(items, toTaskResponse(t))
	}
	writeJSON(w, http.StatusOK, taskListResponse{Items: items, Total: total})
}

// parseListFilter reads the GET /tasks query, returning a message describing
// the first invalid parameter.
func parseListFilter(r *http.Request) (task.ListFilter, string) {
	q := r.URL.Query()
	f := task.ListFilter{Limit: defaultLimit}

	if v := q.Get("status"); v != "" {
		s := task.Status(v)
		if !s.Valid() {
			return f, "status: unknown status " + strconv.Quote(v)
		}
		f.Status = &s
	}
	if v := q.Get("provider"); v != "" {
		f.Provider = &v
	}
	if v := q.Get("project_category_id"); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			return f, "project_category_id: must be a UUID"
		}
		f.ProjectCategoryID = &id
	}
	if v := q.Get("label"); v != "" {
		label, err := task.NormalizeLabel(v)
		if err != nil {
			return f, err.Error()
		}
		f.Label = &label
	}
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > maxLimit {
			return f, "limit: must be an integer between 1 and " + strconv.Itoa(maxLimit)
		}
		f.Limit = n
	}
	if v := q.Get("offset"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			return f, "offset: must be a non-negative integer"
		}
		f.Offset = n
	}
	return f, ""
}

func (h *Tasks) get(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	t, err := h.svc.Get(r.Context(), id)
	if err != nil {
		writeDomainError(w, r, h.log, err)
		return
	}
	writeJSON(w, http.StatusOK, toTaskResponse(t))
}

type updateTaskRequest struct {
	Title                  *string             `json:"title"`
	Description            *string             `json:"description"`
	AdditionalInstructions *[]string           `json:"additional_instructions"`
	Labels                 *[]string           `json:"labels"`
	ProjectCategoryID      Optional[uuid.UUID] `json:"project_category_id"`
	Agent                  Optional[string]    `json:"agent"`
	Model                  Optional[string]    `json:"model"`
	Effort                 Optional[string]    `json:"effort"`
	Runner                 Optional[string]    `json:"runner"`
	Status                 *task.Status        `json:"status"`
}

func (h *Tasks) update(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var req updateTaskRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	t, err := h.svc.Update(r.Context(), id, task.UpdateInput{
		Title:                  req.Title,
		Description:            req.Description,
		AdditionalInstructions: req.AdditionalInstructions,
		Labels:                 req.Labels,
		SetProjectCategory:     req.ProjectCategoryID.Set,
		ProjectCategoryID:      req.ProjectCategoryID.Value,
		SetAgent:               req.Agent.Set,
		Agent:                  req.Agent.Value,
		SetModel:               req.Model.Set,
		Model:                  req.Model.Value,
		SetEffort:              req.Effort.Set,
		Effort:                 req.Effort.Value,
		SetRunner:              req.Runner.Set,
		Runner:                 req.Runner.Value,
		Status:                 req.Status,
	})
	if err != nil {
		writeDomainError(w, r, h.log, err)
		return
	}
	writeJSON(w, http.StatusOK, toTaskResponse(t))
}

func (h *Tasks) delete(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := h.svc.Delete(r.Context(), id); err != nil {
		writeDomainError(w, r, h.log, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type labelListResponse struct {
	Items []string `json:"items"`
}

func (h *Tasks) listLabels(w http.ResponseWriter, r *http.Request) {
	labels, err := h.svc.ListLabels(r.Context())
	if err != nil {
		writeDomainError(w, r, h.log, err)
		return
	}
	writeJSON(w, http.StatusOK, labelListResponse{Items: nonNil(labels)})
}

type executionOptionsResponse struct {
	Agents  []agentOptions `json:"agents"`
	Runners []string       `json:"runners"`
}

type agentOptions struct {
	Name   string         `json:"name"`
	Models []modelOptions `json:"models"`
}

type modelOptions struct {
	ID      string   `json:"id"`
	Efforts []string `json:"efforts"`
}

func toExecutionOptionsResponse(opts execution.Options) executionOptionsResponse {
	resp := executionOptionsResponse{Agents: make([]agentOptions, 0, len(opts.Agents)), Runners: nonNil(opts.Runners)}
	for _, a := range opts.Agents {
		models := make([]modelOptions, 0, len(a.Models))
		for _, m := range a.Models {
			models = append(models, modelOptions{ID: m.ID, Efforts: nonNil(m.Efforts)})
		}
		resp.Agents = append(resp.Agents, agentOptions{Name: a.Name, Models: models})
	}
	return resp
}

func (h *Tasks) executionOptions(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, toExecutionOptionsResponse(h.svc.ExecutionOptions()))
}

func nonNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}
