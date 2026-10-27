package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jrgomesdev/miner/internal/task"
	"github.com/jrgomesdev/miner/internal/task/adapter"
	"github.com/jrgomesdev/miner/internal/task/adapter/local"
	"github.com/jrgomesdev/miner/internal/task/tasktest"
)

type published struct {
	provider, deliveryID string
	body                 []byte
}

type fakePublisher struct {
	err  error
	sent []published
}

func (p *fakePublisher) Publish(_ context.Context, provider, deliveryID string, body []byte) error {
	if p.err != nil {
		return p.err
	}
	p.sent = append(p.sent, published{provider, deliveryID, body})
	return nil
}

type testAPI struct {
	mux  *http.ServeMux
	svc  *task.Service
	repo *tasktest.Repository
	pub  *fakePublisher
}

func newTestAPI(t *testing.T) *testAPI {
	t.Helper()
	log := slog.New(slog.NewJSONHandler(io.Discard, nil))
	repo := tasktest.NewRepository()
	svc := task.NewService(repo, tasktest.Catalog())
	pub := &fakePublisher{}
	mux := http.NewServeMux()
	NewWebhooks(adapter.NewRegistry(local.New("secret")), pub, log).Register(mux)
	NewTasks(svc, log).Register(mux)
	return &testAPI{mux: mux, svc: svc, repo: repo, pub: pub}
}

func (a *testAPI) do(t *testing.T, method, path, body string, header ...string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	for i := 0; i+1 < len(header); i += 2 {
		req.Header.Set(header[i], header[i+1])
	}
	rec := httptest.NewRecorder()
	a.mux.ServeHTTP(rec, req)
	return rec
}

func decode[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatalf("invalid JSON response %q: %v", rec.Body.String(), err)
	}
	return v
}

func expect(t *testing.T, rec *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if rec.Code != status {
		t.Fatalf("status = %d, want %d (body %s)", rec.Code, status, rec.Body.String())
	}
	if code == "" {
		return
	}
	if got := decode[errorBody](t, rec).Error.Code; got != code {
		t.Errorf("error.code = %q, want %q", got, code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q", ct)
	}
}

func TestWebhooks(t *testing.T) {
	const valid = `{"title":"Corrigir login","description":"Implemente ..."}`
	token := []string{local.TokenHeader, "secret"}

	t.Run("accepted", func(t *testing.T) {
		api := newTestAPI(t)
		rec := api.do(t, http.MethodPost, "/webhooks/local", valid, token...)
		expect(t, rec, http.StatusAccepted, "")
		id := decode[deliveryResponse](t, rec).DeliveryID
		if id == "" || len(api.pub.sent) != 1 || api.pub.sent[0].deliveryID != id || api.pub.sent[0].provider != "local" {
			t.Errorf("delivery %q, published %+v", id, api.pub.sent)
		}
	})

	tests := []struct {
		name, path, body string
		header           []string
		status           int
		code             string
	}{
		{"unknown provider", "/webhooks/desconhecido", valid, token, http.StatusNotFound, codeNotFound},
		{"wrong token", "/webhooks/local", valid, []string{local.TokenHeader, "nope"}, http.StatusUnauthorized, codeUnauthorized},
		{"missing token", "/webhooks/local", valid, nil, http.StatusUnauthorized, codeUnauthorized},
		{"not JSON", "/webhooks/local", "isto não é json", token, http.StatusBadRequest, codeValidationFailed},
		{"empty body", "/webhooks/local", "", token, http.StatusBadRequest, codeValidationFailed},
		{"too large", "/webhooks/local", `"` + strings.Repeat("x", 2<<20) + `"`, token, http.StatusRequestEntityTooLarge, codePayloadTooLarge},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := newTestAPI(t)
			expect(t, api.do(t, http.MethodPost, tt.path, tt.body, tt.header...), tt.status, tt.code)
			if len(api.pub.sent) != 0 {
				t.Errorf("published %d messages, want none", len(api.pub.sent))
			}
		})
	}

	t.Run("broker unavailable", func(t *testing.T) {
		api := newTestAPI(t)
		api.pub.err = errors.New("broker: not connected")
		expect(t, api.do(t, http.MethodPost, "/webhooks/local", valid, token...), http.StatusServiceUnavailable, codeUnavailable)
	})
}

func TestCreateTask(t *testing.T) {
	api := newTestAPI(t)

	rec := api.do(t, http.MethodPost, "/tasks", `{"title":"Adicionar paginação","description":"Implemente ..."}`)
	expect(t, rec, http.StatusCreated, "")
	got := decode[map[string]any](t, rec)
	for key, want := range map[string]any{
		"provider": "local", "status": "draft", "external_url": nil, "project_category": nil,
		"agent": nil, "model": nil, "effort": nil, "runner": nil,
	} {
		if got[key] != want {
			t.Errorf("%s = %v, want %v", key, got[key], want)
		}
	}
	if got["external_id"] != got["id"] {
		t.Errorf("external_id = %v, want the id %v", got["external_id"], got["id"])
	}
	for _, key := range []string{"additional_instructions", "labels"} {
		if list, ok := got[key].([]any); !ok || len(list) != 0 {
			t.Errorf("%s = %v, want []", key, got[key])
		}
	}
	if _, ok := got["raw_payload"]; ok {
		t.Error("raw payload exposed")
	}

	expect(t, api.do(t, http.MethodPost, "/tasks", `{"title":"x"}`), http.StatusUnprocessableEntity, codeValidationFailed)
	expect(t, api.do(t, http.MethodPost, "/tasks", `{"title":`), http.StatusBadRequest, codeValidationFailed)
	expect(t, api.do(t, http.MethodPost, "/tasks", `{"title":"x","description":"y","extra":1}`), http.StatusBadRequest, codeValidationFailed)
	expect(t, api.do(t, http.MethodPost, "/tasks",
		`{"title":"x","description":"y","project_category_id":"0199f5b0-0000-7000-8000-000000000000"}`),
		http.StatusUnprocessableEntity, codeValidationFailed)
}

func TestGetAndDeleteTask(t *testing.T) {
	api := newTestAPI(t)
	created := decode[taskResponse](t, api.do(t, http.MethodPost, "/tasks", `{"title":"T","description":"D"}`))
	path := "/tasks/" + created.ID.String()

	expect(t, api.do(t, http.MethodGet, path, ""), http.StatusOK, "")
	expect(t, api.do(t, http.MethodGet, "/tasks/nao-e-uuid", ""), http.StatusNotFound, codeNotFound)
	expect(t, api.do(t, http.MethodGet, "/tasks/0199f5b0-0000-7000-8000-000000000000", ""), http.StatusNotFound, codeNotFound)

	expect(t, api.do(t, http.MethodDelete, path, ""), http.StatusNoContent, "")
	expect(t, api.do(t, http.MethodGet, path, ""), http.StatusNotFound, codeNotFound)
	expect(t, api.do(t, http.MethodDelete, path, ""), http.StatusNotFound, codeNotFound)
}

func TestListTasks(t *testing.T) {
	api := newTestAPI(t)
	cat := decode[categoryResponse](t, api.do(t, http.MethodPost, "/project-categories",
		`{"name":"Backend","repo_url":"https://github.com/org/repo"}`))
	ready := decode[taskResponse](t, api.do(t, http.MethodPost, "/tasks",
		`{"title":"A","description":"D","labels":["Backend"],"project_category_id":"`+cat.ID.String()+`",`+
			`"agent":"claude","model":"opus","effort":"high","runner":"host"}`))
	expect(t, api.do(t, http.MethodPatch, "/tasks/"+ready.ID.String(), `{"status":"ready"}`), http.StatusOK, "")
	api.do(t, http.MethodPost, "/tasks", `{"title":"B","description":"D","labels":["backend"]}`)
	api.do(t, http.MethodPost, "/tasks", `{"title":"C","description":"D"}`)

	rec := api.do(t, http.MethodGet, "/tasks?status=ready&label=backend", "")
	expect(t, rec, http.StatusOK, "")
	page := decode[taskListResponse](t, rec)
	if page.Total != 1 || len(page.Items) != 1 || page.Items[0].ID != ready.ID {
		t.Errorf("filtered page = %+v", page)
	}

	if page := decode[taskListResponse](t, api.do(t, http.MethodGet, "/tasks?limit=2", "")); page.Total != 3 || len(page.Items) != 2 {
		t.Errorf("limited page: total %d, %d items", page.Total, len(page.Items))
	}

	for _, query := range []string{"limit=500", "limit=0", "limit=x", "offset=-1", "status=archived", "project_category_id=x"} {
		expect(t, api.do(t, http.MethodGet, "/tasks?"+query, ""), http.StatusBadRequest, codeValidationFailed)
	}

	labels := decode[labelListResponse](t, api.do(t, http.MethodGet, "/labels", ""))
	if len(labels.Items) != 1 || labels.Items[0] != "backend" {
		t.Errorf("labels = %v", labels.Items)
	}
}

func TestUpdateTask(t *testing.T) {
	api := newTestAPI(t)
	ctx := context.Background()
	if _, err := api.svc.ApplyEvent(ctx, "jira", task.Event{
		Type: task.EventCreated, ExternalID: "PROJ-1", Title: "Ticket", Description: "Prompt",
	}); err != nil {
		t.Fatal(err)
	}
	tasks, _, _ := api.svc.List(ctx, task.ListFilter{Limit: 1})
	external := "/tasks/" + tasks[0].ID.String()
	local := "/tasks/" + decode[taskResponse](t, api.do(t, http.MethodPost, "/tasks", `{"title":"T","description":"D"}`)).ID.String()

	rec := api.do(t, http.MethodPatch, external, `{"additional_instructions":["Use a lib X"]}`)
	expect(t, rec, http.StatusOK, "")
	if got := decode[taskResponse](t, rec); got.Description != "Prompt" || got.AdditionalInstructions[0] != "Use a lib X" {
		t.Errorf("external update = %+v", got)
	}
	expect(t, api.do(t, http.MethodPatch, external, `{"description":"..."}`), http.StatusUnprocessableEntity, codeValidationFailed)

	rec = api.do(t, http.MethodPatch, local, `{"description":"novo prompt","labels":["Backend"," backend ","API"]}`)
	expect(t, rec, http.StatusOK, "")
	if got := decode[taskResponse](t, rec); got.Description != "novo prompt" || strings.Join(got.Labels, ",") != "api,backend" {
		t.Errorf("local update = %+v", got)
	}

	expect(t, api.do(t, http.MethodPatch, local, `{"status":"done"}`), http.StatusConflict, codeInvalidTransition)
	expect(t, api.do(t, http.MethodPatch, local, `{"status":"archived"}`), http.StatusUnprocessableEntity, codeValidationFailed)
	expect(t, api.do(t, http.MethodPatch, local, `{"status":"draft"}`), http.StatusOK, "")
	expect(t, api.do(t, http.MethodPatch, local, `{"additional_instructions":["   "]}`), http.StatusUnprocessableEntity, codeValidationFailed)

	cat := decode[categoryResponse](t, api.do(t, http.MethodPost, "/project-categories", `{"name":"Backend"}`))
	rec = api.do(t, http.MethodPatch, local, `{"project_category_id":"`+cat.ID.String()+`"}`)
	if got := decode[taskResponse](t, rec); got.ProjectCategory == nil || got.ProjectCategory.Slug != "backend" {
		t.Errorf("category not set: %s", rec.Body.String())
	}
	rec = api.do(t, http.MethodPatch, local, `{"project_category_id":null}`)
	if got := decode[taskResponse](t, rec); got.ProjectCategory != nil {
		t.Errorf("category not cleared: %s", rec.Body.String())
	}

	cancelled := api.do(t, http.MethodPatch, local, `{"status":"cancelled"}`)
	expect(t, cancelled, http.StatusOK, "")
	expect(t, api.do(t, http.MethodPatch, local, `{"status":"ready"}`), http.StatusConflict, codeInvalidTransition)
}

func TestProjectCategories(t *testing.T) {
	api := newTestAPI(t)

	rec := api.do(t, http.MethodPost, "/project-categories", `{"name":"Integrações Back-end"}`)
	expect(t, rec, http.StatusCreated, "")
	integracoes := decode[categoryResponse](t, rec)
	if integracoes.Slug != "integracoes-back-end" || integracoes.Description != "" {
		t.Errorf("created = %+v", integracoes)
	}

	api.do(t, http.MethodPost, "/project-categories", `{"name":"Infra"}`)
	api.do(t, http.MethodPost, "/project-categories", `{"name":"Backend"}`)
	expect(t, api.do(t, http.MethodPost, "/project-categories", `{"name":"backend"}`), http.StatusConflict, codeConflict)
	expect(t, api.do(t, http.MethodPost, "/project-categories", `{"name":"X","slug":"Com Espaço"}`),
		http.StatusUnprocessableEntity, codeValidationFailed)

	list := decode[categoryListResponse](t, api.do(t, http.MethodGet, "/project-categories", ""))
	var names []string
	for _, c := range list.Items {
		names = append(names, c.Name)
	}
	if strings.Join(names, ",") != "Backend,Infra,Integrações Back-end" {
		t.Errorf("order = %v", names)
	}

	path := "/project-categories/" + integracoes.ID.String()
	expect(t, api.do(t, http.MethodGet, path, ""), http.StatusOK, "")
	expect(t, api.do(t, http.MethodGet, "/project-categories/x", ""), http.StatusNotFound, codeNotFound)

	created := decode[taskResponse](t, api.do(t, http.MethodPost, "/tasks",
		`{"title":"T","description":"D","project_category_id":"`+integracoes.ID.String()+`"}`))

	rec = api.do(t, http.MethodPatch, path, `{"name":"Plataforma"}`)
	expect(t, rec, http.StatusOK, "")
	got := decode[taskResponse](t, api.do(t, http.MethodGet, "/tasks/"+created.ID.String(), ""))
	if got.ProjectCategory.Name != "Plataforma" || got.ProjectCategory.Slug != "integracoes-back-end" {
		t.Errorf("task category = %+v", got.ProjectCategory)
	}

	expect(t, api.do(t, http.MethodDelete, path, ""), http.StatusConflict, codeConflict)
	api.do(t, http.MethodDelete, "/tasks/"+created.ID.String(), "")
	expect(t, api.do(t, http.MethodDelete, path, ""), http.StatusNoContent, "")
}

func TestTaskExecution(t *testing.T) {
	api := newTestAPI(t)
	withRepo := decode[categoryResponse](t, api.do(t, http.MethodPost, "/project-categories",
		`{"name":"Miner","repo_url":"https://github.com/mjrdev/miner-agent"}`))

	rec := api.do(t, http.MethodPost, "/tasks",
		`{"title":"T","description":"D","agent":"claude","model":"opus","effort":"high","runner":"host"}`)
	expect(t, rec, http.StatusCreated, "")
	created := decode[taskResponse](t, rec)
	if created.Status != task.StatusDraft || *created.Agent != "claude" || *created.Model != "opus" ||
		*created.Effort != "high" || *created.Runner != "host" {
		t.Errorf("created = %+v", created)
	}
	expect(t, api.do(t, http.MethodPost, "/tasks", `{"title":"T","description":"D","agent":"gpt"}`),
		http.StatusUnprocessableEntity, codeValidationFailed)

	path := "/tasks/" + created.ID.String()
	for _, body := range []string{
		`{"agent":"gpt"}`,
		`{"agent":"claude","model":"gpt-5"}`,
		`{"runner":"k8s"}`,
		`{"model":null,"effort":"high"}`,
	} {
		expect(t, api.do(t, http.MethodPatch, path, body), http.StatusUnprocessableEntity, codeValidationFailed)
	}

	rec = api.do(t, http.MethodPatch, path, `{"status":"ready"}`)
	expect(t, rec, http.StatusUnprocessableEntity, codeValidationFailed)
	if msg := decode[errorBody](t, rec).Error.Message; !strings.Contains(msg, "project_category") {
		t.Errorf("message = %q", msg)
	}

	rec = api.do(t, http.MethodPatch, path,
		`{"project_category_id":"`+withRepo.ID.String()+`","model":"sonnet","effort":"low","status":"ready"}`)
	expect(t, rec, http.StatusOK, "")
	if got := decode[taskResponse](t, rec); got.Status != task.StatusReady || *got.Model != "sonnet" {
		t.Errorf("ready = %+v", got)
	}

	expect(t, api.do(t, http.MethodPatch, path, `{"runner":null}`), http.StatusUnprocessableEntity, codeValidationFailed)
	expect(t, api.do(t, http.MethodPatch, path, `{"project_category_id":null}`), http.StatusUnprocessableEntity, codeValidationFailed)
	expect(t, api.do(t, http.MethodPatch, "/project-categories/"+withRepo.ID.String(), `{"repo_url":null}`),
		http.StatusConflict, codeConflict)

	rec = api.do(t, http.MethodPatch, path, `{"status":"draft","runner":null}`)
	expect(t, rec, http.StatusOK, "")
	if got := decode[taskResponse](t, rec); got.Status != task.StatusDraft || got.Runner != nil {
		t.Errorf("back to draft = %+v", got)
	}
	expect(t, api.do(t, http.MethodPatch, "/project-categories/"+withRepo.ID.String(), `{"repo_url":null}`),
		http.StatusOK, "")

	locked := decode[taskResponse](t, api.do(t, http.MethodPost, "/tasks", `{"title":"T","description":"D"}`))
	expect(t, api.do(t, http.MethodPatch, "/tasks/"+locked.ID.String(), `{"status":"cancelled"}`), http.StatusOK, "")
	expect(t, api.do(t, http.MethodPatch, "/tasks/"+locked.ID.String(), `{"model":"sonnet"}`),
		http.StatusUnprocessableEntity, codeValidationFailed)
}

func TestCategoryRepository(t *testing.T) {
	api := newTestAPI(t)

	rec := api.do(t, http.MethodPost, "/project-categories", `{"name":"Integrações Back-end"}`)
	expect(t, rec, http.StatusCreated, "")
	got := decode[map[string]any](t, rec)
	if got["repo_url"] != nil || got["base_branch"] != "main" {
		t.Errorf("defaults = %v", got)
	}

	rec = api.do(t, http.MethodPost, "/project-categories",
		`{"name":"Miner","repo_url":"https://github.com/mjrdev/miner-agent","base_branch":"develop"}`)
	expect(t, rec, http.StatusCreated, "")
	miner := decode[categoryResponse](t, rec)
	if *miner.RepoURL != "https://github.com/mjrdev/miner-agent" || miner.BaseBranch != "develop" {
		t.Errorf("created = %+v", miner)
	}

	for _, body := range []string{
		`{"name":"X","repo_url":"https://user:token@github.com/org/repo"}`,
		`{"name":"X","repo_url":"git@github.com:org/repo.git"}`,
		`{"name":"X","base_branch":"feature..x"}`,
	} {
		expect(t, api.do(t, http.MethodPost, "/project-categories", body), http.StatusUnprocessableEntity, codeValidationFailed)
	}

	path := "/project-categories/" + miner.ID.String()
	rec = api.do(t, http.MethodPatch, path, `{"base_branch":"release/1.x"}`)
	expect(t, rec, http.StatusOK, "")
	if got := decode[categoryResponse](t, rec); got.BaseBranch != "release/1.x" || got.RepoURL == nil || got.Name != "Miner" {
		t.Errorf("branch change = %+v", got)
	}
	expect(t, api.do(t, http.MethodPatch, path, `{"base_branch":null}`), http.StatusUnprocessableEntity, codeValidationFailed)

	rec = api.do(t, http.MethodPatch, path, `{"repo_url":null}`)
	expect(t, rec, http.StatusOK, "")
	if got := decode[map[string]any](t, rec); got["repo_url"] != nil {
		t.Errorf("repo_url = %v", got["repo_url"])
	}
}

func TestExecutionOptions(t *testing.T) {
	api := newTestAPI(t)
	rec := api.do(t, http.MethodGet, "/execution-options", "")
	expect(t, rec, http.StatusOK, "")
	got := decode[executionOptionsResponse](t, rec)

	var names []string
	for _, a := range got.Agents {
		names = append(names, a.Name)
	}
	if strings.Join(names, ",") != "claude,"+tasktest.FakeAgent || strings.Join(got.Runners, ",") != "host" {
		t.Fatalf("options = %+v", got)
	}
	var models []string
	for _, m := range got.Agents[0].Models {
		models = append(models, m.ID)
		if strings.Join(m.Efforts, ",") != "low,medium,high,xhigh,max" {
			t.Errorf("%s efforts = %v", m.ID, m.Efforts)
		}
	}
	if strings.Join(models, ",") != "fable,opus,sonnet" {
		t.Errorf("claude models = %v", models)
	}
	// A model without efforts is still listed with an empty array.
	if !strings.Contains(rec.Body.String(), `{"id":"small","efforts":[]}`) {
		t.Errorf("body = %s", rec.Body.String())
	}
}

func TestInternalErrorsAreGeneric(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&buf, nil))
	repo := tasktest.NewRepository()
	repo.Err = errors.New("password=secret connection refused")
	mux := http.NewServeMux()
	NewTasks(task.NewService(repo, tasktest.Catalog()), log).Register(mux)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/tasks", nil))
	expect(t, rec, http.StatusInternalServerError, codeInternal)
	if strings.Contains(rec.Body.String(), "secret") {
		t.Errorf("internal error leaked: %s", rec.Body.String())
	}
	if !strings.Contains(buf.String(), "request failed") {
		t.Error("internal error not logged")
	}
}
