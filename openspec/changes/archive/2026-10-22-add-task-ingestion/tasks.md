# Tasks

## 1. Infraestrutura local e dependências

- [x] 1.1 Adicionar ao `docker-compose.yml` o serviço `rabbitmq` (`rabbitmq:4.1-management-alpine`, credenciais `RABBITMQ_DEFAULT_USER/PASS` com padrão `miner`, portas `${RABBITMQ_PORT:-5672}` e `${RABBITMQ_MANAGEMENT_PORT:-15672}`, volume `rabbitmqdata`, healthcheck `rabbitmq-diagnostics -q ping`). Adicionar também `RABBITMQ_URL` e `LOCAL_WEBHOOK_TOKEN` (padrão `local-dev-token`) no ambiente da `api`, e `api` passa a depender de `rabbitmq` saudável. Verificar com `docker compose config` sem erro e, após `make up`, o painel abrindo em `http://localhost:15672`
- [x] 1.2 Documentar no `.env.example` `RABBITMQ_URL`, `LOCAL_WEBHOOK_TOKEN`, `RABBITMQ_DEFAULT_USER/PASS` e as portas do RabbitMQ, e verificar que um `.env` copiado dele sobe o ambiente com `make up`
- [x] 1.3 Adicionar as dependências `github.com/rabbitmq/amqp091-go`, `github.com/pressly/goose/v3` e `github.com/google/uuid`, e verificar com `go mod tidy` e `go build ./...` sem erro

## 2. Configuração

- [x] 2.1 Adicionar `RabbitMQURL` (obrigatória) e `LocalWebhookToken` (opcional) em `internal/config`, e verificar com novos casos em `config_test.go`: padrões com `DATABASE_URL` e `RABBITMQ_URL` definidas, erro mencionando `RABBITMQ_URL` quando ausente e token vazio aceito. Rodar `go test ./internal/config/...`

## 3. Migrações (goose)

- [x] 3.1 Criar `migrations/00001_create_task_tables.sql` com as seções Up e Down para `project_categories`, `tasks` (com `UNIQUE (provider, external_id)`, `CHECK` de status e índices), `labels` e `task_labels`, conforme o design. Remover `migrations/.gitkeep`. Verificar aplicando e revertendo num Postgres limpo (tarefa 3.3)
- [x] 3.2 Criar `migrations/embed.go` (`package migrations`, `//go:embed *.sql`) e `cmd/migrate/main.go` com `up`, `down` e `status`, usando `goose.NewProvider` sobre `pgx/v5/stdlib` e lendo `DATABASE_URL`. Verificar que `go run ./cmd/migrate status` lista a migração pendente
- [x] 3.3 Adicionar ao compose o serviço `migrate` (imagem `dev`, `go run ./cmd/migrate up`, dependente de `postgres` saudável) e fazer `api` depender de `migrate: service_completed_successfully`. Adicionar `make migrate` e `make migrate-down`. Verificar que `make reset && make up` cria as tabelas (`make psql` com `\dt`), que `make migrate` repetido não altera nada e que `make migrate-down` seguido de `make migrate` funciona

## 4. Domínio Task

- [x] 4.1 Implementar em `internal/task/task.go` os tipos `Task`, `Status` (com a tabela de transições e `CanTransitionTo`), `ProjectCategory`, `Label` e os erros de domínio (`ErrNotFound`, `ValidationError`, `ErrInvalidTransition`, `ErrFieldNotEditable`, `ErrConflict`). Verificar com um teste de tabela que cobre todas as combinações origem→destino, incluindo terminais e mesmo status
- [x] 4.2 Implementar a normalização e validação de labels (trim, minúsculas, deduplicação, 1–50 caracteres), de instruções adicionais (até 50, não vazias, até 10.000 caracteres), de título (1–200) e de slug (derivação sem acentos e validação do formato). Verificar com testes de unidade que incluem `["Backend"," backend ","API"]` → `["api","backend"]` e `"Integrações Back-end"` → `integracoes-back-end`
- [x] 4.3 Implementar `internal/task/event.go` (`Event` e `EventType`) e a interface `Repository` em `repository.go`, com as operações que o service usa. Verificar com `go vet ./internal/task/...`
- [x] 4.4 Implementar `internal/task/service.go`: `CreateLocal`, `Get`, `List`, `Update` (com donos dos campos e transição de status), `Delete`, `ApplyEvent` (`created` → inserir em `draft` de forma idempotente; `updated`/`closed` → ignorar) e o CRUD de categorias e labels. Verificar com testes usando um repositório fake em memória: `PATCH` de descrição em task `jira` → `ErrFieldNotEditable`, transição inválida → `ErrInvalidTransition`, `ApplyEvent` repetido preservando edições e categoria inexistente → `ValidationError`

## 5. Repositório Postgres (sqlc)

- [x] 5.1 Criar `sqlc.yaml` (schema `migrations/`, queries `internal/task/postgres/queries/`, saída `internal/task/postgres/sqlcdb`, `sql_package: pgx/v5`, override de `uuid` para `uuid.UUID`) e o alvo `make sqlc` (imagem `sqlc/sqlc` com versão fixada). Verificar que `make sqlc` gera código que compila
- [x] 5.2 Escrever as queries de tasks: insert com `ON CONFLICT DO NOTHING`, get, list com `sqlc.narg` + `EXISTS` por label + `count(*) OVER ()`, update, delete e labels por `task_id = ANY(...)`. Escrever também as de categorias e de labels (upsert por nome, substituição em `task_labels`, listagem). Verificar com `make sqlc` e `go build ./...`
- [x] 5.3 Implementar `internal/task/postgres` satisfazendo `task.Repository`, com transações `pgx.Tx` nas escritas compostas e mapeamento de `23505`/`23503` para `ErrConflict` (ou `ValidationError` na FK de categoria inexistente). Verificar com testes de integração ativados por `TEST_DATABASE_URL`: upsert idempotente, filtros combinados, `total`, substituição de labels, ordem das instruções e categoria em uso → conflito

## 6. Broker RabbitMQ

- [x] 6.1 Implementar `internal/broker`:
  - `Connect` com retry por até 30 s e sem logar credenciais
  - declaração idempotente da topologia (`tasks.webhooks`, `tasks.ingest` quorum com `x-delivery-limit=4` e DLX, `tasks.dlx`, `tasks.ingest.dlq`)
  - `Ping`

  Verificar com teste de integração (`TEST_RABBITMQ_URL`) que as filas existem depois do `Connect` e com teste de unidade que o timeout é respeitado com um endereço inacessível
- [x] 6.2 Implementar a reconexão: `NotifyClose`, backoff de 1 s a 30 s, redeclaração da topologia e callbacks de reconexão, com `Ping` falhando enquanto estiver desconectado. Verificar com teste de integração que fecha a conexão à força e espera `Ping` voltar a `nil`
- [x] 6.3 Implementar o publish com confirm num canal dedicado protegido por mutex, com timeout de 5 s, e o consume com prefetch 10 e cancelamento por contexto. Verificar com teste de integração que uma mensagem publicada é recebida e que, sem broker, o publish retorna erro

## 7. Adapters e ingestão

- [x] 7.1 Implementar `internal/task/adapter` (`Adapter`, `Delivery`, `ErrInvalidPayload` e `Registry` com `Lookup`) e `adapter/local` (`Verify` com `subtle.ConstantTimeCompare`, falhando sempre com token vazio; `Parse` com validação e `external_id` igual ao `delivery_id` quando ausente). Verificar com testes de unidade cobrindo token correto, errado e vazio, payload sem `title`, campo desconhecido e `type` padrão `created`
- [x] 7.2 Implementar `internal/task/ingest`: `Envelope`, `Publisher` (routing key `webhook.<provider>`, `message_id` = `delivery_id`, mensagem persistente) e `Consumer` (adapter.Parse → `service.ApplyEvent`, `Ack` após o commit, `Nack(false)` para `ErrInvalidPayload`, `Nack(true)` com atraso de 1 s para erros transitórios, e `Stop` aguardando a mensagem em curso). Verificar com testes de unidade usando deliveries e service fakes para os três caminhos
- [x] 7.3 Teste de integração da ingestão (Postgres e RabbitMQ reais): uma mensagem válida cria a task; o mesmo `delivery_id` publicado duas vezes resulta numa única task; um payload inválido termina na `tasks.ingest.dlq`; um erro transitório forçado leva a exatamente 5 entregas antes da DLQ

## 8. API HTTP

- [x] 8.1 Criar os helpers de `httpapi`: decodificação JSON com `MaxBytesReader` de 1 MiB e `DisallowUnknownFields`, `Optional[T]` para o `PATCH` e mapeamento de erros de domínio para status e `{"error":{"code","message"}}`. Verificar com testes de unidade dos mapeamentos e do 413
- [x] 8.2 Implementar `POST /webhooks/{provider}` (404 para provider desconhecido, 401 em `Verify`, 413, 400 para corpo vazio ou JSON inválido, 202 com `delivery_id` após confirmação e 503 sem confirmação). Verificar com testes `httptest` usando publisher fake para cada caso
- [x] 8.3 Implementar `/tasks` (`POST`, `GET` com filtros e paginação, `GET /{id}`, `PATCH /{id}`, `DELETE /{id}`) e `GET /labels`. Verificar com testes `httptest` que cobrem os cenários das specs `task-management` (201 mínimo, 422, 400 de `limit=500`, 404 de id não UUID, 409 `invalid_transition`, 422 de descrição em task externa e 204)
- [x] 8.4 Implementar `/project-categories` (`POST`, `GET`, `GET /{id}`, `PATCH /{id}`, `DELETE /{id}`). Verificar com testes `httptest` dos cenários da spec `project-categories` (slug derivado, 409 de slug duplicado, 422 de slug inválido, ordenação por nome e 409 de categoria em uso)

## 9. Saúde, composição e encerramento

- [x] 9.1 Fazer `handler.Health` receber um mapa nome → `Pinger`, checado em paralelo com timeout de 2 s, e incluir `broker`. Verificar atualizando `health_test.go` para os cenários "Banco disponível", "Banco indisponível" e "Broker indisponível" da spec `health-checks`
- [x] 9.2 Atualizar `server.NewRouter` para receber os registradores de rota e `cmd/api/main.go` para montar tudo: db, broker, repository, service, registry com `local`, publisher, consumer e handlers. Logar aviso quando `LOCAL_WEBHOOK_TOKEN` estiver vazio. O encerramento segue a ordem HTTP → consumer → broker → pool, com o contexto de processamento separado do sinal. Verificar com `go build ./...`, com os testes de rota (404 e 405 continuam valendo) e com `make up` mostrando nos logs consumer iniciado e encerramento ordenado após `make down`

## 10. Documentação e ADRs

- [x] 10.1 Registrar com a skill `adr` os ADRs 0009 (layout do domínio em `internal/task`), 0010 (RabbitMQ com quorum queue, confirms e DLQ), 0011 (sqlc) e 0012 (goose com serviço `migrate`). Atualizar o ADR-0002 apenas como referência, se a skill indicar. Verificar que `docs/adr/README.md` lista os quatro
- [x] 10.2 Atualizar o `README.md` com o fluxo de ingestão, os exemplos `curl` (webhook `local` com `X-Webhook-Token`, `POST /tasks`, `PATCH` de status), o painel do RabbitMQ, `make migrate`, `make sqlc`, `make test-integration` e o aviso de que o CRUD não tem autenticação. Verificar executando os exemplos documentados contra `make up`

## 11. Verificação de ponta a ponta

- [x] 11.1 Adicionar o alvo `make test-integration` (roda `go test` dentro do compose com `TEST_DATABASE_URL` e `TEST_RABBITMQ_URL`) e verificar que todos os testes de integração passam, além de `make test` e `go vet ./...` limpos
- [x] 11.2 Validar manualmente os cenários de ambiente das specs:
  - `make reset && make up` sem `.env` e `readyz` com 200
  - webhook `local` com o token padrão e a task aparecendo em `GET /tasks`
  - com `docker compose restart rabbitmq`, `readyz` vai a 503 e depois volta a 200, e um novo webhook é processado
  - `SIGTERM` durante o consumo encerra com código 0
  - `openspec validate add-task-ingestion --strict` passa

## Workflow follow-up

- Arquivar a change com `/opsx:archive` depois da revisão. A skill `spec-git-flow` (Fase 2) faz os commits, o push da branch `spec/add-task-ingestion` e abre o PR para a `main`.
