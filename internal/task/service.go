package task

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Service holds the Task domain rules on top of a Repository.
type Service struct {
	repo  Repository
	now   func() time.Time
	newID func() uuid.UUID
}

// NewService returns a service that stores data in repo.
func NewService(repo Repository) *Service {
	return &Service{
		repo: repo,
		// PostgreSQL keeps microseconds; truncating keeps reads equal to writes.
		now:   func() time.Time { return time.Now().UTC().Truncate(time.Microsecond) },
		newID: func() uuid.UUID { return uuid.Must(uuid.NewV7()) },
	}
}

// CreateInput is the data for a task created directly, without a provider.
type CreateInput struct {
	Title                  string
	Description            string
	AdditionalInstructions []string
	Labels                 []string
	ProjectCategoryID      *uuid.UUID
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
// unchanged. ProjectCategoryID is applied only when SetProjectCategory is true,
// and a nil value then removes the category.
type UpdateInput struct {
	Title                  *string
	Description            *string
	AdditionalInstructions *[]string
	Labels                 *[]string
	SetProjectCategory     bool
	ProjectCategoryID      *uuid.UUID
	Status                 *Status
}

func (in UpdateInput) empty() bool {
	return in == (UpdateInput{})
}

// Update applies a partial update. Title and description belong to the
// provider and may only change on local tasks; status changes must follow
// the lifecycle.
func (s *Service) Update(ctx context.Context, id uuid.UUID, in UpdateInput) (Task, error) {
	if in.empty() {
		return s.repo.GetTask(ctx, id)
	}
	if in.Status != nil && !in.Status.Valid() {
		return Task{}, invalid("status", "unknown status %q", *in.Status)
	}

	t, err := s.repo.UpdateTask(ctx, id, func(t *Task) error {
		return s.apply(t, in)
	})
	if err != nil {
		return Task{}, categoryNotFoundAsValidation(err)
	}
	return t, nil
}

// apply validates in against t and changes t in place.
func (s *Service) apply(t *Task, in UpdateInput) error {
	touchesProviderFields := in.Title != nil || in.Description != nil
	if touchesProviderFields && t.Provider != ProviderLocal {
		return fmt.Errorf("%w: title and description belong to provider %q", ErrFieldNotEditable, t.Provider)
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
		t.ProjectCategory = nil
		if in.ProjectCategoryID != nil {
			t.ProjectCategory = &CategoryRef{ID: *in.ProjectCategoryID}
		}
	}
	if in.Status != nil && *in.Status != t.Status {
		if !t.Status.CanTransitionTo(*in.Status) {
			return fmt.Errorf("%w: %s -> %s", ErrInvalidTransition, t.Status, *in.Status)
		}
		t.Status = *in.Status
	}

	t.UpdatedAt = s.now()
	return nil
}

// Delete removes the task with id.
func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {
	return s.repo.DeleteTask(ctx, id)
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
// derived from Name.
type CategoryInput struct {
	Name        string
	Slug        string
	Description string
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

	now := s.now()
	c := ProjectCategory{
		ID:          s.newID(),
		Name:        name,
		Slug:        slug,
		Description: strings.TrimSpace(in.Description),
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
// left unchanged.
type CategoryPatch struct {
	Name        *string
	Slug        *string
	Description *string
}

// UpdateCategory applies a partial update with the same rules as creation.
func (s *Service) UpdateCategory(ctx context.Context, id uuid.UUID, p CategoryPatch) (ProjectCategory, error) {
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
