# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Fluxo de specs (OpenSpec)

- Toda nova change do OpenSpec (propose) começa na branch `spec/<change-name>` criada a partir da `main` — siga a skill `spec-git-flow` (Fase 1) antes de `openspec new change`.
- Ao arquivar uma change, siga a skill `spec-git-flow` (Fase 2): commits, push e PR para a `main` automaticamente.
- Nunca commite direto na `main`.
- As specs vigentes ficam em `openspec/specs/<capability>/spec.md`; as changes arquivadas (com `design.md` e as decisões) em `openspec/changes/archive/`.
- Decisões de arquitetura novas viram ADR em `docs/adr/` (skill `adr`). Leia os ADRs antes de propor mudanças de layout, roteamento, banco, broker ou migrações.

## O que é

Serviço HTTP em Go (`github.com/jrgomesdev/miner`) que recebe tasks de providers externos por webhook, enfileira no RabbitMQ e grava no PostgreSQL, com um CRUD para revisar e enriquecer as tasks que alimentam um agente de desenvolvimento autônomo. Só roda localmente via Docker Compose; o CRUD não tem autenticação.

Documentação, specs e ADRs são em português; código, comentários, mensagens de log e mensagens de commit (Conventional Commits: `feat:`, `spec:`, `docs:`, `chore:`) são em inglês.

## Comandos

```bash
make up                 # imagem do agente + api e worker (hot reload com air) + migrate + postgres + rabbitmq
make logs               # logs da api
make test               # go test ./... no host — testes de integração são pulados
make test-integration   # todos os testes dentro do compose (cria o banco miner_test e o vhost miner_test)
make fmt                # gofmt -w + go vet
make lint               # golangci-lint
make sqlc               # regenera os sqlcdb/ de task e de run (via container sqlc)
make agent-image        # imagem miner-agent:default dos runs (build/agent/)
make test-agent-image   # testa o entrypoint miner-job na imagem com um agente fake
make migrate | make migrate-down
make psql
```

Um teste isolado:

```bash
go test ./internal/task -run TestUpdateStatus -v
# integração (depois de um `make test-integration`, que prepara o banco e o vhost):
docker compose run --rm test go test -count=1 -p 1 -run TestIngestionCreatesTasksOnce ./internal/task/ingest
```

Testes de integração dependem de `TEST_DATABASE_URL` / `TEST_RABBITMQ_URL` e chamam `t.Skip` quando ausentes; os de `internal/execution/host` usam o Docker local e são pulados sem ele (no `make test` do host eles rodam). Eles **apagam os dados** (TRUNCATE nas tabelas, purge das filas) e compartilham banco e vhost, por isso rodam com `-p 1`. Use `testdb.Pool(t)` para obter um pool migrado e limpo.

Postgres no host fica na porta **5433** (`postgres://miner:miner@localhost:5433/miner`); painel do RabbitMQ em `localhost:15672` (`miner`/`miner`).

## Arquitetura

Composição inteira em `cmd/api/main.go`: config → logger → pool pgx → broker (declara a topologia em toda (re)conexão) → registry de adapters → catálogo de execução → `task.Service` → consumer → router → servidor com graceful shutdown. A ordem dos `defer` importa: o consumer para antes do broker e do pool fecharem.

**Domínio em `internal/<domínio>/`** (ADR-0009). O pacote raiz `internal/task` tem entidades, regras de status, validação, `Service` e a interface `Repository`; subpacotes dependem dele, nunca o contrário:

- `adapter/` — contrato `Adapter` (`Name`/`Verify`/`Parse`) e `Registry`. Provider novo = subpacote novo (hoje só `local/`, autenticado por `X-Webhook-Token`) registrado em `main.go`; não precisa tocar fila, banco nem HTTP.
- `ingest/` — topologia, `Publisher` e `Consumer`.
- `postgres/` — implementação do `Repository` com sqlc.
- `httpapi/` — handlers de `/webhooks`, `/tasks`, `/project-categories`, `/labels`, `/execution-options` e dos runs (`/tasks/{id}/runs`, `/runs/...`). Dependem do `*task.Service` concreto; os testes usam o service real com o repositório em memória e o catálogo de `tasktest/` (`tasktest.Catalog()`: `claude` real + agente fake).

**Execução em `internal/execution/`** (ADR-0013): folha do grafo — **não importa `task`**; o `task` importa ele. Contratos `Agent` (`Name`/`Models`/`Command`/`Credential`) e `Runner` (`Run`/`Cleanup`), e o `Catalog` (agentes + nomes de runners, hoje só `host`). O catálogo é estático em código e montado **duas vezes**, em `cmd/api` e em `cmd/worker` — mantenha os dois iguais. Agente novo = subpacote novo (hoje `claude/`).

**Runs** (ADR-0014, ADR-0015):
- `execution/run` importa `task` e `execution`. Tem a entidade Run, o `Service` (Start/Cancel), a fila `tasks.runs` e o `Guard`. O `task` só conhece a porta `task.RunGuard` (`task.NewService(repo, catalog, runs)`; nos testes, `tasktest.NoRuns()`), que bloqueia `PATCH` de status e `DELETE` com run ativo e cancela o run quando a task é cancelada.
- O banco é a fonte da verdade: as transições do run e o efeito na task (`ready → in_progress` no start; `in_review`/`failed`/`ready` no fim) são SQL condicional na mesma transação do repositório do run. `runtest` imita isso sobre o `tasktest`; o teste de contrato `runtest.Contract` roda contra os dois.
- `execution/run/worker`: o handler espera um slot (`MAX_CONCURRENT_RUNS`, prefetch 1), faz `MarkRunning` e **dá ack logo**, antes de executar (por causa do `consumer_timeout` do RabbitMQ). Cancelamento por polling de `cancel_requested_at`; na subida, `Recover` falha os runs `running` órfãos e remove containers com label `miner.run`.
- `execution/host`: único pacote com o SDK do Docker (`moby/moby/client`). Os bind mounts usam caminhos **do host**: o worker descobre `./runs` inspecionando o próprio container (`RUNS_HOST_DIR` sobrescreve).
- O job (`build/agent/miner-job`) lê `prompt.md`/`command.json` do diretório do run, clona, roda o agente e commita; códigos de saída 10 clone, 11 sem mudanças, 12 agente. O `GIT_TOKEN` vai por arquivo apagado antes do agente, nunca por env.
- Só o serviço `worker` recebe segredos e o `docker.sock`; a api lista explicitamente suas variáveis (sem `env_file`) e monta `./runs` só leitura para servir os eventos.

Pacotes fora de domínio (`broker`, `database`, `server`, `config`, `logger`, `handler`) ficam direto em `internal/`. Roteamento usa `net/http.ServeMux` com middlewares próprios (sem framework); logs JSON com `log/slog`; config só por variáveis de ambiente.

**Fluxo de ingestão:** `POST /webhooks/{provider}` só faz `Verify` e publica um `Envelope` no exchange topic `tasks.webhooks` (routing key `webhook.<provider>`), respondendo `202` após o publisher confirm. O consumer lê a quorum queue `tasks.ingest`, faz `Parse` e `Service.ApplyEvent`. Classificação de erro no consumer:
- envelope inválido, provider desconhecido, `adapter.ErrInvalidPayload` ou `*task.ValidationError` → `Nack` sem requeue → DLQ `tasks.ingest.dlq` imediatamente;
- qualquer outro erro → requeue após 1s; a `x-delivery-limit` (4) manda para a DLQ após 5 entregas.

A idempotência vem de `InsertTaskIfAbsent` por `(provider, external_id)`: reentregas não duplicam nem sobrescrevem edições. Por enquanto só eventos `created` são aplicados.

**Regras de domínio:** campos do provider (título, descrição, URL) só são editáveis no CRUD para tasks `local`; campos da pessoa usuária são instruções adicionais, labels, categoria, configuração de execução (`agent`/`model`/`effort`/`runner`) e status. Transições de status ficam no pacote `task` e uma inválida vira `409 invalid_transition`. `Repository.UpdateTask`/`UpdateCategory` recebem uma função de mutação aplicada atomicamente — as regras rodam dentro dela, e ela **não pode consultar o repositório** (deadlock no mutex do `tasktest`); por isso `Service.Update` busca a categoria nova antes. Ordem em `Service.apply`: campos do provider → lock da execução (`in_progress`/`done`/`cancelled`) → aplicação → transição (`409`) → validação no catálogo → pré-requisitos de `ready` (agente, modelo, runner, esforço quando o modelo tem, categoria com `repo_url`; `422` listando o que falta). Erros HTTP seguem `{"error":{"code","message"}}`.

**Banco:** migrações goose em `migrations/` (numeração sequencial, embutidas via `migrations.FS`, aplicadas pelo serviço `migrate` antes da api subir). Queries em `internal/task/postgres/queries/` e `internal/execution/run/postgres/queries/` (duas entradas em `sqlc.yaml`); após mudar queries ou migrações rode `make sqlc` e commite o código gerado em `sqlcdb/` (nunca edite à mão). O `testdb` faz `TRUNCATE` de todas as tabelas, incluindo `runs` — inclua tabelas novas lá.
