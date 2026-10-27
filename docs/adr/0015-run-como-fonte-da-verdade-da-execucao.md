# ADR-0015: Run como fonte da verdade da execução

**Status:** Aceito
**Data:** 2026-10-26

## Contexto

Cada execução de uma task é um run disparado por `POST /tasks/{id}/runs` e executado pelo worker do [ADR-0014](0014-worker-com-acesso-ao-docker-para-executar-os-runs.md). Um run pode durar até `RUN_TIMEOUT` (60 minutos por padrão).

O RabbitMQ fecha o canal de um consumer que segura uma mensagem sem ack por mais que o `consumer_timeout` (30 minutos por padrão), e a mensagem volta para a fila. A task e o run precisam concordar sobre o status, por exemplo `in_progress` enquanto o run está ativo. O cancelamento e a queda do worker não podem deixar runs nem containers perdidos.

## Decisão

- **Banco como fonte da verdade:** o run é uma linha da tabela `runs`. Ela guarda o snapshot da configuração, o resultado, `cancel_requested_at` e as datas. Um índice único parcial (`task_id WHERE status IN ('queued','running')`) garante no banco um run ativo por task.
- **Fila só como transporte:** a fila `tasks.runs` (quorum, com DLX `tasks.runs.dlx` e DLQ `tasks.runs.dlq` próprios, separados da ingestão) leva só o `run_id`.
- **Ack cedo:** o worker espera um slot livre (`MAX_CONCURRENT_RUNS`, com prefetch 1), marca o run como `running` com um `UPDATE ... WHERE status='queued'` e **só então** confirma a mensagem. A partir daí o run depende só do banco.
- **Mesma transação:** as transições do run e o efeito na task ficam numa transação só, no repositório do run, com SQL condicional sobre `tasks`.
  - O `Start` move a task de `ready` para `in_progress`.
  - O `Finish` move a task que ainda estiver em `in_progress` para `in_review`, `failed` ou `ready`.
  - O `Abort` desfaz um run que não pôde ser enfileirado.
- **Cancelamento pelo banco:**
  - Um run `queued` é finalizado na hora.
  - Um run `running` recebe `cancel_requested_at`. O worker consulta essa coluna a cada 2 s, mata o container e finaliza o run.
- **Recuperação na subida:** o worker remove os containers com o label `miner.run` e finaliza os runs `running` como `failed` (`worker restarted`), ou como `cancelled` se havia um pedido de cancelamento. Os runs `queued` continuam na fila.
- **Porta para o task:** o `task` não importa o `run`. Ele recebe a porta `task.RunGuard`, implementada por `run.Guard`, e com ela bloqueia o status e o `DELETE` com run ativo, e cancela o run quando a task é cancelada.

## Alternativas consideradas

- **Ack só no fim do run:** estoura o `consumer_timeout` em runs longos, e o RabbitMQ reentrega um run que já está rodando.
- **Exchange fanout de controle para o cancelamento:** uma fila exclusiva por worker muda de nome a cada reconexão, e a mensagem se perde com o worker fora do ar. O polling custa uma consulta por chave primária a cada 2 s por run ativo.
- **`run.Service` chamar o `task.Service.Update` depois de gravar o run:** perderia a atomicidade, porque o run poderia ficar `running` com a task em `ready`, e esbarraria na própria regra de "status bloqueado com run ativo".

## Consequências

### Positivas

- Uma queda do worker ou do RabbitMQ não perde runs: o banco diz o que estava rodando e a subida recupera.
- A task e o run nunca divergem, e um run duplicado é barrado pelo próprio banco.
- O cancelamento funciona mesmo com o broker fora do ar.

### Negativas

- Uma mensagem que vai para a DLQ deixa o run `queued` e bloqueia a task. A saída é cancelar o run por `POST /runs/{id}/cancel`, que não depende da fila.
- O cancelamento de um run em execução leva até 2 s para ser percebido.
- Entre o `HasActiveRun` do `task` e a gravação pode haver corrida. No pior caso, um `PATCH` de status é recusado com `409` sem necessidade.

## Referências

- `migrations/00003_create_runs.sql`, `internal/execution/run/`, `internal/execution/run/postgres/`, `internal/execution/run/worker/`
- `openspec/changes/add-host-task-runner/design.md`
- [ADR-0010](0010-rabbitmq-com-quorum-queue-e-dlq-para-a-ingestao-de-tasks.md), [ADR-0014](0014-worker-com-acesso-ao-docker-para-executar-os-runs.md)
