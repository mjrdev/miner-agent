# Tasks

## 1. Catálogo de execução

- [x] 1.1 Criar `internal/execution` com a interface `Agent` (`Name`, `Models`), `Model` (`ID`, `Efforts`) e `Catalog` (`NewCatalog(agents, runners)`, `Agent(name)`, `HasRunner(name)` e uma visão ordenada com agentes por nome e runners por nome). O pacote não pode importar `internal/task`. Verificar com testes de unidade: lookup de agente registrado e desconhecido, `HasRunner("host")` verdadeiro e `HasRunner("k8s")` falso, e ordenação da visão com dois agentes fake
- [x] 1.2 Criar `internal/execution/claude` com o agente `claude` e os modelos `fable`, `opus` e `sonnet`, cada um com `low`, `medium`, `high`, `xhigh` e `max`, nessa ordem. Verificar com um teste de unidade que confere a lista exata da spec `execution-catalog`

## 2. Banco

- [x] 2.1 Criar `migrations/00002_add_execution_config.sql`: em `tasks`, as colunas `agent`, `model`, `effort` e `runner` (`text NULL`); em `project_categories`, `repo_url text NULL` e `base_branch text NOT NULL DEFAULT 'main'`, com o `Down` removendo-as. Verificar com `make migrate`, `make migrate-down` e `make migrate` sem erro, e com `make psql` (`\d tasks`, `\d project_categories`) mostrando as colunas, incluindo `base_branch` = `main` nas categorias existentes
- [x] 2.2 Atualizar as queries em `internal/task/postgres/queries/`: insert e update de task com os quatro campos de execução, select de task e listagem trazendo também `repo_url` da categoria, e insert, select e update de categoria com `repo_url` e `base_branch`. Rodar `make sqlc` e verificar que `go build ./...` passa e que o código gerado está em `sqlcdb/`

## 3. Domínio task

- [x] 3.1 Adicionar `Execution` (`Agent`, `Model`, `Effort`, `Runner *string`) a `Task`, `RepoURL` a `CategoryRef`, e `RepoURL` e `BaseBranch` a `ProjectCategory`. Adicionar a validação de `repo_url` (https, host e path, sem credenciais, até 500 caracteres) e de `base_branch` (regex, início e fim, sem `..`, de 1 a 255 caracteres) em `validate.go`. Verificar com testes de tabela em `task_test.go`, incluindo `https://user:token@github.com/org/repo`, `git@github.com:org/repo.git`, `feature..x`, `-x`, `x/` e `release/1.x`
- [x] 3.2 Mudar `NewService(repo, catalog *execution.Catalog)` e adicionar `Service.ExecutionOptions()`. Ajustar todos os chamadores (`cmd/api/main.go` e os testes de `task`, `httpapi`, `postgres` e `ingest`) e criar `tasktest.Catalog()` com o `claude` real mais um agente fake de dois modelos. Verificar com `go build ./...` e `make test`
- [x] 3.3 Adicionar a configuração de execução a `CreateInput` (validada contra o catálogo quando enviada) e a `UpdateInput` (`Set*` + valor para cada um dos quatro campos). Implementar em `apply` a ordem do design: campos do provider → lock por status atual → aplicação → transição → validação no catálogo (quando um dos quatro foi enviado ou o status resultante é `ready`) → pré-requisitos de `ready`, com uma mensagem que lista todos os faltantes. Verificar com testes em `service_test.go` que cobrem cada cenário das requirements `Execution configuration`, `Execution configuration lock` e `Ready prerequisites`, e que `cancelled → ready` continua `ErrInvalidTransition`
- [x] 3.4 Fazer `Service.Update`, quando o `PATCH` define uma categoria, buscá-la com `GetCategory` antes do `UpdateTask` e passar o `CategoryRef` completo (com `RepoURL`) para a mutação. Uma categoria inexistente continua sendo `ValidationError`. Verificar com um teste em `service_test.go` que faz `PATCH {"project_category_id":<categoria com repo>,"status":"ready",...}` numa task sem categoria e obtém `ready`, e com outro que mostra que o `tasktest` não trava
- [x] 3.5 Adicionar `RepoURL`/`BaseBranch` a `CategoryInput` (padrão `main`) e `SetRepoURL`/`RepoURL`/`BaseBranch` a `CategoryPatch`. Implementar a regra de conflito: ao remover o `repo_url`, consultar `ListTasks{ProjectCategoryID, Status: ready, Limit: 1}` e retornar `ErrConflict` se `total > 0`. Verificar com testes em `service_test.go` dos dois cenários de `Keep repository of categories with ready tasks`

## 4. Repositórios

- [x] 4.1 Atualizar `internal/task/tasktest` para guardar e devolver a execução da task, `RepoURL` no `CategoryRef` resolvido e os campos novos da categoria. Verificar com `make test`, com os testes de `service_test.go` e `httpapi` passando
- [x] 4.2 Atualizar `internal/task/postgres` para mapear os campos novos em insert, get, list e update de tasks e categorias. Verificar com testes de integração em `repository_test.go` (via `make test-integration`): a execução e o `repo_url` fazem ida e volta, `CategoryRef.RepoURL` reflete a categoria após um `PATCH` dela, e `base_branch` assume `main` por padrão

## 5. API HTTP

- [x] 5.1 Adicionar `agent`, `model`, `effort` e `runner` a `taskResponse`, `createTaskRequest` (`*string`) e `updateTaskRequest` (`Optional[string]`), e `repo_url`/`base_branch` às requests e responses de categoria (`repo_url` como `Optional[string]` no `PATCH`, `base_branch: null` → 422). Verificar com testes `httptest` dos cenários novos das specs `task-management` e `project-categories`
- [x] 5.2 Implementar `GET /execution-options` em `httpapi.Tasks.Register`, no formato `{"agents":[{"name","models":[{"id","efforts"}]}],"runners":[...]}`. Verificar com um teste `httptest` do cenário `Leitura das opções` e da lista de modelos do `claude`
- [x] 5.3 Ajustar os testes existentes de `httpapi_test.go` que fazem `draft → ready` (filtro por status e label) para configurar a task e a categoria antes. Verificar com `make test` passando

## 6. Composição, ADR e documentação

- [x] 6.1 Em `cmd/api/main.go`, montar `execution.NewCatalog([]execution.Agent{claude.New()}, []string{"host"})` e passá-lo ao `task.NewService`. Verificar com `make up` e `curl localhost:8080/execution-options` retornando o catálogo do `claude` e `["host"]`
- [x] 6.2 Registrar com a skill `adr` o ADR 0013 (domínio `internal/execution` como folha, contrato `Agent` e catálogo estático em código, runners como nomes até existir a interface). Verificar que `docs/adr/README.md` o lista
- [x] 6.3 Atualizar o `README.md` com os exemplos de `GET /execution-options`, criação de categoria com `repo_url`, configuração de execução da task e liberação para `ready`, além da mudança de comportamento do `422`. Atualizar o `CLAUDE.md` com o novo pacote `internal/execution` na seção de arquitetura. Verificar executando os exemplos `curl` documentados contra `make up`

## 7. Verificação final

- [x] 7.1 Rodar `make fmt`, `make lint`, `make test` e `make test-integration` sem erros e `openspec validate add-task-execution-config --strict` passando

## Workflow follow-up

- Arquivar a change com `/opsx:archive` depois da revisão. A skill `spec-git-flow` (Fase 2) faz os commits, o push da branch `spec/add-task-execution-config` e abre o PR para a `main`.
- Explorar e propor a change `add-host-task-runner` (runner `host`, Dockerfile default, entidade de execução, fila, entrega via PR).
