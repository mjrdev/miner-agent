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
make up                 # api (hot reload com air) + migrate + postgres + rabbitmq
make logs               # logs da api
make test               # go test ./... no host — testes de integração são pulados
make test-integration   # todos os testes dentro do compose (cria o banco miner_test e o vhost miner_test)
make fmt                # gofmt -w + go vet
make lint               # golangci-lint
make sqlc               # regenera internal/task/postgres/sqlcdb (via container sqlc)
make migrate | make migrate-down
make psql
```

Um teste isolado:

```bash
go test ./internal/task -run TestUpdateStatus -v
# integração (depois de um `make test-integration`, que prepara o banco e o vhost):
docker compose run --rm test go test -count=1 -p 1 -run TestIngestionCreatesTasksOnce ./internal/task/ingest
```

Testes de integração dependem de `TEST_DATABASE_URL` / `TEST_RABBITMQ_URL` e chamam `t.Skip` quando ausentes. Eles **apagam os dados** (TRUNCATE nas tabelas, purge das filas) e compartilham banco e vhost, por isso rodam com `-p 1`. Use `testdb.Pool(t)` para obter um pool migrado e limpo.

Postgres no host fica na porta **5433** (`postgres://miner:miner@localhost:5433/miner`); painel do RabbitMQ em `localhost:15672` (`miner`/`miner`).

## Arquitetura

Composição inteira em `cmd/api/main.go`: config → logger → pool pgx → broker (declara a topologia em toda (re)conexão) → registry de adapters → `task.Service` → consumer → router → servidor com graceful shutdown. A ordem dos `defer` importa: o consumer para antes do broker e do pool fecharem.

**Domínio em `internal/<domínio>/`** (ADR-0009). O pacote raiz `internal/task` tem entidades, regras de status, validação, `Service` e a interface `Repository`; subpacotes dependem dele, nunca o contrário:

- `adapter/` — contrato `Adapter` (`Name`/`Verify`/`Parse`) e `Registry`. Provider novo = subpacote novo (hoje só `local/`, autenticado por `X-Webhook-Token`) registrado em `main.go`; não precisa tocar fila, banco nem HTTP.
- `ingest/` — topologia, `Publisher` e `Consumer`.
- `postgres/` — implementação do `Repository` com sqlc.
- `httpapi/` — handlers de `/webhooks`, `/tasks`, `/project-categories`, `/labels`. Dependem do `*task.Service` concreto; os testes usam o service real com o repositório em memória de `tasktest/`.

Pacotes fora de domínio (`broker`, `database`, `server`, `config`, `logger`, `handler`) ficam direto em `internal/`. Roteamento usa `net/http.ServeMux` com middlewares próprios (sem framework); logs JSON com `log/slog`; config só por variáveis de ambiente.

**Fluxo de ingestão:** `POST /webhooks/{provider}` só faz `Verify` e publica um `Envelope` no exchange topic `tasks.webhooks` (routing key `webhook.<provider>`), respondendo `202` após o publisher confirm. O consumer lê a quorum queue `tasks.ingest`, faz `Parse` e `Service.ApplyEvent`. Classificação de erro no consumer:
- envelope inválido, provider desconhecido, `adapter.ErrInvalidPayload` ou `*task.ValidationError` → `Nack` sem requeue → DLQ `tasks.ingest.dlq` imediatamente;
- qualquer outro erro → requeue após 1s; a `x-delivery-limit` (4) manda para a DLQ após 5 entregas.

A idempotência vem de `InsertTaskIfAbsent` por `(provider, external_id)`: reentregas não duplicam nem sobrescrevem edições. Por enquanto só eventos `created` são aplicados.

**Regras de domínio:** campos do provider (título, descrição, URL) só são editáveis no CRUD para tasks `local`; campos da pessoa usuária são instruções adicionais, labels, categoria e status. Transições de status ficam no pacote `task` e uma inválida vira `409 invalid_transition`. `Repository.UpdateTask`/`UpdateCategory` recebem uma função de mutação aplicada atomicamente — as regras rodam dentro dela. Erros HTTP seguem `{"error":{"code","message"}}`.

**Banco:** migrações goose em `migrations/` (numeração sequencial, embutidas via `migrations.FS`, aplicadas pelo serviço `migrate` antes da api subir). Queries em `internal/task/postgres/queries/*.sql`; após mudar queries ou migrações rode `make sqlc` e commite o código gerado em `sqlcdb/` (nunca edite à mão).
