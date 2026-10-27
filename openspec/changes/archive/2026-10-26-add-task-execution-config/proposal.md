# Proposal

## Why

O próximo passo do `miner` é executar as tasks com um agente de IA dentro de um container (no host agora, em Jobs do Kubernetes depois). Antes de existir o executor, cada task precisa dizer **com qual agente, modelo e esforço** e **em qual runner** vai rodar, e cada projeto precisa dizer **qual repositório** o agente clona. Hoje nada disso existe: uma task vai para `ready` sem nenhuma dessas informações e a categoria de projeto só tem nome e descrição.

Esta é a primeira de duas changes. Ela entrega só a configuração no CRUD e as validações. O executor (runner `host`, Dockerfile default, entidade de execução e fila) fica para a change `add-host-task-runner`.

## What Changes

- **Catálogo de execução:** novo contrato `Agent` com nome e catálogo de modelos e níveis de esforço. A interface é comum para aceitar outros agentes no futuro. Primeiro adapter: `claude`, com os aliases de modelo e os níveis de `--effort` aceitos pelo Claude Code CLI. Os runners disponíveis são uma lista de nomes, e nesta change só existe `host`.
- **`GET /execution-options`:** lista os agentes, com seus modelos e esforços, e os runners. É a base dos selects do CRUD.
- **Configuração de execução na task:** novos campos `agent`, `model`, `effort` e `runner`, opcionais no `POST /tasks` e editáveis no `PATCH /tasks/{id}` (`null` limpa). A combinação é validada contra o catálogo. Eles não podem mudar enquanto a task está em `in_progress` nem depois que ela chega a um estado terminal.
- **Pré-requisitos de `ready`:** uma task só fica em `ready` com `agent`, `model`, `runner` (e `effort`, quando o modelo tem níveis de esforço), categoria de projeto definida e categoria com `repo_url`. A regra vale na transição e em qualquer edição feita enquanto a task está em `ready`. Faltando algo, a resposta é `422`.
- **Perfil de execução na categoria:** novos campos `repo_url` (URL HTTPS do repositório, opcional) e `base_branch` (padrão `main`). Remover o `repo_url` de uma categoria com tasks em `ready` resulta em `409`.
- **Migração** `00002` com as novas colunas, e o código do sqlc regenerado.
- **ADR** para o domínio `execution` e o contrato de agentes.

Fora de escopo: executar qualquer coisa. Ou seja, ficam de fora o runner `host`, o runner `k8s`, o Dockerfile default, a entidade de execução, a fila de execução, o push e o PR. Também ficam fora a imagem por projeto e os ambientes (`environments/<slug>/`), que dependem do runner e entram na próxima change.

## Capabilities

### New Capabilities
- `execution-catalog`: contrato de agentes de IA, catálogo de modelos e esforços, runners disponíveis e o endpoint `GET /execution-options`.

### Modified Capabilities
- `task-management`: a representação, a criação e a edição da task ganham a configuração de execução, e a transição para `ready` passa a ter pré-requisitos.
- `project-categories`: a representação, a criação e a edição da categoria ganham `repo_url` e `base_branch`, com a regra de não remover o `repo_url` enquanto houver tasks em `ready`.

## Impact

- **Código novo:** `internal/execution/` (contrato `Agent`, `Model`, catálogo) e `internal/execution/claude/`.
- **Código alterado:**
  - `internal/task` (campos, validação e pré-requisitos de `ready`; o `Service` recebe o catálogo)
  - `internal/task/postgres` (queries e sqlc)
  - `internal/task/tasktest`
  - `internal/task/httpapi` (campos novos e `GET /execution-options`)
  - `cmd/api/main.go` (composição do catálogo)
- **Banco:** `migrations/00002_add_execution_config.sql`, aditiva e com `Down`.
- **APIs:**
  - `GET /execution-options` é novo.
  - Tasks e categorias ganham campos na resposta, o que é compatível com clientes existentes.
  - **BREAKING (comportamento):** `PATCH {"status":"ready"}` em uma task sem configuração completa, que hoje responde `200`, passa a responder `422`.
- **Docs:** um ADR novo em `docs/adr/`; `README.md` com exemplos de configuração de execução.
