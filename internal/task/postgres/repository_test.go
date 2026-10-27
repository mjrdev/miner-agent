package postgres_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/jrgomesdev/miner/internal/task"
	"github.com/jrgomesdev/miner/internal/task/postgres"
	"github.com/jrgomesdev/miner/internal/task/tasktest"
	"github.com/jrgomesdev/miner/internal/testdb"
)

func newService(t *testing.T) *task.Service {
	t.Helper()
	return task.NewService(postgres.NewRepository(testdb.Pool(t)), tasktest.Catalog())
}

func TestApplyEventIsIdempotent(t *testing.T) {
	ctx := context.Background()
	svc := newService(t)
	ev := task.Event{
		Type:        task.EventCreated,
		ExternalID:  "abc",
		URL:         new("https://example.com/abc"),
		Title:       "T",
		Description: "D",
		Raw:         []byte(`{"title":"T"}`),
	}

	for i, want := range []bool{true, false} {
		created, err := svc.ApplyEvent(ctx, task.ProviderLocal, ev)
		if err != nil || created != want {
			t.Fatalf("delivery %d = %v, %v, want %v", i+1, created, err, want)
		}
	}

	tasks, total, err := svc.List(ctx, task.ListFilter{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || tasks[0].ExternalID != "abc" || *tasks[0].ExternalURL != "https://example.com/abc" {
		t.Errorf("unexpected tasks: total=%d %+v", total, tasks)
	}
}

func TestListFilters(t *testing.T) {
	ctx := context.Background()
	svc := newService(t)
	cat, err := svc.CreateCategory(ctx, task.CategoryInput{Name: "Backend", RepoURL: new("https://github.com/org/repo")})
	if err != nil {
		t.Fatal(err)
	}

	mk := func(title string, labels []string, ready bool) task.Task {
		t.Helper()
		created, err := svc.CreateLocal(ctx, task.CreateInput{
			Title:             title,
			Description:       "D",
			Labels:            labels,
			ProjectCategoryID: &cat.ID,
			Execution:         task.Execution{Agent: new("claude"), Model: new("opus"), Effort: new("high"), Runner: new("host")},
		})
		if err != nil {
			t.Fatal(err)
		}
		if ready {
			created, err = svc.Update(ctx, created.ID, task.UpdateInput{Status: new(task.StatusReady)})
			if err != nil {
				t.Fatal(err)
			}
		}
		// Distinct creation times keep the newest-first order deterministic.
		time.Sleep(2 * time.Millisecond)
		return created
	}
	mk("draft backend", []string{"backend"}, false)
	readyBackend := mk("ready backend", []string{"backend", "api"}, true)
	mk("ready other", []string{"frontend"}, true)
	newest := mk("draft other", nil, false)

	got, total, err := svc.List(ctx, task.ListFilter{Status: new(task.StatusReady), Label: new("backend"), Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(got) != 1 || got[0].ID != readyBackend.ID {
		t.Fatalf("filtered = total %d %+v", total, got)
	}
	if !slices.Equal(got[0].Labels, []string{"api", "backend"}) || got[0].ProjectCategory.Slug != "backend" {
		t.Errorf("labels/category not loaded: %+v", got[0])
	}

	page, total, err := svc.List(ctx, task.ListFilter{ProjectCategoryID: &cat.ID, Limit: 2, Offset: 0})
	if err != nil || total != 4 || len(page) != 2 || page[0].ID != newest.ID {
		t.Errorf("page = total %d len %d first %v, err %v", total, len(page), page[0].Title, err)
	}

	beyond, total, err := svc.List(ctx, task.ListFilter{Limit: 10, Offset: 10})
	if err != nil || total != 4 || len(beyond) != 0 {
		t.Errorf("offset beyond total = total %d len %d, err %v", total, len(beyond), err)
	}
}

func TestUpdateReplacesLabelsAndInstructions(t *testing.T) {
	ctx := context.Background()
	svc := newService(t)
	created, err := svc.CreateLocal(ctx, task.CreateInput{Title: "T", Description: "D", Labels: []string{"a", "b"}})
	if err != nil {
		t.Fatal(err)
	}

	got, err := svc.Update(ctx, created.ID, task.UpdateInput{
		Labels:                 &[]string{"C", "b"},
		AdditionalInstructions: &[]string{"passo 2", "passo 1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got.Labels, []string{"b", "c"}) {
		t.Errorf("labels = %v", got.Labels)
	}
	if !slices.Equal(got.AdditionalInstructions, []string{"passo 2", "passo 1"}) {
		t.Errorf("instructions = %v", got.AdditionalInstructions)
	}
	if !got.UpdatedAt.After(created.UpdatedAt) {
		t.Errorf("updated_at not advanced")
	}

	labels, err := svc.ListLabels(ctx)
	if err != nil || !slices.Equal(labels, []string{"a", "b", "c"}) {
		t.Errorf("ListLabels() = %v, %v", labels, err)
	}

	got, err = svc.Update(ctx, created.ID, task.UpdateInput{Labels: &[]string{}})
	if err != nil || len(got.Labels) != 0 {
		t.Errorf("clearing labels = %v, %v", got.Labels, err)
	}

	// A rejected update leaves the row untouched.
	_, err = svc.Update(ctx, created.ID, task.UpdateInput{Labels: &[]string{"x"}, Status: new(task.StatusDone)})
	if !errors.Is(err, task.ErrInvalidTransition) {
		t.Fatalf("error = %v", err)
	}
	if got, _ := svc.Get(ctx, created.ID); len(got.Labels) != 0 {
		t.Errorf("rejected update stored labels: %v", got.Labels)
	}
}

func TestCategoryConstraints(t *testing.T) {
	ctx := context.Background()
	svc := newService(t)

	cat, err := svc.CreateCategory(ctx, task.CategoryInput{Name: "Backend"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateCategory(ctx, task.CategoryInput{Name: "backend"}); !errors.Is(err, task.ErrConflict) {
		t.Errorf("duplicate slug error = %v", err)
	}

	_, err = svc.CreateLocal(ctx, task.CreateInput{Title: "T", Description: "D", ProjectCategoryID: new(uuid.New())})
	var ve *task.ValidationError
	if !errors.As(err, &ve) || ve.Field != "project_category_id" {
		t.Errorf("unknown category error = %v", err)
	}

	created, err := svc.CreateLocal(ctx, task.CreateInput{Title: "T", Description: "D", ProjectCategoryID: &cat.ID})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.DeleteCategory(ctx, cat.ID); !errors.Is(err, task.ErrConflict) {
		t.Errorf("deleting used category error = %v", err)
	}

	if _, err := svc.UpdateCategory(ctx, cat.ID, task.CategoryPatch{Name: new("Plataforma")}); err != nil {
		t.Fatal(err)
	}
	if got, _ := svc.Get(ctx, created.ID); got.ProjectCategory.Name != "Plataforma" {
		t.Errorf("task category name = %q", got.ProjectCategory.Name)
	}

	if err := svc.Delete(ctx, created.ID); err != nil {
		t.Fatal(err)
	}
	if err := svc.DeleteCategory(ctx, cat.ID); err != nil {
		t.Errorf("deleting unused category: %v", err)
	}
	if _, err := svc.GetCategory(ctx, cat.ID); !errors.Is(err, task.ErrNotFound) {
		t.Errorf("deleted category error = %v", err)
	}
	if err := svc.Delete(ctx, created.ID); !errors.Is(err, task.ErrNotFound) {
		t.Errorf("deleting missing task error = %v", err)
	}
}

func TestExecutionAndRepositoryRoundTrip(t *testing.T) {
	ctx := context.Background()
	svc := newService(t)

	plain, err := svc.CreateCategory(ctx, task.CategoryInput{Name: "Plain"})
	if err != nil {
		t.Fatal(err)
	}
	if got, err := svc.GetCategory(ctx, plain.ID); err != nil || got.BaseBranch != "main" || got.RepoURL != nil {
		t.Errorf("defaults = %+v, %v", got, err)
	}

	cat, err := svc.CreateCategory(ctx, task.CategoryInput{
		Name: "Miner", RepoURL: new("https://github.com/mjrdev/miner-agent"), BaseBranch: "develop",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := svc.GetCategory(ctx, cat.ID); *got.RepoURL != "https://github.com/mjrdev/miner-agent" || got.BaseBranch != "develop" {
		t.Errorf("category = %+v", got)
	}

	created, err := svc.CreateLocal(ctx, task.CreateInput{
		Title:       "T",
		Description: "D",
		Execution:   task.Execution{Agent: new("claude"), Model: new("opus"), Effort: new("high"), Runner: new("host")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if e := created.Execution; *e.Agent != "claude" || *e.Model != "opus" || *e.Effort != "high" || *e.Runner != "host" {
		t.Errorf("created execution = %+v", e)
	}

	got, err := svc.Update(ctx, created.ID, task.UpdateInput{
		SetProjectCategory: true, ProjectCategoryID: &cat.ID,
		SetEffort: true, Effort: new("low"), Status: new(task.StatusReady),
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != task.StatusReady || *got.Execution.Effort != "low" ||
		*got.ProjectCategory.RepoURL != "https://github.com/mjrdev/miner-agent" {
		t.Errorf("updated = %+v", got)
	}

	if _, err := svc.UpdateCategory(ctx, cat.ID, task.CategoryPatch{SetRepoURL: true}); !errors.Is(err, task.ErrConflict) {
		t.Errorf("removing repository in use = %v, want ErrConflict", err)
	}
	if _, err := svc.UpdateCategory(ctx, cat.ID, task.CategoryPatch{
		SetRepoURL: true, RepoURL: new("https://github.com/org/other"), BaseBranch: new("release/1.x"),
	}); err != nil {
		t.Fatal(err)
	}
	list, _, err := svc.List(ctx, task.ListFilter{ProjectCategoryID: &cat.ID, Limit: 1})
	if err != nil || len(list) != 1 || *list[0].ProjectCategory.RepoURL != "https://github.com/org/other" {
		t.Errorf("listed category ref = %+v, %v", list, err)
	}
	if c, _ := svc.GetCategory(ctx, cat.ID); c.BaseBranch != "release/1.x" {
		t.Errorf("base branch = %q", c.BaseBranch)
	}

	got, err = svc.Update(ctx, created.ID, task.UpdateInput{Status: new(task.StatusDraft), SetRunner: true})
	if err != nil || got.Execution.Runner != nil {
		t.Errorf("clearing runner = %+v, %v", got.Execution, err)
	}
}
