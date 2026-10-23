package task

import (
	"context"

	"github.com/google/uuid"
)

// ListFilter selects tasks; nil fields do not filter. Limit and Offset are
// already validated.
type ListFilter struct {
	Status            *Status
	Provider          *string
	ProjectCategoryID *uuid.UUID
	Label             *string
	Limit             int
	Offset            int
}

// Repository persists tasks, project categories and labels.
//
// Implementations return ErrNotFound for missing rows, ErrCategoryNotFound when
// a task references a project category that does not exist and ErrConflict for
// a duplicate slug or a category still referenced by tasks.
type Repository interface {
	// InsertTask stores a new task together with its labels.
	InsertTask(ctx context.Context, t Task) error
	// InsertTaskIfAbsent stores t unless a task with the same provider and
	// external id exists, reporting whether it was inserted.
	InsertTaskIfAbsent(ctx context.Context, t Task) (bool, error)
	GetTask(ctx context.Context, id uuid.UUID) (Task, error)
	// ListTasks returns a page of tasks, newest first, and the total matching
	// the filter.
	ListTasks(ctx context.Context, f ListFilter) ([]Task, int, error)
	// UpdateTask loads the task, applies fn and stores every user-editable
	// field, title, description and labels atomically. Nothing is stored when
	// fn returns an error.
	UpdateTask(ctx context.Context, id uuid.UUID, fn func(*Task) error) (Task, error)
	DeleteTask(ctx context.Context, id uuid.UUID) error
	ListLabels(ctx context.Context) ([]string, error)

	InsertCategory(ctx context.Context, c ProjectCategory) error
	GetCategory(ctx context.Context, id uuid.UUID) (ProjectCategory, error)
	ListCategories(ctx context.Context) ([]ProjectCategory, error)
	// UpdateCategory loads the category, applies fn and stores it atomically.
	UpdateCategory(ctx context.Context, id uuid.UUID, fn func(*ProjectCategory) error) (ProjectCategory, error)
	DeleteCategory(ctx context.Context, id uuid.UUID) error
}
