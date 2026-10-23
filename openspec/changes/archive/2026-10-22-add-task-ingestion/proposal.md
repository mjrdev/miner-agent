# Proposal

## Why

O `miner` vai alimentar um agente de desenvolvimento autônomo com tarefas cuja descrição já é o prompt de trabalho (instruções, critérios de aceite etc.). Essas tarefas nascem em ferramentas externas (Jira, GitHub, Trello…) ou são criadas diretamente, e hoje o serviço não tem nenhum domínio: só health checks. Precisamos do domínio Task com uma entrada desacoplada dos providers e um CRUD para que pessoas revisem e enriqueçam as tasks antes de liberá-las ao agente.

## What Changes

- **Ingestão por webhook:** novo grupo de rotas `POST /webhooks/{provider}`. O handler autentica a requisição com o adapter do provider, publica o payload bruto no RabbitMQ e responde `202 Accepted`.
- **Fila e consumer:** um consumer, no mesmo processo da API, lê a fila, normaliza o payload com o adapter do provider e grava a task de forma idempotente por `(provider, external_id)`. Mensagens que falham repetidamente vão para uma dead-letter queue.
- **Adapter comum:** contrato único (`Verify`, `Parse` e um evento normalizado com tipo `created`, `updated` ou `closed`) e um registry de providers. Nesta change só existe o adapter **`local`**, origem genérica para testes, tasks sem ticket em provider e fontes sem adapter próprio. Nesta change, só eventos `created` são aplicados.
- **Separação de donos dos campos:** o provider é dono de título, descrição, URL e payload bruto; a pessoa usuária é dona de instruções adicionais, labels, categoria e status. Título e descrição só são editáveis pelo CRUD em tasks `local`.
- **CRUD de tasks** (`/tasks`): criação síncrona de tasks `local`, listagem com filtros, leitura, edição parcial e remoção. As tasks têm instruções adicionais (lista ordenada), labels e categoria de projeto.
- **Ciclo de vida do status:** a task nasce em `draft`, e as transições (`draft`, `ready`, `in_progress`, `in_review`, `done`, `failed`, `cancelled`) são validadas no domínio. Por enquanto todas as transições acontecem pelo CRUD; o agente fica para uma change futura.
- **Categorias de projeto:** nova entidade cadastrável com CRUD próprio (`/project-categories`). **Labels:** criadas ao serem atribuídas e listáveis em `GET /labels`.
- **Infraestrutura:**
  - RabbitMQ (`amqp091-go`) no `docker-compose`.
  - Migrações com goose em `migrations/`, aplicadas por `make migrate` e automaticamente na subida do compose.
  - Acesso ao banco com código gerado pelo sqlc sobre o pool `pgx/v5` existente.
- **Saúde e ciclo de vida:** o readiness passa a verificar também o RabbitMQ. O serviço conecta ao broker com retry na subida e, no encerramento, para o consumer antes de fechar as conexões. Novas variáveis: `RABBITMQ_URL` (obrigatória) e `LOCAL_WEBHOOK_TOKEN`.
- **ADRs** para o layout do domínio, o RabbitMQ, o sqlc e o goose.

Fora de escopo: adapters reais (Jira, GitHub, Trello), aplicação de eventos `updated` e `closed`, o agente autônomo e os endpoints específicos dele (como `claim`), montagem do prompt final, autenticação do CRUD, UI e deploy.

## Capabilities

### New Capabilities
- `task-ingestion`: recebimento de webhooks por provider, contrato de adapter e registry, adapter `local`, publicação na fila, consumo idempotente e dead-letter.
- `task-management`: CRUD de tasks, regras de edição por dono do campo, ciclo de vida do status, instruções adicionais e labels.
- `project-categories`: cadastro e manutenção das categorias de projeto às quais as tasks são associadas.

### Modified Capabilities
- `health-checks`: o readiness passa a verificar o RabbitMQ, além do PostgreSQL.
- `service-lifecycle`: novas variáveis de configuração; conexão com o RabbitMQ na subida; o encerramento gracioso inclui parar o consumer e fechar a conexão com o broker.
- `local-dev-environment`: o compose sobe o RabbitMQ e aplica as migrações antes da API, mantendo a subida em um comando.

## Impact

- **Código novo:**
  - `internal/task/` (domínio, service, repository gerado pelo sqlc, adapters, fila, handlers HTTP)
  - `internal/broker/` (conexão com o RabbitMQ)
  - `cmd/migrate/`
  - `migrations/*.sql`
  - `sqlc.yaml`
- **Código alterado:**
  - `cmd/api/main.go` (composição)
  - `internal/config` (novas variáveis)
  - `internal/handler/health.go` (check do broker)
  - `internal/server/routes.go` (novas rotas)
- **Dependências novas:** `github.com/rabbitmq/amqp091-go`, `github.com/pressly/goose/v3`, `github.com/google/uuid`. O sqlc roda via imagem Docker e não entra no `go.mod`.
- **Infra:** serviços `rabbitmq` e `migrate` no `docker-compose.yml`; novas variáveis em `.env.example`; novos alvos no `Makefile` (`migrate`, `sqlc`).
- **APIs novas:** `/webhooks/{provider}`, `/tasks`, `/project-categories` e `/labels`. Os endpoints existentes não mudam, mas o corpo de `/readyz` ganha a chave `broker`.
- **Docs:** ADRs 0009 a 0012 em `docs/adr/`; `README.md` com o fluxo de ingestão e exemplos de uso.
