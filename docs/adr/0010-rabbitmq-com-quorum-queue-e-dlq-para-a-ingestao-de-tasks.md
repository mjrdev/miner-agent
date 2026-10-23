# ADR-0010: RabbitMQ com quorum queue e DLQ para a ingestão de tasks

**Status:** Aceito
**Data:** 2026-10-22

## Contexto

Os webhooks dos providers (Jira, GitHub, Trello e o `local`) precisam ser respondidos rápido, para que o provider não reenvie por timeout, e nenhuma entrega aceita pode se perder. Por isso o handler só autentica e enfileira, e um consumer grava a task depois. Os providers também reenviam webhooks, então a gravação precisa ser idempotente. Antes desta change o projeto só tinha PostgreSQL.

## Decisão

Usamos **RabbitMQ** (cliente `github.com/rabbitmq/amqp091-go`), com esta topologia declarada pela própria aplicação a cada conexão:

- **Exchange `tasks.webhooks`** (topic): recebe mensagens persistentes com routing key `webhook.<provider>` e `message_id` igual ao `delivery_id`.
- **Fila `tasks.ingest`** (quorum): `x-delivery-limit=4` (até 5 entregas) e `x-dead-letter-exchange=tasks.dlx`.
- **Exchange `tasks.dlx`** (fanout): encaminha as mensagens descartadas para a fila `tasks.ingest.dlq` (quorum).

Semântica de entrega:
- **Publicação:** o handler responde `202` só depois do *publisher confirm*. Sem confirmação em 5 s, responde `503` para o provider reenviar.
- **Consumo:** sequencial, com prefetch 10.
  - Sucesso → `ack`, só depois do commit no banco.
  - Payload inválido → `nack` sem requeue, direto para a DLQ.
  - Erro transitório → `nack` com requeue depois de 1 s.
- **Duplicatas:** a entrega é at-least-once, e o upsert por `(provider, external_id)` absorve as reentregas.
- **Reconexão:** a conexão (`internal/broker`) reconecta sozinha com backoff de até 30 s. Enquanto estiver fora, `/readyz` e os webhooks respondem `503`.

## Alternativas consideradas

- **Fila em PostgreSQL com `FOR UPDATE SKIP LOCKED`**: era a recomendação inicial da exploração (zero infraestrutura nova, enfileirar e gravar na mesma transação). A pessoa usuária escolheu RabbitMQ.
- **Redis Streams / NATS JetStream**: infraestrutura sem nenhum outro uso no projeto.
- **Fila *classic* com contador de tentativas em header**: obriga a aplicação a republicar e contar tentativas. Na quorum queue, o broker conta as entregas sozinho.

## Consequências

### Positivas

- O webhook é respondido sem depender do banco, e nenhuma entrega aceita se perde.
- As falhas ficam visíveis e podem ser reenviadas pelo painel em `http://localhost:15672`.
- Retry e dead-letter vêm do broker, sem código de controle de tentativas.

### Negativas

- Mais um serviço para operar: `rabbitmq` no compose e `RABBITMQ_URL` obrigatória.
- A reentrega não tem backoff exponencial: as 5 tentativas acontecem em poucos segundos. Uma queda curta do banco pode mandar mensagens para a DLQ, de onde elas precisam ser reenviadas à mão.
- Um webhook aceito com `202` e depois descartado por payload inválido não avisa o chamador. Quem precisa de validação imediata usa `POST /tasks`.
- O consumer roda no processo da API, uma mensagem por vez.

## Referências

- `internal/broker/`, `internal/task/ingest/`, `docker-compose.yml`
- `openspec/changes/add-task-ingestion/design.md` (decisões 2 a 4)
