// Package task is the Task domain: tasks that feed the autonomous development
// agent, their project categories and labels, and the rules for changing them.
package task

import (
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// ProviderLocal is the generic source: tasks created without a ticket in an
// external provider, local tests and sources without a dedicated adapter.
const ProviderLocal = "local"

// Task is a unit of work for the agent. Provider, ExternalID, ExternalURL,
// Title, Description and RawPayload belong to the provider; the remaining
// fields belong to the user.
type Task struct {
	ID          uuid.UUID
	Provider    string
	ExternalID  string
	ExternalURL *string
	Title       string
	// Description is the prompt: instructions, acceptance criteria and so on.
	Description            string
	AdditionalInstructions []string
	// Labels are normalized names in alphabetical order.
	Labels          []string
	ProjectCategory *CategoryRef
	Status          Status
	Execution       Execution
	RawPayload      []byte
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// Execution is the user's choice of what runs the task and where. Each field
// is nil until set and must match the execution catalog.
type Execution struct {
	Agent  *string
	Model  *string
	Effort *string
	Runner *string
}

// CategoryRef is the summary of a project category embedded in a task.
type CategoryRef struct {
	ID      uuid.UUID
	Slug    string
	Name    string
	RepoURL *string
}

// ProjectCategory groups tasks by project. RepoURL is the repository the
// agent clones, and BaseBranch the branch it starts from.
type ProjectCategory struct {
	ID          uuid.UUID
	Name        string
	Slug        string
	Description string
	RepoURL     *string
	BaseBranch  string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// DefaultBaseBranch is the base branch of a category created without one.
const DefaultBaseBranch = "main"

// Status is the lifecycle state of a task.
type Status string

const (
	StatusDraft      Status = "draft"
	StatusReady      Status = "ready"
	StatusInProgress Status = "in_progress"
	StatusInReview   Status = "in_review"
	StatusDone       Status = "done"
	StatusFailed     Status = "failed"
	StatusCancelled  Status = "cancelled"
)

// transitions lists, for each status, the statuses it may move to.
var transitions = map[Status][]Status{
	StatusDraft:      {StatusReady, StatusCancelled},
	StatusReady:      {StatusInProgress, StatusDraft, StatusCancelled},
	StatusInProgress: {StatusInReview, StatusFailed, StatusCancelled},
	StatusInReview:   {StatusDone, StatusReady, StatusCancelled},
	StatusFailed:     {StatusReady, StatusCancelled},
	StatusDone:       nil,
	StatusCancelled:  nil,
}

// Valid reports whether s is a known status.
func (s Status) Valid() bool {
	_, ok := transitions[s]
	return ok
}

// locksExecution reports whether the execution configuration of a task in s
// is frozen: the task is running or has ended.
func (s Status) locksExecution() bool {
	return s == StatusInProgress || s == StatusDone || s == StatusCancelled
}

// CanTransitionTo reports whether a task may move from s to to. Staying in the
// same status is not a transition and returns false.
func (s Status) CanTransitionTo(to Status) bool {
	for _, allowed := range transitions[s] {
		if allowed == to {
			return true
		}
	}
	return false
}

var (
	// ErrNotFound means the task or category does not exist.
	ErrNotFound = errors.New("not found")
	// ErrConflict means the change clashes with existing data, such as a
	// duplicate slug or a category still in use.
	ErrConflict = errors.New("conflict")
	// ErrInvalidTransition means the requested status change is not allowed.
	ErrInvalidTransition = errors.New("invalid status transition")
	// ErrFieldNotEditable means the field belongs to the task's provider.
	ErrFieldNotEditable = errors.New("field not editable")
	// ErrCategoryNotFound is returned by repositories when a task references a
	// project category that does not exist.
	ErrCategoryNotFound = errors.New("project category not found")
)

// ValidationError reports an invalid input value.
type ValidationError struct {
	Field string
	Msg   string
}

func (e *ValidationError) Error() string { return fmt.Sprintf("%s: %s", e.Field, e.Msg) }

func invalid(field, format string, args ...any) error {
	return &ValidationError{Field: field, Msg: fmt.Sprintf(format, args...)}
}
