# Design

## Context

O estado atual relevante para esta change:
- O domínio `internal/task` (ADR-0009) tem `Task`, `Status` com a tabela de transições, `Service` e `Repository`.
- `Repository.UpdateTask` carrega a task (com `LockTask` no Postgres e com o mutex no `tasktest`), aplica uma função de mutação e grava. Por isso, essa função **não pode** consultar o repositório: no repositório em memória isso causaria deadlock no mutex.
- `CategoryRef` (`{ID, Slug, Name}`) é o resumo da categoria carregado junto com a task.
- O padrão de "provider plugável" já existe na ingestão: interface `adapter.Adapter` + `Registry`, composta em `cmd/api/main.go`.

A motivação está no `proposal.md`, e o comportamento, nas specs da change (`execution-catalog` e os deltas de `task-management` e `project-categories`).

Decisões tomadas com a pessoa usuária na exploração:
- Dois eixos de adapter: Agent (o quê) e Runner (onde).
- Só o runner `host` por enquanto.
- O perfil de execução fica na categoria.
- A categoria precisa de `repo_url` para a task ir para `ready`.
- O trabalho é dividido em duas changes, e esta é a primeira.

## Goals / Non-Goals

**Goals:**
- Um contrato de agente que a change `add-host-task-runner` possa estender (com a montagem do comando) sem mudar as regras de task.
- Pré-requisitos de `ready` validados no domínio, para que o futuro executor nunca encontre uma task em `ready` sem agente, modelo, runner ou repositório (exceto pela corrida descrita em Risks).
- Migração apenas aditiva, sem mexer nos dados existentes.

**Non-Goals:**
- Montar comando, prompt, imagem ou ambiente. Isso fica na próxima change.
- Uma interface `Runner`. Por enquanto, um runner é só um nome numa lista, e a interface nasce junto com a primeira implementação.
- Catálogo configurável por variável de ambiente ou banco. Ele é estático, em código, no adapter.
- Filtros de `GET /tasks` por agente ou runner.

## Decisions

### 1. Novo domínio `internal/execution`, folha do grafo

```
internal/
  execution/            Agent (interface), Model, Catalog
    claude/             adapter do Claude Code: nome + modelos/esforços
  task/                 importa execution (para validar a configuração)
    httpapi/            GET /execution-options via *task.Service
cmd/api/main.go         execution.NewCatalog([]Agent{claude.New()}, []string{"host"})
```

```go
// esboço do contrato, não implementação final
type Agent interface {
    Name() string
    Models() []Model        // ordem de exibição
}
type Model struct {
    ID      string
    Efforts []string        // vazio = o modelo não aceita esforço
}
```

`Catalog` indexa os agentes por nome e guarda os runners. Ele expõe `Agent(name)`, `HasRunner(name)` e uma visão ordenada para o endpoint.

`execution` **não importa** `task`. Na próxima change, o `Agent` ganha a montagem do comando a partir de dados neutros (prompt, modelo, esforço), não de `task.Task`, e o worker que junta task + agent + runner fica num subpacote (`execution/<algo>`) que importa os dois. Isso segue a regra do ADR-0009 (subpacotes dependem do pai, nunca o contrário) sem criar ciclo.

*Alternativas:*
- Colocar os agentes dentro de `internal/task/agent/`: execução não é parte do domínio task, e o runner da próxima change também não caberia ali.
- Definir no `task` uma interface-porta `ExecutionCatalog` com tipos próprios: duplica `Model` sem ganho, porque o catálogo é uma folha sem dependências e os testes podem montar um `Catalog` real com agentes fake.

Vai para um ADR novo (0013).

### 2. Catálogo do `claude`

Ele usa os aliases do CLI (`fable`, `opus`, `sonnet`), que sempre apontam para a versão mais recente, e os níveis que `claude --effort` aceita hoje: `low`, `medium`, `high`, `xhigh` e `max`. Os valores são strings repassadas tal como estão ao CLI na próxima change. Mudar o catálogo é uma alteração de código no adapter.

*Alternativa:* IDs completos (`claude-opus-5-5`). Eles travariam o catálogo numa versão e exigiriam manutenção a cada lançamento.

### 3. Campos na task: struct `Execution`, JSON plano

```go
type Execution struct {
    Agent, Model, Effort, Runner *string
}
// Task ganha o campo Execution
```

Na API, os quatro campos ficam no nível da task (`agent`, `model`, `effort`, `runner`). O `POST` usa `*string`, e o `PATCH` usa `Optional[string]` (já existente), para distinguir "ausente" de `null`. No banco, são quatro colunas `text` anuláveis, sem `CHECK`, porque o catálogo vive em código.

*Alternativa:* um objeto `execution` aninhado no JSON. Ele exigiria definir semântica de merge parcial no `PATCH`, enquanto os campos planos reaproveitam a regra "só o que foi enviado muda".

### 4. Ordem das validações no `Service.apply`

```
1. campos do provider (title/description)       -> ErrFieldNotEditable (422)
2. lock: execução enviada e status atual
   in (in_progress, done, cancelled)             -> ValidationError (422)
3. aplica instruções, labels, categoria, execução
4. transição de status                           -> ErrInvalidTransition (409)
5. se algum dos 4 campos foi enviado OU o status
   resultante é ready: valida contra o catálogo  -> ValidationError (422)
6. se o status resultante é ready: pré-requisitos -> ValidationError (422)
                                                    field "status", lista o que falta
```

- O lock usa o status **antes** do `PATCH`. Por isso, `{"status":"failed","model":"x"}` numa task `in_progress` é rejeitado.
- O passo 5 só valida quando é relevante. Se um modelo sair do catálogo, a task que o usa continua editável (labels, instruções) e só falha ao mexer na execução ou ao ir para `ready`.
- Os pré-requisitos ausentes são reunidos numa única mensagem, por exemplo `task cannot be ready: missing agent, effort, project_category.repo_url`, para o CRUD mostrar tudo de uma vez.
- `ErrInvalidTransition` vem antes dos pré-requisitos. Por isso, `cancelled → ready` continua `409`, e não `422`.

### 5. `repo_url` disponível dentro da mutação, sem consulta ao repositório

`CategoryRef` ganha `RepoURL *string`, e `getTask`/`ListTasks` passam a trazê-lo no join com `project_categories`. Quando o `PATCH` troca a categoria, o `Service` busca a categoria nova **antes** de chamar `UpdateTask` (com `GetCategory`; `ErrNotFound` vira o `ValidationError` de categoria inexistente, como hoje) e a função de mutação recebe o `CategoryRef` completo. Assim, o passo 6 lê `t.ProjectCategory.RepoURL` sem consultar o repositório dentro da mutação.

*Alternativa:* consultar a categoria dentro da função de mutação. Isso causa deadlock no `tasktest` e abre uma segunda conexão dentro da transação no Postgres.

### 6. Categoria: `repo_url`, `base_branch` e a regra de remoção

- Colunas `repo_url text NULL` e `base_branch text NOT NULL DEFAULT 'main'`. O `DEFAULT` preenche as linhas existentes na migração.
- Validação no pacote `task`, junto das demais: `net/url` com `Scheme == "https"`, host e path não vazios, `User == nil` e no máximo 500 caracteres. A branch é validada com uma regex (`^[A-Za-z0-9._/-]+$`) mais as checagens de início, fim e `..`.
- `CategoryPatch` ganha `SetRepoURL bool` + `RepoURL *string`, no mesmo padrão de `SetProjectCategory`, e `BaseBranch *string`. No HTTP, `repo_url` usa `Optional[string]`. `base_branch: null` é rejeitado com `422`.
- **Remover `repo_url` com tasks em `ready`:** antes de `UpdateCategory`, o `Service` consulta `ListTasks{ProjectCategoryID, Status: ready, Limit: 1}`. Se `total > 0`, retorna `ErrConflict`. Isso não exige método novo no `Repository`.

### 7. Endpoint `GET /execution-options`

Ele é registrado em `httpapi.Tasks.Register`, que já serve `/labels` e `/project-categories`, e lê `Service.ExecutionOptions()`, que devolve a visão ordenada do catálogo. O handler não tem lógica: só serializa.

### 8. Construção do `Service`

`task.NewService(repo, catalog *execution.Catalog)`. O argumento é obrigatório, sem functional options, porque um `Service` sem catálogo não consegue validar nada. Os chamadores (`main.go`, os testes de `task`, `httpapi`, `postgres` e `ingest`) passam um catálogo. Nos testes, `tasktest` ganha um `Catalog()` com o agente `claude` real mais um agente fake de dois modelos, para cobrir o cenário de troca de agente.

## Risks / Trade-offs

- **[Corrida categoria × task]** Entre o `ListTasks` da regra de remoção e o `UpdateCategory`, uma task pode ir para `ready`, e uma task pode ir para `ready` usando um `CategoryRef` lido antes de o `repo_url` ser removido. → Aceito: é um serviço local, de uso individual. A próxima change revalida `repo_url` ao iniciar a execução e falha o run com uma mensagem clara.
- **[Catálogo muda entre versões]** Uma task em `ready` com um modelo removido do catálogo fica num estado que não seria aceito hoje. → O executor da próxima change revalida contra o catálogo antes de rodar. Para corrigir, basta um `PATCH` com outro modelo, já que só o passo 5 valida.
- **[BREAKING no `PATCH` para `ready`]** Clientes que hoje liberam tasks sem configuração passam a receber `422`. → Não há clientes externos (ferramenta local). Atualizar o README e os testes de `httpapi` que fazem `draft → ready`.
- **[Aliases de modelo]** `opus`/`sonnet` mudam de versão sem aviso, e um run pode usar um modelo diferente do run anterior. → Aceito. A próxima change registra no run o modelo efetivamente usado (o stream-json do CLI informa).

## Migration Plan

1. `migrations/00002_add_execution_config.sql`: `ALTER TABLE` aditivos em `tasks` e `project_categories`. O `Down` remove as colunas.
2. `make sqlc` e commit do código gerado.
3. Tasks existentes em `ready` continuam em `ready`, sem configuração, porque a migração não mexe em status. Elas só passam pela validação no próximo `PATCH`. Num ambiente local, isso basta. Se necessário, essas tasks voltam para `draft` à mão.

Rollback: `make migrate-down` e reverter o código. Nenhum dado anterior é perdido.
