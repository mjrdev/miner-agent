package task

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/jrgomesdev/miner/internal/execution"
)

// Service holds the Task domain rules on top of a Repository.
type Service struct {
	repo    Repository
	catalog *execution.Catalog
	runs    RunGuard
	now     func() time.Time
	newID   func() uuid.UUID
}

// RunGuard tells the task rules about the runs of a task, which belong to
// another package. While a task has an active run, its status follows the run.
type RunGuard interface {
	HasActiveRun(ctx context.Context, taskID uuid.UUID) (bool, error)
	// CancelActiveRun cancels the active run of a task, if any.
	CancelActiveRun(ctx context.Context, taskID uuid.UUID) error
}

// NewService returns a service that stores data in repo, validates the
// execution configuration of tasks against catalog and consults runs before
// changing the status of a task or deleting it.
func NewService(repo Repository, catalog *execution.Catalog, runs RunGuard) *Service {
	return &Service{
		repo:    repo,
		catalog: catalog,
		runs:    runs,
		// PostgreSQL keeps microseconds; truncating keeps reads equal to writes.
		now:   func() time.Time { return time.Now().UTC().Truncate(time.Microsecond) },
		newID: func() uuid.UUID { return uuid.Must(uuid.NewV7()) },
	}
}

// ExecutionOptions returns the agents, models, efforts and runners a task may
// be configured with.
func (s *Service) ExecutionOptions() execution.Options {
	return s.catalog.Options()
}

// CreateInput is the data for a task created directly, without a provider.
type CreateInput struct {
	Title                  string
	Description            string
	AdditionalInstructions []string
	Labels                 []string
	ProjectCategoryID      *uuid.UUID
	Execution              Execution
}

// CreateLocal creates a draft task from the local provider whose external id
// is its own id.
func (s *Service) CreateLocal(ctx context.Context, in CreateInput) (Task, error) {
	title, err := NormalizeTitle(in.Title)
	if err != nil {
		return Task{}, err
	}
	description, err := NormalizeDescription(in.Description)
	if err != nil {
		return Task{}, err
	}
	instructions, err := NormalizeInstructions(in.AdditionalInstructions)
	if err != nil {
		return Task{}, err
	}
	labels, err := NormalizeLabels(in.Labels)
	if err != nil {
		return Task{}, err
	}
	if err := s.validateExecution(in.Execution); err != nil {
		return Task{}, err
	}

	now := s.now()
	id := s.newID()
	t := Task{
		ID:                     id,
		Provider:               ProviderLocal,
		ExternalID:             id.String(),
		Title:                  title,
		Description:            description,
		AdditionalInstructions: instructions,
		Labels:                 labels,
		Status:                 StatusDraft,
		Execution:              in.Execution,
		CreatedAt:              now,
		UpdatedAt:              now,
	}
	if in.ProjectCategoryID != nil {
		t.ProjectCategory = &CategoryRef{ID: *in.ProjectCategoryID}
	}

	if err := s.repo.InsertTask(ctx, t); err != nil {
		return Task{}, categoryNotFoundAsValidation(err)
	}
	return s.repo.GetTask(ctx, id)
}

// Get returns the task with id.
func (s *Service) Get(ctx context.Context, id uuid.UUID) (Task, error) {
	return s.repo.GetTask(ctx, id)
}

// List returns a page of tasks matching f and the total that match it.
func (s *Service) List(ctx context.Context, f ListFilter) ([]Task, int, error) {
	return s.repo.ListTasks(ctx, f)
}

// UpdateInput holds the fields of a partial task update; nil fields are left
// unchanged. ProjectCategoryID and each execution field are applied only when
// their Set flag is true, and a nil value then clears them.
type UpdateInput struct {
	Title                  *string
	Description            *string
	AdditionalInstructions *[]string
	Labels                 *[]string
	SetProjectCategory     bool
	ProjectCategoryID      *uuid.UUID
	SetAgent               bool
	Agent                  *string
	SetModel               bool
	Model                  *string
	SetEffort              bool
	Effort                 *string
	SetRunner              bool
	Runner                 *string
	Status                 *Status
}

// executionFields returns the names of the execution fields in the update.
func (in UpdateInput) executionFields() []string {
	var fields []string
	for _, f := range []struct {
		set  bool
		name string
	}{{in.SetAgent, "agent"}, {in.SetModel, "model"}, {in.SetEffort, "effort"}, {in.SetRunner, "runner"}} {
		if f.set {
			fields = append(fields, f.name)
		}
	}
	return fields
}

func (in UpdateInput) empty() bool {
	return in == (UpdateInput{})
}

// Update applies a partial update. Title and description belong to the
// provider and may only change on local tasks; status changes must follow
// the lifecycle; the execution configuration is locked while the task runs or
// after it ends, and a ready task must be fully configured.
func (s *Service) Update(ctx context.Context, id uuid.UUID, in UpdateInput) (Task, error) {
	if in.empty() {
		return s.repo.GetTask(ctx, id)
	}
	if in.Status != nil && !in.Status.Valid() {
		return Task{}, invalid("status", "unknown status %q", *in.Status)
	}
	if in.Status != nil && *in.Status != StatusCancelled {
		if err := s.checkNoActiveRun(ctx, id, "status follows the active run"); err != nil {
			return Task{}, err
		}
	}

	// The new category is loaded here because the mutation below runs while
	// the repository holds the task and must not query it.
	var category *CategoryRef
	if in.SetProjectCategory && in.ProjectCategoryID != nil {
		c, err := s.repo.GetCategory(ctx, *in.ProjectCategoryID)
		if errors.Is(err, ErrNotFound) {
			return Task{}, categoryNotFoundAsValidation(ErrCategoryNotFound)
		}
		if err != nil {
			return Task{}, err
		}
		category = &CategoryRef{ID: c.ID, Slug: c.Slug, Name: c.Name, RepoURL: c.RepoURL}
	}

	t, err := s.repo.UpdateTask(ctx, id, func(t *Task) error {
		return s.apply(t, in, category)
	})
	if err != nil {
		return Task{}, categoryNotFoundAsValidation(err)
	}
	if t.Status == StatusCancelled {
		if err := s.runs.CancelActiveRun(ctx, id); err != nil {
			return Task{}, fmt.Errorf("cancel active run: %w", err)
		}
	}
	return t, nil
}

// apply validates in against t and changes t in place. category is the
// resolved category set by in, if any.
func (s *Service) apply(t *Task, in UpdateInput, category *CategoryRef) error {
	touchesProviderFields := in.Title != nil || in.Description != nil
	if touchesProviderFields && t.Provider != ProviderLocal {
		return fmt.Errorf("%w: title and description belong to provider %q", ErrFieldNotEditable, t.Provider)
	}
	executionFields := in.executionFields()
	if len(executionFields) > 0 && t.Status.locksExecution() {
		return invalid(executionFields[0], "execution configuration cannot change while the task is %s", t.Status)
	}

	if in.Title != nil {
		title, err := NormalizeTitle(*in.Title)
		if err != nil {
			return err
		}
		t.Title = title
	}
	if in.Description != nil {
		description, err := NormalizeDescription(*in.Description)
		if err != nil {
			return err
		}
		t.Description = description
	}
	if in.AdditionalInstructions != nil {
		instructions, err := NormalizeInstructions(*in.AdditionalInstructions)
		if err != nil {
			return err
		}
		t.AdditionalInstructions = instructions
	}
	if in.Labels != nil {
		labels, err := NormalizeLabels(*in.Labels)
		if err != nil {
			return err
		}
		t.Labels = labels
	}
	if in.SetProjectCategory {
		t.ProjectCategory = category
	}
	if in.SetAgent {
		t.Execution.Agent = in.Agent
	}
	if in.SetModel {
		t.Execution.Model = in.Model
	}
	if in.SetEffort {
		t.Execution.Effort = in.Effort
	}
	if in.SetRunner {
		t.Execution.Runner = in.Runner
	}
	if in.Status != nil && *in.Status != t.Status {
		if !t.Status.CanTransitionTo(*in.Status) {
			return fmt.Errorf("%w: %s -> %s", ErrInvalidTransition, t.Status, *in.Status)
		}
		t.Status = *in.Status
	}
	// A model removed from the catalog only fails when the configuration is
	// touched or the task must be ready, so the task stays editable otherwise.
	if len(executionFields) > 0 || t.Status == StatusReady {
		if err := s.validateExecution(t.Execution); err != nil {
			return err
		}
	}
	if t.Status == StatusReady {
		if err := s.checkReady(t); err != nil {
			return err
		}
	}

	t.UpdatedAt = s.now()
	return nil
}

// validateExecution checks e against the catalog: each field set must exist,
// within the agent and model chosen.
func (s *Service) validateExecution(e Execution) error {
	var agent execution.Agent
	if e.Agent != nil {
		a, ok := s.catalog.Agent(*e.Agent)
		if !ok {
			return invalid("agent", "unknown agent %q", *e.Agent)
		}
		agent = a
	}
	var model *execution.Model
	if e.Model != nil {
		if agent == nil {
			return invalid("model", "requires an agent")
		}
		m, ok := execution.FindModel(agent, *e.Model)
		if !ok {
			return invalid("model", "agent %q has no model %q", agent.Name(), *e.Model)
		}
		model = &m
	}
	if e.Effort != nil {
		if model == nil {
			return invalid("effort", "requires a model")
		}
		if !slices.Contains(model.Efforts, *e.Effort) {
			return invalid("effort", "model %q has no effort %q", model.ID, *e.Effort)
		}
	}
	if e.Runner != nil && !s.catalog.HasRunner(*e.Runner) {
		return invalid("runner", "unknown runner %q", *e.Runner)
	}
	return nil
}

// checkReady reports every missing prerequisite of a ready task. The
// execution configuration must already be valid.
func (s *Service) checkReady(t *Task) error {
	var missing []string
	e := t.Execution
	if e.Agent == nil {
		missing = append(missing, "agent")
	}
	if e.Model == nil {
		missing = append(missing, "model")
	} else if e.Effort == nil {
		agent, _ := s.catalog.Agent(*e.Agent)
		if m, _ := execution.FindModel(agent, *e.Model); len(m.Efforts) > 0 {
			missing = append(missing, "effort")
		}
	}
	if e.Runner == nil {
		missing = append(missing, "runner")
	}
	switch {
	case t.ProjectCategory == nil:
		missing = append(missing, "project_category")
	case t.ProjectCategory.RepoURL == nil:
		missing = append(missing, "project_category.repo_url")
	}
	if len(missing) > 0 {
		return invalid("status", "task cannot be ready: missing %s", strings.Join(missing, ", "))
	}
	return nil
}

// Delete removes the task with id.
func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {
	if err := s.checkNoActiveRun(ctx, id, "task has an active run"); err != nil {
		return err
	}
	return s.repo.DeleteTask(ctx, id)
}

// checkNoActiveRun returns ErrConflict with reason when the task has an
// active run.
func (s *Service) checkNoActiveRun(ctx context.Context, id uuid.UUID, reason string) error {
	active, err := s.runs.HasActiveRun(ctx, id)
	if err != nil {
		return fmt.Errorf("check active run: %w", err)
	}
	if active {
		return fmt.Errorf("%w: %s", ErrConflict, reason)
	}
	return nil
}

// CheckRunnable reports why t cannot start a run: it must be ready, with an
// execution configuration that is still in the catalog and every ready
// prerequisite met.
func (s *Service) CheckRunnable(t Task) error {
	if t.Status != StatusReady {
		return fmt.Errorf("%w: task is %s, not ready", ErrInvalidTransition, t.Status)
	}
	if err := s.validateExecution(t.Execution); err != nil {
		return err
	}
	return s.checkReady(&t)
}

// ApplyEvent writes the effect of a provider event and reports whether a task
// was created. A created event inserts a draft task unless one already exists
// for the same provider and external id, so redeliveries keep user edits.
// Updated and closed events are not applied yet.
func (s *Service) ApplyEvent(ctx context.Context, provider string, ev Event) (bool, error) {
	if ev.Type != EventCreated {
		return false, nil
	}

	if strings.TrimSpace(ev.ExternalID) == "" {
		return false, invalid("external_id", "must not be empty")
	}
	title, err := NormalizeTitle(ev.Title)
	if err != nil {
		return false, err
	}
	description, err := NormalizeDescription(ev.Description)
	if err != nil {
		return false, err
	}

	now := s.now()
	return s.repo.InsertTaskIfAbsent(ctx, Task{
		ID:                     s.newID(),
		Provider:               provider,
		ExternalID:             ev.ExternalID,
		ExternalURL:            ev.URL,
		Title:                  title,
		Description:            description,
		AdditionalInstructions: []string{},
		Labels:                 []string{},
		Status:                 StatusDraft,
		RawPayload:             ev.Raw,
		CreatedAt:              now,
		UpdatedAt:              now,
	})
}

// ListLabels returns every label name in alphabetical order.
func (s *Service) ListLabels(ctx context.Context) ([]string, error) {
	return s.repo.ListLabels(ctx)
}

// CategoryInput is the data for a new project category. An empty Slug is
// derived from Name, and an empty BaseBranch is DefaultBaseBranch.
type CategoryInput struct {
	Name        string
	Slug        string
	Description string
	RepoURL     *string
	BaseBranch  string
}

// CreateCategory creates a project category with a unique slug.
func (s *Service) CreateCategory(ctx context.Context, in CategoryInput) (ProjectCategory, error) {
	name, err := NormalizeCategoryName(in.Name)
	if err != nil {
		return ProjectCategory{}, err
	}
	slug := in.Slug
	if slug == "" {
		slug = Slugify(name)
	}
	if err := ValidateSlug(slug); err != nil {
		return ProjectCategory{}, err
	}
	if in.RepoURL != nil {
		if err := ValidateRepoURL(*in.RepoURL); err != nil {
			return ProjectCategory{}, err
		}
	}
	branch := in.BaseBranch
	if branch == "" {
		branch = DefaultBaseBranch
	}
	if err := ValidateBaseBranch(branch); err != nil {
		return ProjectCategory{}, err
	}

	now := s.now()
	c := ProjectCategory{
		ID:          s.newID(),
		Name:        name,
		Slug:        slug,
		Description: strings.TrimSpace(in.Description),
		RepoURL:     in.RepoURL,
		BaseBranch:  branch,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := s.repo.InsertCategory(ctx, c); err != nil {
		return ProjectCategory{}, err
	}
	return c, nil
}

// GetCategory returns the project category with id.
func (s *Service) GetCategory(ctx context.Context, id uuid.UUID) (ProjectCategory, error) {
	return s.repo.GetCategory(ctx, id)
}

// ListCategories returns every project category ordered by name.
func (s *Service) ListCategories(ctx context.Context) ([]ProjectCategory, error) {
	return s.repo.ListCategories(ctx)
}

// CategoryPatch holds the fields of a partial category update; nil fields are
// left unchanged. RepoURL is applied only when SetRepoURL is true, and a nil
// value then removes the repository.
type CategoryPatch struct {
	Name        *string
	Slug        *string
	Description *string
	SetRepoURL  bool
	RepoURL     *string
	BaseBranch  *string
}

// UpdateCategory applies a partial update with the same rules as creation.
// The repository cannot be removed while a task of the category is ready.
func (s *Service) UpdateCategory(ctx context.Context, id uuid.UUID, p CategoryPatch) (ProjectCategory, error) {
	if p.SetRepoURL && p.RepoURL == nil {
		_, ready, err := s.repo.ListTasks(ctx, ListFilter{ProjectCategoryID: &id, Status: new(StatusReady), Limit: 1})
		if err != nil {
			return ProjectCategory{}, err
		}
		if ready > 0 {
			return ProjectCategory{}, fmt.Errorf("%w: project category has ready tasks that need its repo_url", ErrConflict)
		}
	}
	return s.repo.UpdateCategory(ctx, id, func(c *ProjectCategory) error {
		if p.Name != nil {
			name, err := NormalizeCategoryName(*p.Name)
			if err != nil {
				return err
			}
			c.Name = name
		}
		if p.Slug != nil {
			if err := ValidateSlug(*p.Slug); err != nil {
				return err
			}
			c.Slug = *p.Slug
		}
		if p.Description != nil {
			c.Description = strings.TrimSpace(*p.Description)
		}
		if p.SetRepoURL {
			if p.RepoURL != nil {
				if err := ValidateRepoURL(*p.RepoURL); err != nil {
					return err
				}
			}
			c.RepoURL = p.RepoURL
		}
		if p.BaseBranch != nil {
			if err := ValidateBaseBranch(*p.BaseBranch); err != nil {
				return err
			}
			c.BaseBranch = *p.BaseBranch
		}
		c.UpdatedAt = s.now()
		return nil
	})
}

// DeleteCategory removes a project category that no task references.
func (s *Service) DeleteCategory(ctx context.Context, id uuid.UUID) error {
	return s.repo.DeleteCategory(ctx, id)
}

func categoryNotFoundAsValidation(err error) error {
	if errors.Is(err, ErrCategoryNotFound) {
		return invalid("project_category_id", "project category does not exist")
	}
	return err
}
