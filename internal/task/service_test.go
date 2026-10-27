package task_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/jrgomesdev/miner/internal/task"
	"github.com/jrgomesdev/miner/internal/task/tasktest"
)

func newService(t *testing.T) (*task.Service, *tasktest.Repository) {
	t.Helper()
	repo := tasktest.NewRepository()
	return task.NewService(repo, tasktest.Catalog()), repo
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

// configured returns the input of a task that may be ready: fully
// configured and in a category with a repository.
func configured(t *testing.T, svc *task.Service) task.CreateInput {
	t.Helper()
	cat, err := svc.CreateCategory(context.Background(), task.CategoryInput{
		Name:    "Repo " + uuid.NewString(),
		RepoURL: new("https://github.com/org/repo"),
	})
	if err != nil {
		t.Fatal(err)
	}
	return task.CreateInput{
		Title:             "T",
		Description:       "D",
		ProjectCategoryID: &cat.ID,
		Execution:         claudeOpus("high"),
	}
}

func claudeOpus(effort string) task.Execution {
	return task.Execution{Agent: new("claude"), Model: new("opus"), Effort: new(effort), Runner: new("host")}
}

func TestUpdateStatus(t *testing.T) {
	ctx := context.Background()
	svc, _ := newService(t)
	created, err := svc.CreateLocal(ctx, configured(t, svc))
	if err != nil {
		t.Fatal(err)
	}

	_, err = svc.Update(ctx, created.ID, task.UpdateInput{Status: new(task.StatusDone)})
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

func TestCreateWithExecution(t *testing.T) {
	ctx := context.Background()
	svc, _ := newService(t)

	got, err := svc.CreateLocal(ctx, task.CreateInput{Title: "T", Description: "D", Execution: claudeOpus("high")})
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != task.StatusDraft || !sameExecution(got.Execution, claudeOpus("high")) {
		t.Errorf("created = %+v", got)
	}

	bare, _ := svc.CreateLocal(ctx, task.CreateInput{Title: "T", Description: "D"})
	if !sameExecution(bare.Execution, task.Execution{}) {
		t.Errorf("execution without input = %+v", bare.Execution)
	}

	_, err = svc.CreateLocal(ctx, task.CreateInput{Title: "T", Description: "D", Execution: task.Execution{Agent: new("gpt")}})
	assertValidation(t, err, "agent")
	if _, total, _ := svc.List(ctx, task.ListFilter{Limit: 10}); total != 2 {
		t.Errorf("invalid create stored a task: total %d", total)
	}
}

func TestExecutionConfiguration(t *testing.T) {
	ctx := context.Background()
	svc, _ := newService(t)
	draft, _ := svc.CreateLocal(ctx, task.CreateInput{Title: "T", Description: "D"})

	tests := []struct {
		name  string
		in    task.UpdateInput
		field string
	}{
		{"unknown agent", task.UpdateInput{SetAgent: true, Agent: new("gpt")}, "agent"},
		{"model of another agent", task.UpdateInput{SetAgent: true, Agent: new("claude"), SetModel: true, Model: new("gpt-5")}, "model"},
		{"model without agent", task.UpdateInput{SetModel: true, Model: new("opus")}, "model"},
		{"effort without model", task.UpdateInput{SetEffort: true, Effort: new("high")}, "effort"},
		{"unknown effort", task.UpdateInput{
			SetAgent: true, Agent: new("claude"), SetModel: true, Model: new("opus"), SetEffort: true, Effort: new("extreme"),
		}, "effort"},
		{"unavailable runner", task.UpdateInput{SetRunner: true, Runner: new("k8s")}, "runner"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.Update(ctx, draft.ID, tt.in)
			assertValidation(t, err, tt.field)
			if got, _ := svc.Get(ctx, draft.ID); !sameExecution(got.Execution, task.Execution{}) {
				t.Errorf("rejected update changed the task: %+v", got.Execution)
			}
		})
	}

	// An external task can be configured too.
	if _, err := svc.ApplyEvent(ctx, "jira", task.Event{
		Type: task.EventCreated, ExternalID: "PROJ-1", Title: "Ticket", Description: "Prompt",
	}); err != nil {
		t.Fatal(err)
	}
	jira, _, _ := svc.List(ctx, task.ListFilter{Provider: new("jira"), Limit: 1})
	got, err := svc.Update(ctx, jira[0].ID, task.UpdateInput{
		SetAgent: true, Agent: new("claude"), SetModel: true, Model: new("sonnet"),
		SetEffort: true, Effort: new("medium"), SetRunner: true, Runner: new("host"),
	})
	if err != nil {
		t.Fatal(err)
	}
	want := task.Execution{Agent: new("claude"), Model: new("sonnet"), Effort: new("medium"), Runner: new("host")}
	if !sameExecution(got.Execution, want) {
		t.Errorf("configured = %+v", got.Execution)
	}

	// Switching the agent alone leaves an incompatible model.
	_, err = svc.Update(ctx, jira[0].ID, task.UpdateInput{SetAgent: true, Agent: new(tasktest.FakeAgent)})
	assertValidation(t, err, "model")
	if got, _ := svc.Get(ctx, jira[0].ID); !sameExecution(got.Execution, want) {
		t.Errorf("rejected agent switch changed the task: %+v", got.Execution)
	}

	// Switching agent and model together, and clearing a field with nil.
	got, err = svc.Update(ctx, jira[0].ID, task.UpdateInput{
		SetAgent: true, Agent: new(tasktest.FakeAgent), SetModel: true, Model: new("small"),
		SetEffort: true, Effort: nil,
	})
	if err != nil || *got.Execution.Agent != tasktest.FakeAgent || got.Execution.Effort != nil {
		t.Errorf("switch to fake agent = %+v, %v", got.Execution, err)
	}
}

func TestExecutionConfigurationLock(t *testing.T) {
	ctx := context.Background()
	svc, _ := newService(t)
	created, _ := svc.CreateLocal(ctx, configured(t, svc))
	advance := func(statuses ...task.Status) {
		t.Helper()
		for _, st := range statuses {
			if _, err := svc.Update(ctx, created.ID, task.UpdateInput{Status: new(st)}); err != nil {
				t.Fatalf("-> %s: %v", st, err)
			}
		}
	}
	setModel := task.UpdateInput{SetModel: true, Model: new("sonnet")}

	advance(task.StatusReady, task.StatusInProgress)
	_, err := svc.Update(ctx, created.ID, setModel)
	assertValidation(t, err, "model")
	// The lock uses the status before the update.
	_, err = svc.Update(ctx, created.ID, task.UpdateInput{Status: new(task.StatusFailed), SetModel: true, Model: new("sonnet")})
	assertValidation(t, err, "model")
	// Even the current value is rejected.
	_, err = svc.Update(ctx, created.ID, task.UpdateInput{SetRunner: true, Runner: new("host")})
	assertValidation(t, err, "runner")
	if got, _ := svc.Get(ctx, created.ID); got.Status != task.StatusInProgress || *got.Execution.Model != "opus" {
		t.Errorf("locked task changed: %+v", got)
	}
	// Other fields stay editable.
	if _, err := svc.Update(ctx, created.ID, task.UpdateInput{Labels: &[]string{"x"}}); err != nil {
		t.Errorf("label edit while in progress: %v", err)
	}

	advance(task.StatusFailed)
	got, err := svc.Update(ctx, created.ID, task.UpdateInput{
		SetModel: true, Model: new("sonnet"), SetEffort: true, Effort: new("high"),
	})
	if err != nil || *got.Execution.Model != "sonnet" {
		t.Errorf("model change after failure = %+v, %v", got.Execution, err)
	}

	advance(task.StatusCancelled)
	_, err = svc.Update(ctx, created.ID, setModel)
	assertValidation(t, err, "model")
}

func TestReadyPrerequisites(t *testing.T) {
	ctx := context.Background()
	svc, _ := newService(t)
	toReady := task.UpdateInput{Status: new(task.StatusReady)}

	t.Run("configured", func(t *testing.T) {
		created, _ := svc.CreateLocal(ctx, configured(t, svc))
		got, err := svc.Update(ctx, created.ID, toReady)
		if err != nil || got.Status != task.StatusReady {
			t.Errorf("ready = %v, %v", got.Status, err)
		}
	})

	t.Run("lists every missing item", func(t *testing.T) {
		in := configured(t, svc)
		in.Execution = task.Execution{Agent: new("claude"), Model: new("opus")}
		created, _ := svc.CreateLocal(ctx, in)
		_, err := svc.Update(ctx, created.ID, toReady)
		assertValidation(t, err, "status")
		if err == nil || !strings.Contains(err.Error(), "missing effort, runner") {
			t.Errorf("message = %v", err)
		}
		bare, _ := svc.CreateLocal(ctx, task.CreateInput{Title: "T", Description: "D"})
		_, err = svc.Update(ctx, bare.ID, toReady)
		if err == nil || !strings.Contains(err.Error(), "missing agent, model, runner, project_category") {
			t.Errorf("message = %v", err)
		}
		if got, _ := svc.Get(ctx, bare.ID); got.Status != task.StatusDraft {
			t.Errorf("status = %s", got.Status)
		}
	})

	t.Run("category without repository", func(t *testing.T) {
		cat, _ := svc.CreateCategory(ctx, task.CategoryInput{Name: "Sem repo"})
		created, _ := svc.CreateLocal(ctx, task.CreateInput{
			Title: "T", Description: "D", ProjectCategoryID: &cat.ID, Execution: claudeOpus("low"),
		})
		_, err := svc.Update(ctx, created.ID, toReady)
		if err == nil || !strings.Contains(err.Error(), "missing project_category.repo_url") {
			t.Errorf("error = %v", err)
		}
	})

	t.Run("model without efforts", func(t *testing.T) {
		in := configured(t, svc)
		in.Execution = task.Execution{Agent: new(tasktest.FakeAgent), Model: new("small"), Runner: new("host")}
		created, _ := svc.CreateLocal(ctx, in)
		if _, err := svc.Update(ctx, created.ID, toReady); err != nil {
			t.Errorf("ready without effort = %v", err)
		}
	})

	t.Run("configure and release together", func(t *testing.T) {
		in := configured(t, svc)
		in.Execution = task.Execution{}
		created, _ := svc.CreateLocal(ctx, in)
		got, err := svc.Update(ctx, created.ID, task.UpdateInput{
			SetAgent: true, Agent: new("claude"), SetModel: true, Model: new("opus"),
			SetEffort: true, Effort: new("low"), SetRunner: true, Runner: new("host"),
			Status: new(task.StatusReady),
		})
		if err != nil || got.Status != task.StatusReady || !sameExecution(got.Execution, claudeOpus("low")) {
			t.Errorf("ready = %+v, %v", got, err)
		}
	})

	t.Run("edits while ready", func(t *testing.T) {
		created, _ := svc.CreateLocal(ctx, configured(t, svc))
		if _, err := svc.Update(ctx, created.ID, toReady); err != nil {
			t.Fatal(err)
		}
		for name, in := range map[string]task.UpdateInput{
			"clear runner":   {SetRunner: true},
			"clear category": {SetProjectCategory: true},
		} {
			_, err := svc.Update(ctx, created.ID, in)
			assertValidation(t, err, "status")
			if got, _ := svc.Get(ctx, created.ID); got.Execution.Runner == nil || got.ProjectCategory == nil {
				t.Errorf("%s changed the task: %+v", name, got)
			}
		}
		got, err := svc.Update(ctx, created.ID, task.UpdateInput{Status: new(task.StatusDraft), SetRunner: true})
		if err != nil || got.Status != task.StatusDraft || got.Execution.Runner != nil {
			t.Errorf("back to draft = %+v, %v", got, err)
		}
	})

	t.Run("cancelled stays an invalid transition", func(t *testing.T) {
		created, _ := svc.CreateLocal(ctx, task.CreateInput{Title: "T", Description: "D"})
		if _, err := svc.Update(ctx, created.ID, task.UpdateInput{Status: new(task.StatusCancelled)}); err != nil {
			t.Fatal(err)
		}
		if _, err := svc.Update(ctx, created.ID, toReady); !errors.Is(err, task.ErrInvalidTransition) {
			t.Errorf("cancelled -> ready = %v, want ErrInvalidTransition", err)
		}
	})
}

func TestSetCategoryAndReady(t *testing.T) {
	ctx := context.Background()
	svc, _ := newService(t)
	in := configured(t, svc)
	categoryID := in.ProjectCategoryID
	in.ProjectCategoryID = nil
	created, _ := svc.CreateLocal(ctx, in)

	// The category is resolved before the mutation, which would otherwise
	// deadlock on the in-memory repository.
	got, err := svc.Update(ctx, created.ID, task.UpdateInput{
		SetProjectCategory: true, ProjectCategoryID: categoryID, Status: new(task.StatusReady),
	})
	if err != nil || got.Status != task.StatusReady || got.ProjectCategory.RepoURL == nil {
		t.Fatalf("ready with new category = %+v, %v", got, err)
	}

	_, err = svc.Update(ctx, created.ID, task.UpdateInput{SetProjectCategory: true, ProjectCategoryID: new(uuid.New())})
	assertValidation(t, err, "project_category_id")
}

func TestCategoryRepository(t *testing.T) {
	ctx := context.Background()
	svc, _ := newService(t)

	c, err := svc.CreateCategory(ctx, task.CategoryInput{Name: "Backend"})
	if err != nil || c.RepoURL != nil || c.BaseBranch != "main" {
		t.Fatalf("defaults = %+v, %v", c, err)
	}
	c, err = svc.CreateCategory(ctx, task.CategoryInput{
		Name: "Miner", RepoURL: new("https://github.com/mjrdev/miner-agent"), BaseBranch: "develop",
	})
	if err != nil || *c.RepoURL != "https://github.com/mjrdev/miner-agent" || c.BaseBranch != "develop" {
		t.Fatalf("with repository = %+v, %v", c, err)
	}
	_, err = svc.CreateCategory(ctx, task.CategoryInput{Name: "X", RepoURL: new("git@github.com:org/repo.git")})
	assertValidation(t, err, "repo_url")
	_, err = svc.CreateCategory(ctx, task.CategoryInput{Name: "Y", BaseBranch: "feature..x"})
	assertValidation(t, err, "base_branch")

	got, err := svc.UpdateCategory(ctx, c.ID, task.CategoryPatch{BaseBranch: new("release/1.x")})
	if err != nil || got.BaseBranch != "release/1.x" || got.RepoURL == nil || got.Name != "Miner" {
		t.Errorf("branch change = %+v, %v", got, err)
	}
	_, err = svc.UpdateCategory(ctx, c.ID, task.CategoryPatch{SetRepoURL: true, RepoURL: new("https://user:token@github.com/org/repo")})
	assertValidation(t, err, "repo_url")
}

func TestKeepRepositoryOfCategoriesWithReadyTasks(t *testing.T) {
	ctx := context.Background()
	svc, _ := newService(t)
	in := configured(t, svc)
	categoryID := *in.ProjectCategoryID
	created, _ := svc.CreateLocal(ctx, in)
	removeRepo := task.CategoryPatch{SetRepoURL: true}

	if _, err := svc.Update(ctx, created.ID, task.UpdateInput{Status: new(task.StatusReady)}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.UpdateCategory(ctx, categoryID, removeRepo); !errors.Is(err, task.ErrConflict) {
		t.Errorf("removing repository in use = %v, want ErrConflict", err)
	}
	if c, _ := svc.GetCategory(ctx, categoryID); c.RepoURL == nil {
		t.Error("repository removed")
	}
	// Replacing it is allowed.
	if _, err := svc.UpdateCategory(ctx, categoryID, task.CategoryPatch{
		SetRepoURL: true, RepoURL: new("https://github.com/org/other"),
	}); err != nil {
		t.Errorf("replacing repository = %v", err)
	}

	if _, err := svc.Update(ctx, created.ID, task.UpdateInput{Status: new(task.StatusDraft)}); err != nil {
		t.Fatal(err)
	}
	c, err := svc.UpdateCategory(ctx, categoryID, removeRepo)
	if err != nil || c.RepoURL != nil {
		t.Errorf("removing repository without ready tasks = %+v, %v", c, err)
	}
}

func sameExecution(a, b task.Execution) bool {
	eq := func(x, y *string) bool { return (x == nil) == (y == nil) && (x == nil || *x == *y) }
	return eq(a.Agent, b.Agent) && eq(a.Model, b.Model) && eq(a.Effort, b.Effort) && eq(a.Runner, b.Runner)
}

func assertValidation(t *testing.T, err error, field string) {
	t.Helper()
	var ve *task.ValidationError
	if !errors.As(err, &ve) || ve.Field != field {
		t.Errorf("error = %v, want validation error on %q", err, field)
	}
}
