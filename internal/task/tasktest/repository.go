// Package tasktest provides an in-memory task.Repository for tests.
package tasktest

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/google/uuid"

	"github.com/jrgomesdev/miner/internal/task"
)

// Repository is an in-memory task.Repository that mimics the constraints of
// the PostgreSQL schema. Set Err to make every call fail.
type Repository struct {
	mu         sync.Mutex
	tasks      map[uuid.UUID]task.Task
	categories map[uuid.UUID]task.ProjectCategory
	labels     map[string]bool
	Err        error
}

// NewRepository returns an empty repository.
func NewRepository() *Repository {
	return &Repository{
		tasks:      map[uuid.UUID]task.Task{},
		categories: map[uuid.UUID]task.ProjectCategory{},
		labels:     map[string]bool{},
	}
}

var _ task.Repository = (*Repository)(nil)

func (r *Repository) InsertTask(_ context.Context, t task.Task) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return r.Err
	}
	if r.findExternal(t.Provider, t.ExternalID) {
		return fmt.Errorf("%w: duplicate task", task.ErrConflict)
	}
	return r.store(t)
}

func (r *Repository) InsertTaskIfAbsent(_ context.Context, t task.Task) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return false, r.Err
	}
	if r.findExternal(t.Provider, t.ExternalID) {
		return false, nil
	}
	return true, r.store(t)
}

func (r *Repository) GetTask(_ context.Context, id uuid.UUID) (task.Task, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return task.Task{}, r.Err
	}
	t, ok := r.tasks[id]
	if !ok {
		return task.Task{}, task.ErrNotFound
	}
	return r.resolve(t), nil
}

func (r *Repository) ListTasks(_ context.Context, f task.ListFilter) ([]task.Task, int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return nil, 0, r.Err
	}
	matched := []task.Task{}
	for _, t := range r.tasks {
		if matches(t, f) {
			matched = append(matched, r.resolve(t))
		}
	}
	slices.SortFunc(matched, func(a, b task.Task) int { return b.CreatedAt.Compare(a.CreatedAt) })

	total := len(matched)
	start := min(f.Offset, total)
	end := min(start+f.Limit, total)
	return matched[start:end], total, nil
}

func (r *Repository) UpdateTask(_ context.Context, id uuid.UUID, fn func(*task.Task) error) (task.Task, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return task.Task{}, r.Err
	}
	t, ok := r.tasks[id]
	if !ok {
		return task.Task{}, task.ErrNotFound
	}
	t = clone(r.resolve(t))
	if err := fn(&t); err != nil {
		return task.Task{}, err
	}
	if err := r.store(t); err != nil {
		return task.Task{}, err
	}
	return r.resolve(r.tasks[id]), nil
}

func (r *Repository) DeleteTask(_ context.Context, id uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return r.Err
	}
	if _, ok := r.tasks[id]; !ok {
		return task.ErrNotFound
	}
	delete(r.tasks, id)
	return nil
}

func (r *Repository) ListLabels(context.Context) ([]string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return nil, r.Err
	}
	names := []string{}
	for name := range r.labels {
		names = append(names, name)
	}
	slices.Sort(names)
	return names, nil
}

func (r *Repository) InsertCategory(_ context.Context, c task.ProjectCategory) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return r.Err
	}
	if r.slugTaken(c.Slug, c.ID) {
		return fmt.Errorf("%w: slug %q already in use", task.ErrConflict, c.Slug)
	}
	r.categories[c.ID] = c
	return nil
}

func (r *Repository) GetCategory(_ context.Context, id uuid.UUID) (task.ProjectCategory, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return task.ProjectCategory{}, r.Err
	}
	c, ok := r.categories[id]
	if !ok {
		return task.ProjectCategory{}, task.ErrNotFound
	}
	return c, nil
}

func (r *Repository) ListCategories(context.Context) ([]task.ProjectCategory, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return nil, r.Err
	}
	out := []task.ProjectCategory{}
	for _, c := range r.categories {
		out = append(out, c)
	}
	slices.SortFunc(out, func(a, b task.ProjectCategory) int { return strings.Compare(a.Name, b.Name) })
	return out, nil
}

func (r *Repository) UpdateCategory(
	_ context.Context,
	id uuid.UUID,
	fn func(*task.ProjectCategory) error,
) (task.ProjectCategory, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return task.ProjectCategory{}, r.Err
	}
	c, ok := r.categories[id]
	if !ok {
		return task.ProjectCategory{}, task.ErrNotFound
	}
	if err := fn(&c); err != nil {
		return task.ProjectCategory{}, err
	}
	if r.slugTaken(c.Slug, c.ID) {
		return task.ProjectCategory{}, fmt.Errorf("%w: slug %q already in use", task.ErrConflict, c.Slug)
	}
	r.categories[id] = c
	return c, nil
}

func (r *Repository) DeleteCategory(_ context.Context, id uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return r.Err
	}
	if _, ok := r.categories[id]; !ok {
		return task.ErrNotFound
	}
	for _, t := range r.tasks {
		if t.ProjectCategory != nil && t.ProjectCategory.ID == id {
			return fmt.Errorf("%w: project category is used by tasks", task.ErrConflict)
		}
	}
	delete(r.categories, id)
	return nil
}

// store saves t, checking the category reference and registering its labels.
func (r *Repository) store(t task.Task) error {
	if t.ProjectCategory != nil {
		if _, ok := r.categories[t.ProjectCategory.ID]; !ok {
			return task.ErrCategoryNotFound
		}
	}
	for _, l := range t.Labels {
		r.labels[l] = true
	}
	r.tasks[t.ID] = clone(t)
	return nil
}

func (r *Repository) findExternal(provider, externalID string) bool {
	for _, t := range r.tasks {
		if t.Provider == provider && t.ExternalID == externalID {
			return true
		}
	}
	return false
}

func (r *Repository) slugTaken(slug string, except uuid.UUID) bool {
	for _, c := range r.categories {
		if c.Slug == slug && c.ID != except {
			return true
		}
	}
	return false
}

// resolve fills the category name, slug and repository from the current
// category, as the SQL join does.
func (r *Repository) resolve(t task.Task) task.Task {
	t = clone(t)
	if t.ProjectCategory != nil {
		c := r.categories[t.ProjectCategory.ID]
		t.ProjectCategory = &task.CategoryRef{ID: c.ID, Slug: c.Slug, Name: c.Name, RepoURL: c.RepoURL}
	}
	return t
}

func matches(t task.Task, f task.ListFilter) bool {
	switch {
	case f.Status != nil && t.Status != *f.Status:
		return false
	case f.Provider != nil && t.Provider != *f.Provider:
		return false
	case f.ProjectCategoryID != nil && (t.ProjectCategory == nil || t.ProjectCategory.ID != *f.ProjectCategoryID):
		return false
	case f.Label != nil && !slices.Contains(t.Labels, *f.Label):
		return false
	}
	return true
}

func clone(t task.Task) task.Task {
	t.AdditionalInstructions = append([]string{}, t.AdditionalInstructions...)
	t.Labels = append([]string{}, t.Labels...)
	if t.ProjectCategory != nil {
		ref := *t.ProjectCategory
		t.ProjectCategory = &ref
	}
	e := &t.Execution
	e.Agent, e.Model, e.Effort, e.Runner = copyPtr(e.Agent), copyPtr(e.Model), copyPtr(e.Effort), copyPtr(e.Runner)
	return t
}

func copyPtr[T any](p *T) *T {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}
