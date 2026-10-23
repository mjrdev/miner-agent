# Design

## Context

O serviço hoje é só o bootstrap:
- `cmd/api/main.go` monta config, logger, pool `pgx/v5`, `http.ServeMux` e graceful shutdown;
- `internal/handler/health.go` faz o readiness contra uma interface `database.Pinger`;
- `migrations/` está vazia e não há ferramenta de migração.

O ADR-0002 adiou para "quando houver domínio" a decisão de onde ficam entidades e regras. Esta change é esse momento. Motivação e escopo estão em `proposal.md`, e o comportamento observável está nas specs da change (`task-ingestion`, `task-management`, `project-categories` e os deltas de `health-checks`, `service-lifecycle` e `local-dev-environment`).

Decisões tomadas com a pessoa usuária na exploração: RabbitMQ como fila, sqlc para acesso ao banco, goose para migrações, adapter executado no consumer, separação de donos dos campos (opção "B"), tasks nascendo em `draft`, categoria como entidade, labels em tabela própria e instruções adicionais como lista ordenada.

## Goals / Non-Goals

**Goals:**
- Um contrato de adapter para que adicionar Jira, GitHub ou Trello seja só implementar `Verify` e `Parse`, sem mexer na fila, no consumer nem no banco.
- Ingestão at-least-once com gravação idempotente: nenhuma entrega aceita se perde e nenhuma reentrega duplica uma task.
- SQL explícito e tipado (sqlc), com o mesmo arquivo de migração servindo de schema para o sqlc e para o banco.
- Testes de unidade sem infraestrutura (domínio, adapter, handlers com fakes) e testes de integração opcionais contra Postgres e RabbitMQ reais.

**Non-Goals:**
- Consumer em processo separado ou escalável horizontalmente. Ele roda como goroutine na API e pode ser extraído depois.
- Backoff exponencial entre reentregas e filas de retry com TTL. A reentrega é imediata, com um pequeno atraso fixo.
- Um outbox transacional. Não é necessário, porque o handler não grava nada no banco antes de publicar.
- CRUD de labels além da listagem (renomear ou remover labels).

## Decisions

### 1. Layout do domínio em `internal/task/`

```
internal/
  broker/                 conexão RabbitMQ: connect com retry, reconexão, topologia,
                          publish com confirm, consume; implementa Ping
  task/                   domínio (package task)
    task.go               Task, Status, transições, ProjectCategory, erros de domínio
    event.go              Event, EventType (created|updated|closed)
    service.go            regras: Create, Update, Delete, List, ApplyEvent; categorias; labels
    repository.go         interface Repository consumida pelo service
    postgres/             implementação do Repository
      queries/*.sql       consultas do sqlc
      sqlcdb/             código gerado (commitado, não editar)
    adapter/              interface Adapter + Registry
      local/              adapter "local"
    ingest/               Envelope da mensagem, Publisher, Consumer
    httpapi/              handlers /webhooks, /tasks, /project-categories, /labels
cmd/migrate/              aplica migrações (up | down | status)
migrations/               *.sql do goose + embed.go (package migrations)
```

As dependências apontam para dentro: `httpapi`, `ingest` e `postgres` importam `task`, e `task` não importa nenhum deles. O `service` depende só da interface `Repository`, então os testes usam um fake em memória. Categorias e labels ficam no pacote `task` porque só existem em função das tasks. Se crescerem, viram pacotes próprios.

*Alternativas:*
- Pacotes por camada no topo (`internal/handler/task.go`, `internal/repository/...`): espalha um único domínio por vários pacotes e incentiva dependências cruzadas.
- Hexagonal completo (`ports/`, `adapters/`): cerimônia demais para um domínio.

Registrado no ADR-0009.

### 2. RabbitMQ: topologia e semântica de entrega

```
POST /webhooks/{provider}
   |  publish (persistent, confirm)  routing key: webhook.<provider>
   v
[tasks.webhooks]  topic, durable
   |  binding webhook.*
   v
[tasks.ingest]  quorum, durable, x-delivery-limit=4, x-dead-letter-exchange=tasks.dlx
   |                                   |
   v  consumer (prefetch 10)           | nack(requeue=false) ou limite excedido
 ack após commit                       v
                               [tasks.dlx] fanout --> [tasks.ingest.dlq] quorum, durable
```

- **Cliente:** `github.com/rabbitmq/amqp091-go`.
- **Topologia:** declarada pela própria aplicação na conexão e em cada reconexão, de forma idempotente. O ambiente local não precisa de definitions file.
- **Fila quorum com `x-delivery-limit`:** o broker conta as reentregas sozinho, sem headers de retry escritos pela aplicação. O limite 4 significa até 4 devoluções, ou seja, 5 entregas, como pede a spec. Um teste de integração confere essa contagem.
- **Classificação de erros no consumer:**
  - `adapter.ErrInvalidPayload` (permanente) → `Nack(requeue=false)` → DLQ imediata.
  - Erro de banco (transitório) → espera de 1 s → `Nack(requeue=true)`.
  - Evento `updated`/`closed` → `Ack` com log de "ignorado".
- **Publicação:** um canal dedicado em modo confirm, protegido por mutex, com `PublishWithDeferredConfirmWithContext` e 5 s de espera. Sem confirmação, a resposta é 503. O volume de webhooks é baixo, e um canal serializado basta.
- **Envelope (JSON):** `{"provider","delivery_id","received_at","payload"}`, com `payload` como `json.RawMessage`. O `message_id` AMQP é o `delivery_id` (UUID v7 gerado no handler).
- **Consumer:** uma goroutine processando mensagens em sequência, com prefetch 10. Isso basta para o volume atual e evita corridas no upsert.

*Alternativas:* a fila em Postgres com `SKIP LOCKED` (recomendada na exploração, mas a pessoa usuária escolheu RabbitMQ) e Redis Streams ou NATS (infraestrutura sem uso no projeto). Uma fila *classic* exigiria contar tentativas em header e republicar.

Registrado no ADR-0010.

### 3. Conexão, reconexão e readiness do broker

`broker.Connect` repete as tentativas por até 30 s, como `database.Connect`. Depois, uma goroutine observa `NotifyClose`:
- quando a conexão cai, marca o estado como desconectado e tenta reconectar com backoff de 1 s, 2 s, 4 s… até no máximo 30 s;
- quando reconecta, redeclara a topologia e chama os *callbacks* registrados, o que reabre o canal de publish e reinicia o consumo.

`Ping(ctx)` devolve erro enquanto o estado for desconectado. Assim o broker satisfaz a mesma interface `Pinger`, e `handler.Health` passa a receber um mapa nome → `Pinger` (`database`, `broker`), checado em paralelo com o timeout de 2 s. As chaves do JSON saem ordenadas porque o `encoding/json` ordena mapas.

### 4. Ordem de subida e de encerramento

```
subida:      config -> logger -> db.Connect -> broker.Connect (+topologia)
             -> consumer.Start -> http server
encerramento (SIGTERM):
             http Shutdown (para de aceitar e publicar)
             -> consumer.Stop (cancela o consume e aguarda a mensagem em curso)
             -> broker.Close -> pool.Close
```

O processamento de cada mensagem usa um contexto **separado** do contexto do sinal. Assim, um `SIGTERM` não aborta uma gravação no meio. `consumer.Stop` espera até o que sobrar de `SHUTDOWN_TIMEOUT` e, se estourar, retorna. A mensagem não confirmada volta para a fila quando o canal fecha.

### 5. Modelo de dados

```sql
project_categories(id uuid PK, name text, slug text UNIQUE, description text DEFAULT '',
                   created_at timestamptz, updated_at timestamptz)

tasks(id uuid PK, provider text, external_id text, external_url text NULL,
      title text, description text, additional_instructions text[] DEFAULT '{}',
      project_category_id uuid NULL REFERENCES project_categories ON DELETE RESTRICT,
      status text CHECK (status IN ('draft','ready','in_progress','in_review',
                                    'done','failed','cancelled')) DEFAULT 'draft',
      raw_payload jsonb NULL, created_at timestamptz, updated_at timestamptz,
      UNIQUE (provider, external_id))
  INDEX (status), INDEX (project_category_id), INDEX (created_at DESC)

labels(id uuid PK, name text UNIQUE)
task_labels(task_id uuid REFERENCES tasks ON DELETE CASCADE,
            label_id uuid REFERENCES labels ON DELETE CASCADE,
            PRIMARY KEY (task_id, label_id))
  INDEX (label_id)
```

- **IDs:** UUID v7 gerados em Go (`github.com/google/uuid`), ordenáveis por tempo. Gerar no Go também permite que `POST /tasks` defina `external_id = id` no mesmo `INSERT`.
- **`updated_at`:** atualizado pelas próprias queries de `UPDATE`, sem trigger, para manter a regra visível no SQL.
- **Ingestão idempotente:** `INSERT ... ON CONFLICT (provider, external_id) DO NOTHING`.
- **`ON DELETE RESTRICT` na categoria:** implementa o 409 de categoria em uso. O service mapeia a violação de FK (SQLSTATE `23503`) para `ErrConflict`. Da mesma forma, violação de unicidade de slug (`23505`) vira `ErrConflict`.
- **`text[]` para instruções:** a lista é lida e substituída sempre inteira, nunca consultada item a item. Não justifica uma tabela.
- **Transições de status:** validadas no domínio (`Status.CanTransitionTo`) por uma tabela fixa, conforme a spec. O `CHECK` no banco é só a última barreira.

### 6. sqlc

- **Configuração:** `sqlc.yaml` na raiz, com `engine: postgresql`, `schema: migrations/` (o sqlc entende as anotações `-- +goose Up/Down`), `queries: internal/task/postgres/queries/` e `sql_package: pgx/v5`, sobre o pool que já existe. *Overrides* mapeiam `uuid` para `uuid.UUID`.
- **Execução:** pela imagem `sqlc/sqlc` (versão fixada) via `make sqlc`. O sqlc depende de cgo/`pg_query`, então rodar pelo Docker evita adicioná-lo ao `go.mod` e mantém o "não precisa de nada além de Docker". O código gerado é commitado.
- **Filtros opcionais da listagem:** `sqlc.narg`, no padrão `(sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status'))`. O filtro por label usa `EXISTS` sobre `task_labels`/`labels`. O `total` vem de uma query `CountTasks` com os mesmos filtros: `count(*) OVER ()` na própria listagem devolveria 0 quando o `offset` passa do fim.
- **Labels da listagem:** uma segunda query (`WHERE task_id = ANY($1::uuid[])`) carrega as labels das tasks da página, sem N+1.
- **Escritas compostas:** `PATCH` com labels e criação com labels rodam em `pgx.Tx`, via `sqlcdb.New(tx)`: upsert das labels, `DELETE` + `INSERT` em `task_labels` e `UPDATE` da task.

*Alternativas:* pgx puro (mapeamento manual), sqlx/scany (erros de coluna só em runtime) e ORMs (escondem o SQL e lidam mal com `text[]`, `jsonb` e `ON CONFLICT`). Registrado no ADR-0011.

### 7. goose

- **Arquivos:** migrações em SQL, `migrations/NNNNN_descricao.sql`, com numeração sequencial (`goose create -s`).
- **Embed:** `migrations/embed.go` declara `package migrations` com `//go:embed *.sql`.
- **Comando:** `cmd/migrate` usa `goose.NewProvider(goose.DialectPostgres, db, migrations.FS)` sobre `database/sql` com `pgx/v5/stdlib`, e aceita `up`, `down` e `status`.
- **No compose:** um serviço `migrate` (imagem `dev`, `go run ./cmd/migrate up`) depende de `postgres` saudável, e `api` depende de `migrate: service_completed_successfully`. Fora do compose, usa-se `make migrate` e `make migrate-down`.
- **Por que não rodar no startup da API:** com várias réplicas, todas tentariam migrar. O passo separado já resolve isso, e embutir no startup continua possível mais tarde.

*Alternativas:* golang-migrate (dois arquivos por migração e estado "dirty" manual), tern e Atlas (mais pesado). Registrado no ADR-0012.

### 8. Contrato do adapter

```go
type Adapter interface {
    Name() string
    Verify(h http.Header, body []byte) error   // no handler; erro => 401
    Parse(d Delivery) (task.Event, error)      // no consumer; ErrInvalidPayload => DLQ
}
type Delivery struct { ID string; Body []byte }  // ID = delivery_id
```

O `Registry` é um mapa nome → Adapter montado em `main.go`. O handler resolve `{provider}` por ele, e uma busca sem resultado responde 404. O adapter `local` funciona assim:
- `Verify` compara `X-Webhook-Token` com `subtle.ConstantTimeCompare` e sempre falha se o token configurado estiver vazio.
- `Parse` decodifica `{type?, external_id?, url?, title, description}` com `DisallowUnknownFields`, valida os campos obrigatórios e usa `d.ID` quando `external_id` está vazio. Por isso uma reentrega da mesma mensagem cai na mesma chave e não duplica a task.

### 9. HTTP

- **Rotas:** registradas pelo `httpapi` no mesmo `ServeMux`, com padrões do Go 1.22+ (`POST /webhooks/{provider}`, `PATCH /tasks/{id}`…). `server.NewRouter` passa a receber os registradores de rota em vez de só o `Health`.
- **Corpos:** limitados com `http.MaxBytesReader`, 1 MiB nos webhooks e no CRUD. `*http.MaxBytesError` vira 413.
- **CRUD:** usa `DisallowUnknownFields`. Um id inválido no path (não UUID) vira 404.
- **`PATCH`:** usa um tipo `Optional[T]` (definido? + valor) para separar campo ausente de `null`, necessário em `project_category_id: null`.
- **Erros:** os de domínio (`ErrNotFound`, `ValidationError{Field}`, `ErrInvalidTransition`, `ErrFieldNotEditable`, `ErrConflict`) viram status e código num único helper, seguindo a tabela da spec "Error responses". Erros inesperados viram 500 genérico com log.

### 10. Testes

- **Unidade:** transições de status (tabela completa), normalização de labels e slug, adapter `local` (`Verify`/`Parse`), service com repositório fake, handlers com `httptest` e fakes de service e publisher, e config.
- **Integração:** repositório Postgres e broker (publish/confirm, DLQ, contagem de entregas, reconexão), ativados por `TEST_DATABASE_URL` e `TEST_RABBITMQ_URL` e pulados sem elas. `make test-integration` roda dentro do compose.

## Risks / Trade-offs

- [Remover uma task externa e o provider reenviar o mesmo `created` faz a task voltar] → Aceito por ora: hoje só o `local` existe, e o reenvio vem do próprio cliente. Quando houver providers reais, avaliar soft delete ou uma tabela de chaves removidas.
- [Reentrega imediata pode esgotar as 5 tentativas numa queda curta do banco] → Atraso fixo de 1 s antes do `nack`. As mensagens esgotadas ficam na DLQ e podem ser reenviadas pelo painel do RabbitMQ.
- [Consumer sequencial limita a vazão] → Suficiente para webhooks de tickets. A concorrência pode ser configurada depois sem mudar o contrato.
- [`x-delivery-limit` tem semântica sutil (devoluções contra entregas)] → Teste de integração que confere exatamente 5 entregas antes da DLQ.
- [Sem autenticação no CRUD] → Fora de escopo e documentado no README. O serviço é só local por enquanto.
- [Código gerado do sqlc desatualizado em relação às queries] → `make sqlc` documentado. Uma verificação em CI (`sqlc diff`) fica para quando houver CI.
- [Webhook aceito com 202 e depois descartado por payload inválido não dá retorno ao chamador] → O caminho síncrono `POST /tasks` existe para quem precisa de validação imediata. A DLQ e o log registram o motivo.

## Migration Plan

São só tabelas novas, sem dados existentes.
- **Ambiente local:** o `docker compose up` aplica as migrações automaticamente.
- **Rollback:** `make migrate-down` reverte a última migração. Reverter o código basta para o resto, e a fila pode ser removida pelo painel.
- **Variáveis novas:** `RABBITMQ_URL` é obrigatória. Ambientes fora do compose precisam defini-la antes de subir a nova versão.
