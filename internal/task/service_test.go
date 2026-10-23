package task_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/google/uuid"

	"github.com/jrgomesdev/miner/internal/task"
	"github.com/jrgomesdev/miner/internal/task/tasktest"
)

func newService(t *testing.T) (*task.Service, *tasktest.Repository) {
	t.Helper()
	repo := tasktest.NewRepository()
	return task.NewService(repo), repo
}

func TestCreateLocal(t *testing.T) {
	ctx := context.Background()
	svc, _ := newService(t)

	got, err := svc.CreateLocal(ctx, task.CreateInput{
		Title:       " Adicionar paginação ",
		Description: "Implemente ...",
		Labels:      []string{"Backend", "backend"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Provider != task.ProviderLocal || got.ExternalID != got.ID.String() || got.Status != task.StatusDraft {
		t.Errorf("unexpected task: %+v", got)
	}
	if got.Title != "Adicionar paginação" || !slices.Equal(got.Labels, []string{"backend"}) {
		t.Errorf("fields not normalized: %+v", got)
	}
	if got.AdditionalInstructions == nil {
		t.Error("AdditionalInstructions is nil, want empty slice")
	}

	_, err = svc.CreateLocal(ctx, task.CreateInput{Title: "x"})
	assertValidation(t, err, "description")

	_, err = svc.CreateLocal(ctx, task.CreateInput{Title: "x", Description: "y", ProjectCategoryID: new(uuid.New())})
	assertValidation(t, err, "project_category_id")
}

func TestUpdateFieldOwnership(t *testing.T) {
	ctx := context.Background()
	svc, _ := newService(t)

	if _, err := svc.ApplyEvent(ctx, "jira", task.Event{
		Type:        task.EventCreated,
		ExternalID:  "PROJ-1",
		Title:       "Ticket",
		Description: "Prompt",
	}); err != nil {
		t.Fatal(err)
	}
	tasks, _, _ := svc.List(ctx, task.ListFilter{Limit: 10})
	external := tasks[0]

	_, err := svc.Update(ctx, external.ID, task.UpdateInput{Description: new("novo")})
	if !errors.Is(err, task.ErrFieldNotEditable) {
		t.Fatalf("error = %v, want ErrFieldNotEditable", err)
	}

	got, err := svc.Update(ctx, external.ID, task.UpdateInput{
		AdditionalInstructions: &[]string{"Use a lib X"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Description != "Prompt" || !slices.Equal(got.AdditionalInstructions, []string{"Use a lib X"}) {
		t.Errorf("unexpected task after update: %+v", got)
	}

	local, _ := svc.CreateLocal(ctx, task.CreateInput{Title: "Local", Description: "a"})
	got, err = svc.Update(ctx, local.ID, task.UpdateInput{Description: new("novo prompt")})
	if err != nil {
		t.Fatal(err)
	}
	if got.Description != "novo prompt" || got.UpdatedAt.Before(local.UpdatedAt) {
		t.Errorf("description not updated: %+v", got)
	}
}

func TestUpdateStatus(t *testing.T) {
	ctx := context.Background()
	svc, _ := newService(t)
	created, _ := svc.CreateLocal(ctx, task.CreateInput{Title: "T", Description: "D"})

	_, err := svc.Update(ctx, created.ID, task.UpdateInput{Status: new(task.StatusDone)})
	if !errors.Is(err, task.ErrInvalidTransition) {
		t.Fatalf("draft -> done error = %v, want ErrInvalidTransition", err)
	}

	_, err = svc.Update(ctx, created.ID, task.UpdateInput{Status: new(task.Status("archived"))})
	assertValidation(t, err, "status")

	if _, err := svc.Update(ctx, created.ID, task.UpdateInput{Status: new(task.StatusDraft)}); err != nil {
		t.Errorf("same status error = %v, want nil", err)
	}

	got, err := svc.Update(ctx, created.ID, task.UpdateInput{Status: new(task.StatusReady)})
	if err != nil || got.Status != task.StatusReady {
		t.Fatalf("draft -> ready = %v, %v", got.Status, err)
	}

	// A failed update must not change anything, even fields that were valid.
	_, err = svc.Update(ctx, created.ID, task.UpdateInput{
		Labels: &[]string{"x"},
		Status: new(task.StatusDone),
	})
	if !errors.Is(err, task.ErrInvalidTransition) {
		t.Fatalf("ready -> done error = %v", err)
	}
	got, _ = svc.Get(ctx, created.ID)
	if len(got.Labels) != 0 || got.Status != task.StatusReady {
		t.Errorf("failed update changed the task: %+v", got)
	}
}

func TestUpdateCategory(t *testing.T) {
	ctx := context.Background()
	svc, _ := newService(t)
	cat, _ := svc.CreateCategory(ctx, task.CategoryInput{Name: "Backend"})
	created, _ := svc.CreateLocal(ctx, task.CreateInput{Title: "T", Description: "D", ProjectCategoryID: &cat.ID})

	if created.ProjectCategory == nil || created.ProjectCategory.Slug != "backend" {
		t.Fatalf("category not resolved: %+v", created.ProjectCategory)
	}

	got, err := svc.Update(ctx, created.ID, task.UpdateInput{SetProjectCategory: true})
	if err != nil || got.ProjectCategory != nil {
		t.Fatalf("clearing category = %+v, %v", got.ProjectCategory, err)
	}

	_, err = svc.Update(ctx, created.ID, task.UpdateInput{SetProjectCategory: true, ProjectCategoryID: new(uuid.New())})
	assertValidation(t, err, "project_category_id")
}

func TestApplyEvent(t *testing.T) {
	ctx := context.Background()
	svc, _ := newService(t)
	ev := task.Event{Type: task.EventCreated, ExternalID: "abc", Title: "T", Description: "D", Raw: []byte(`{}`)}

	created, err := svc.ApplyEvent(ctx, task.ProviderLocal, ev)
	if err != nil || !created {
		t.Fatalf("first event = %v, %v", created, err)
	}
	tasks, _, _ := svc.List(ctx, task.ListFilter{Limit: 10})
	if _, err := svc.Update(ctx, tasks[0].ID, task.UpdateInput{Labels: &[]string{"editado"}}); err != nil {
		t.Fatal(err)
	}

	created, err = svc.ApplyEvent(ctx, task.ProviderLocal, ev)
	if err != nil || created {
		t.Fatalf("redelivery = %v, %v, want false, nil", created, err)
	}
	tasks, total, _ := svc.List(ctx, task.ListFilter{Limit: 10})
	if total != 1 || !slices.Equal(tasks[0].Labels, []string{"editado"}) {
		t.Errorf("redelivery changed tasks: total=%d %+v", total, tasks)
	}

	for _, typ := range []task.EventType{task.EventUpdated, task.EventClosed} {
		created, err := svc.ApplyEvent(ctx, task.ProviderLocal, task.Event{Type: typ, ExternalID: "new"})
		if err != nil || created {
			t.Errorf("%s event = %v, %v, want ignored", typ, created, err)
		}
	}

	_, err = svc.ApplyEvent(ctx, task.ProviderLocal, task.Event{Type: task.EventCreated, ExternalID: "x", Description: "D"})
	assertValidation(t, err, "title")
}

func TestCategories(t *testing.T) {
	ctx := context.Background()
	svc, _ := newService(t)

	c, err := svc.CreateCategory(ctx, task.CategoryInput{Name: "Integrações Back-end"})
	if err != nil || c.Slug != "integracoes-back-end" {
		t.Fatalf("derived slug = %q, %v", c.Slug, err)
	}

	if _, err := svc.CreateCategory(ctx, task.CategoryInput{Name: "Backend"}); err != nil {
		t.Fatal(err)
	}
	_, err = svc.CreateCategory(ctx, task.CategoryInput{Name: "backend"})
	if !errors.Is(err, task.ErrConflict) {
		t.Errorf("duplicate slug error = %v, want ErrConflict", err)
	}

	_, err = svc.CreateCategory(ctx, task.CategoryInput{Name: "X", Slug: "Com Espaço"})
	assertValidation(t, err, "slug")

	_, err = svc.CreateCategory(ctx, task.CategoryInput{Name: "!!!"})
	assertValidation(t, err, "slug")

	if _, err := svc.CreateLocal(ctx, task.CreateInput{Title: "T", Description: "D", ProjectCategoryID: &c.ID}); err != nil {
		t.Fatal(err)
	}
	if err := svc.DeleteCategory(ctx, c.ID); !errors.Is(err, task.ErrConflict) {
		t.Errorf("deleting used category error = %v, want ErrConflict", err)
	}

	renamed, err := svc.UpdateCategory(ctx, c.ID, task.CategoryPatch{Name: new("Plataforma")})
	if err != nil || renamed.Name != "Plataforma" || renamed.Slug != "integracoes-back-end" {
		t.Errorf("rename = %+v, %v", renamed, err)
	}
}

func assertValidation(t *testing.T, err error, field string) {
	t.Helper()
	var ve *task.ValidationError
	if !errors.As(err, &ve) || ve.Field != field {
		t.Errorf("error = %v, want validation error on %q", err, field)
	}
}
