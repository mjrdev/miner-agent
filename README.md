# miner

Serviço HTTP em Go que recebe tasks de providers externos (Jira, GitHub, Trello…) por webhook, enfileira no RabbitMQ e as grava no PostgreSQL, com um CRUD para revisar e enriquecer as tasks que alimentam o agente de desenvolvimento autônomo. Preparado para desenvolvimento local com Docker.

## Pré-requisitos

- Docker com Docker Compose v2+
- `make`
- Go 1.26 (opcional, só para rodar testes fora do container)

> **WSL2:** mantenha o repositório no filesystem Linux (ex.: `~/dev/miner`), não em `/mnt/c/...`. Bind mounts a partir do disco do Windows deixam o hot reload e o build muito lentos.

## Como rodar

```bash
make up        # sobe api + postgres + rabbitmq em background
make logs      # acompanha os logs da api
```

A API fica em <http://localhost:8080>. Ela só inicia depois que o PostgreSQL e o RabbitMQ passam no healthcheck e o serviço `migrate` aplica as migrações pendentes. O painel do RabbitMQ fica em <http://localhost:15672> (usuário e senha `miner`), onde dá para ver as filas `tasks.ingest` e `tasks.ingest.dlq`.

O container da API roda com [air](https://github.com/air-verse/air): ao salvar qualquer arquivo `.go`, o serviço é recompilado e reiniciado automaticamente.

| Comando                 | O que faz                                                        |
|-------------------------|------------------------------------------------------------------|
| `make up`               | Builda e sobe a stack                                            |
| `make down`             | Para a stack mantendo os dados do banco e do RabbitMQ            |
| `make reset`            | Para a stack e **apaga** os dados do banco, do RabbitMQ (e o cache do Go) |
| `make logs`             | Segue os logs da API                                             |
| `make ps`               | Status dos containers                                            |
| `make psql`             | Abre um shell `psql` no banco                                    |
| `make migrate`          | Aplica as migrações pendentes                                    |
| `make migrate-down`     | Reverte a última migração                                        |
| `make sqlc`             | Regenera o código de acesso ao banco a partir das queries        |
| `make test`             | Roda `go test ./...` (os testes de integração são pulados)       |
| `make test-integration` | Roda todos os testes, inclusive integração, dentro do compose    |
| `make fmt`              | `gofmt` + `go vet`                                               |
| `make lint`             | `golangci-lint run` (precisa estar instalado)                    |
| `make build`            | Builda a imagem de runtime (`miner-api`)                         |

## Endpoints

| Método | Caminho    | Resposta                                                                   |
|--------|------------|----------------------------------------------------------------------------|
| GET    | `/healthz` | `200 {"status":"ok"}` enquanto o processo estiver de pé                     |
| GET    | `/readyz`  | `200 {"status":"ready","checks":{"broker":"ok","database":"ok"}}` ou `503` se o banco ou o RabbitMQ não responderem em 2s |

```bash
curl -i localhost:8080/healthz
curl -i localhost:8080/readyz
```

## Tasks

```
POST /webhooks/{provider} --> RabbitMQ (tasks.webhooks -> tasks.ingest) --> consumer --> PostgreSQL
                               |                                              |
                               +-- 5 entregas sem sucesso / payload inválido --+--> tasks.ingest.dlq
POST|GET|PATCH|DELETE /tasks --------------------------------------------------^
```

- O webhook só autentica e enfileira, e responde `202` com um `delivery_id` depois que o RabbitMQ confirma a mensagem. O consumer normaliza o payload com o adapter do provider e cria a task em `draft`. Reentregas com o mesmo `(provider, external_id)` não duplicam a task nem desfazem edições.
- Hoje só existe o provider `local`: tasks sem ticket em provider, testes locais e fontes sem adapter próprio. Ele autentica pelo header `X-Webhook-Token` (valor de `LOCAL_WEBHOOK_TOKEN`, `local-dev-token` em desenvolvimento). O corpo aceita `title` e `description` obrigatórios e `type` (`created`, `updated` ou `closed`; por enquanto só `created` é aplicado), `external_id` e `url` opcionais.
- Campos do provider: título, descrição (o prompt), URL. Só são editáveis pelo CRUD em tasks `local`. Campos da pessoa usuária: instruções adicionais (lista ordenada), labels, categoria de projeto, configuração de execução e status.
- Status: `draft → ready → in_progress → in_review → done`, com `ready → draft`, `in_progress → failed`, `failed → ready`, `in_review → ready` e `cancelled` a partir de qualquer estado não terminal. Uma transição inválida responde `409`.
- Configuração de execução: `agent`, `model`, `effort` e `runner`, validados contra o catálogo de `GET /execution-options` (`null` limpa o campo). Ela não muda enquanto a task está em `in_progress`, `done` ou `cancelled`.
- Uma task só fica em `ready` com `agent`, `model`, `runner` (e `effort`, quando o modelo tem esforços) e uma categoria com `repo_url`. Faltando algo, a transição, ou qualquer edição de uma task já em `ready`, responde `422` com a lista do que falta. Remover o `repo_url` de uma categoria com tasks em `ready` responde `409`.

> O CRUD **não tem autenticação**. Não exponha o serviço fora do ambiente local.

```bash
# Task via webhook (assíncrono)
curl -i -X POST localhost:8080/webhooks/local \
  -H 'X-Webhook-Token: local-dev-token' \
  -d '{"title":"Corrigir login","description":"Implemente ... Critérios de aceite: ..."}'

# Task direta (síncrona, devolve a task)
curl -i -X POST localhost:8080/tasks \
  -d '{"title":"Adicionar paginação","description":"Implemente ...","labels":["backend"]}'

# Listar com filtros (status, provider, project_category_id, label, limit, offset)
curl -s 'localhost:8080/tasks?status=draft&label=backend'

# Agentes, modelos, esforços e runners disponíveis
curl -s localhost:8080/execution-options

# Categoria de projeto com o repositório que o agente clona (base_branch padrão: main)
curl -s -X POST localhost:8080/project-categories \
  -d '{"name":"Backend","repo_url":"https://github.com/org/backend","base_branch":"develop"}'

# Configurar a execução e liberar a task para o agente
curl -i -X PATCH localhost:8080/tasks/<id> \
  -d '{"project_category_id":"<categoria>","agent":"claude","model":"opus","effort":"high","runner":"host"}'
curl -i -X PATCH localhost:8080/tasks/<id> -d '{"status":"ready"}'
```

| Método | Caminho                       | Descrição |
|--------|-------------------------------|-----------|
| POST   | `/webhooks/{provider}`        | Recebe um webhook: `202`, `401`, `404` (provider desconhecido), `400`/`413` (corpo) ou `503` (RabbitMQ indisponível) |
| POST   | `/tasks`                      | Cria uma task `local` em `draft` |
| GET    | `/tasks`                      | Lista `{"items":[...],"total":n}`, da mais recente para a mais antiga (`limit` padrão 50, máximo 200) |
| GET    | `/tasks/{id}`                 | Lê uma task |
| PATCH  | `/tasks/{id}`                 | Altera os campos enviados (`null` em `project_category_id`, `agent`, `model`, `effort` ou `runner` limpa o campo) |
| DELETE | `/tasks/{id}`                 | Remove uma task |
| GET    | `/labels`                     | Lista as labels existentes |
| GET    | `/execution-options`          | Lista `{"agents":[{"name","models":[{"id","efforts"}]}],"runners":[...]}` |
| POST/GET | `/project-categories`       | Cria (slug derivado do nome quando omitido) e lista categorias |
| GET/PATCH/DELETE | `/project-categories/{id}` | Lê, altera (`repo_url: null` remove o repositório) e remove (`409` se houver tasks associadas) |

Erros usam `{"error":{"code":"...","message":"..."}}`, com os códigos `validation_failed`, `not_found`, `conflict`, `invalid_transition`, `unauthorized`, `payload_too_large`, `unavailable` e `internal_error`.

## Configuração

Os valores padrão já funcionam. Para sobrescrever, copie `.env.example` para `.env` (não versionado) e ajuste:

| Variável           | Padrão                                                   | Descrição                                   |
|--------------------|----------------------------------------------------------|---------------------------------------------|
| `APP_ENV`          | `development`                                            | Nome do ambiente, aparece nos logs          |
| `HTTP_PORT`        | `8080`                                                   | Porta da API (no container e no host)       |
| `LOG_LEVEL`        | `info`                                                   | `debug`, `info`, `warn` ou `error`          |
| `SHUTDOWN_TIMEOUT` | `10s`                                                    | Tempo máximo para encerramento gracioso     |
| `DATABASE_URL`     | `postgres://miner:miner@postgres:5432/miner?sslmode=disable` | Conexão com o banco (obrigatória fora do compose) |
| `RABBITMQ_URL`     | `amqp://miner:miner@rabbitmq:5672/`                      | Conexão com o RabbitMQ (obrigatória fora do compose) |
| `LOCAL_WEBHOOK_TOKEN` | `local-dev-token`                                     | Token do webhook `local`; vazio rejeita todas as requisições |
| `POSTGRES_USER`    | `miner`                                                  | Usuário do banco                            |
| `POSTGRES_PASSWORD`| `miner`                                                  | Senha do banco                              |
| `POSTGRES_DB`      | `miner`                                                  | Nome do banco                               |
| `POSTGRES_PORT`    | `5433`                                                   | Porta do banco **no host** (5433 evita conflito com um Postgres local na 5432) |
| `RABBITMQ_DEFAULT_USER` / `RABBITMQ_DEFAULT_PASS` | `miner` / `miner`             | Credenciais do RabbitMQ |
| `RABBITMQ_PORT`    | `5672`                                                   | Porta AMQP **no host** |
| `RABBITMQ_MANAGEMENT_PORT` | `15672`                                          | Porta do painel do RabbitMQ **no host** |

Para conectar um cliente externo (DBeaver, psql local): `postgres://miner:miner@localhost:5433/miner`.

> As credenciais `miner/miner` servem **apenas para desenvolvimento local**. Não as use em nenhum outro ambiente.

## Banco de dados

Os dados ficam nos volumes `miner_pgdata` e `miner_rabbitmqdata` e sobrevivem a `make down`. Para começar do zero (isso também remove os volumes de cache do Go, então o próximo build demora um pouco mais):

```bash
make reset && make up
```

As migrações ficam em `migrations/` (SQL do [goose](https://github.com/pressly/goose), numeração sequencial). Elas são aplicadas automaticamente pelo serviço `migrate` em `make up` e manualmente com `make migrate` ou `make migrate-down`. As queries ficam em `internal/task/postgres/queries/`. Depois de mudar queries ou migrações, rode `make sqlc` e commite o código gerado em `internal/task/postgres/sqlcdb/`.

Os testes de integração usam o banco `miner_test` e o vhost `miner_test` do RabbitMQ, que o `make test-integration` cria e cujos dados os testes apagam. Para rodá-los fora do compose, defina `TEST_DATABASE_URL` e `TEST_RABBITMQ_URL` apontando para um banco e um vhost descartáveis e use `go test -p 1 ./...`.

## Estrutura

```
cmd/api/               ponto de entrada do binário
cmd/migrate/           aplica as migrações (up | down | status)
internal/config/       leitura e validação das variáveis de ambiente
internal/logger/       logger JSON (slog)
internal/database/     pool de conexões PostgreSQL (pgx)
internal/broker/       conexão com o RabbitMQ (retry, reconexão, confirms)
internal/handler/      handlers de saúde
internal/server/       rotas, middlewares e servidor com graceful shutdown
internal/execution/    catálogo de agentes (contrato Agent) e runners
  claude/              agente do Claude Code
internal/task/         domínio Task: entidades, regras e service
  adapter/             contrato dos providers e adapter local
  ingest/              topologia, publisher e consumer da fila
  postgres/            repositório (sqlc)
  httpapi/             handlers de webhooks, tasks, categorias, labels e opções de execução
migrations/            migrações SQL (goose)
docs/adr/              decisões de arquitetura (ADRs)
```

## Decisões de arquitetura

O porquê de cada escolha técnica (layout, roteador, logs, configuração, banco, ambiente Docker) está registrado em ADRs em [`docs/adr/`](docs/adr/README.md). Para registrar uma decisão nova, use `/adr <decisão>` no Claude Code ou copie [`docs/adr/template.md`](docs/adr/template.md).
