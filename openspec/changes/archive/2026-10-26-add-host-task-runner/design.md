# Design

## Context

A motivação está em `proposal.md` e o comportamento, nas specs da change (`task-runs` e os deltas de `task-management`, `execution-catalog` e `local-dev-environment`). Do estado atual, o que molda esta change é:

- **Domínio `execution`:** `internal/execution` é folha (ADR-0013). Ele tem `Agent` (`Name`, `Models`), `Catalog` e o agente `claude`. Os runners são só nomes. Na change anterior ficou combinado que o worker que junta task, agente e runner vive num subpacote de `execution` que importa os dois.
- **Regras de task:** ficam no `task.Service`, que valida a execução e os pré-requisitos de `ready` em `validateExecution` e `checkReady`.
- **Mutação atômica:** `Repository.UpdateTask` aplica uma função de mutação, que não pode consultar o repositório.
- **Broker:** `broker.Connect` recebe uma única `SetupFunc` de topologia. `Consume` chama o handler **sequencialmente**, uma mensagem por vez, com `prefetch` de buffer.
- **DLX compartilhado:** o DLX `tasks.dlx` é um fanout da ingestão. Ligar outra DLQ a ele misturaria as mensagens mortas.
- **Tempo de ack:** o RabbitMQ fecha o canal de um consumer que segura uma mensagem sem ack por mais que o `consumer_timeout` (padrão de 30 minutos), e um run pode durar 60.
- **Containers:** a api roda num container do compose sem acesso ao Docker do host. Os bind mounts de um container criado via `docker.sock` usam caminhos **do host**, não do container que pediu.

## Goals / Non-Goals

**Goals:**
- O banco é a fonte da verdade do run. A fila só transporta o "pode começar", e qualquer falha de worker ou broker se recupera a partir do banco.
- A api continua sem segredos e sem acesso ao Docker.
- Os contratos `Agent` e `Runner` ficam prontos para o runner `k8s` e para a change de entrega sem mudar as regras de run.

**Non-Goals:**
- Vários workers coordenados. O desenho tolera mais de um, mas só um é testado.
- Limites de CPU e memória por run, e acompanhamento ao vivo dos eventos (`follow`). O `GET /runs/{id}/logs` lê o arquivo do jeito que ele estiver.
- Limpar `./runs` e reaproveitar clones entre runs.

## Decisions

### 1. Pacotes

```
internal/execution/            Agent (+ Command, Credentials), Runner, Job, Catalog
  claude/                      comando do CLI e prioridade de credenciais
  host/                        Runner "host" sobre o SDK do Docker Engine
  run/                         Run, Status, Service, Repository, Guard (importa task)
    postgres/                  repositório com sqlc (queries/ + sqlcdb/)
    runtest/                   repositório em memória (sobre tasktest)
    worker/                    consumer, slots, execução, timeout, cancelamento, recuperação
internal/task/httpapi/         + handlers de /tasks/{id}/runs e /runs/...
cmd/worker/                    composição do worker
build/agent/                   Dockerfile da imagem default + entrypoint miner-job
```

O `run` importa `task` e `execution`. O `task` não importa o `run`: ele recebe uma interface-porta (decisão 4). Os handlers ficam em `task/httpapi` para reaproveitar `writeDomainError`, `decodeJSON` e `pathID`, e porque as rotas são aninhadas em `/tasks`.

*Alternativa:* pacote `run/httpapi` próprio. Duplicaria os helpers de erro e JSON, ou forçaria extraí-los agora.

### 2. Contratos `Agent` e `Runner`

```go
// esboço, não implementação final
type Invocation struct{ Prompt, Model string; Effort *string }
type Agent interface {
    Name() string
    Models() []Model
    Command(Invocation) []string                   // argv; o prompt vai por stdin
    Credential(getenv func(string) string) (Credential, bool)
}
type Credential struct{ Method, Env, Value string } // ex.: "oauth", "CLAUDE_CODE_OAUTH_TOKEN"

type Job struct {
    RunID  uuid.UUID
    Dir    string            // diretório do run no host
    Image  string
    Env    map[string]string
}
type Runner interface {
    Name() string
    Run(ctx context.Context, j Job) (exitCode int, err error) // ctx cancelado = kill
    Cleanup(ctx context.Context) error                        // remove órfãos por label
}
```

- O `claude` monta `claude -p --model <m> [--effort <e>] --output-format stream-json --verbose --dangerously-skip-permissions`, com o prompt pelo stdin, para evitar limite de tamanho e escaping de argumentos. A prioridade de credenciais é `CLAUDE_CODE_OAUTH_TOKEN` e depois `ANTHROPIC_API_KEY`.
- O catálogo continua com runners como nomes. O worker recebe um `map[string]Runner`, e `main` verifica na subida que cada nome do catálogo tem uma implementação.

*Alternativa:* deixar a escolha da credencial no worker. Isso espalharia nomes de variáveis do Claude fora do adapter.

### 3. Entidade Run e banco

A tabela `runs` (migração `00003`) guarda:
- o snapshot (`agent`, `model`, `effort`, `runner`, `repo_url`, `base_branch`, `branch`)
- `status` com `CHECK`, `auth_method`, `effective_model`, `cost_usd numeric`, `num_turns`, `exit_code`, `error` e `diff`
- `cancel_requested_at`, `created_at`, `started_at` e `finished_at`

Restrições e índices:
- `task_id` referencia `tasks` com `ON DELETE CASCADE`. O `DELETE` de uma task com run ativo é barrado antes pelo service.
- **Índice único parcial** `runs(task_id) WHERE status IN ('queued','running')`. Ele garante no banco a regra de um run ativo por task e vira `ErrConflict`.
- Índice `(task_id, created_at DESC)` para a listagem.

As transições do run e os efeitos na task acontecem **na mesma transação**, no repositório do run, com SQL condicional sobre `tasks`:

| Operação | SQL na transação |
|---|---|
| `Start` | lock da task, `INSERT run` e `UPDATE tasks SET status='in_progress' WHERE id=$1 AND status='ready'` (0 linhas → `ErrInvalidTransition`) |
| `MarkRunning` | `UPDATE runs SET status='running', started_at, auth_method WHERE id=$1 AND status='queued'` (0 linhas → descartar a mensagem) |
| `Finish(status, result)` | `UPDATE runs ... WHERE status IN (queued, running)` e `UPDATE tasks SET status=<alvo> WHERE id AND status='in_progress'` |
| `Cancel` de um run `queued` | é um `Finish(cancelled)` direto |
| `Cancel` de um run `running` | só `cancel_requested_at = now()`; quem finaliza é o worker |

O mapa run → task (`succeeded → in_review`, `failed → failed`, `cancelled → ready`) fica no pacote `run`. Os três destinos são transições válidas a partir de `in_progress` no `task` (`in_progress → ready` é nova nesta change). Um teste garante que o mapa só produz transições válidas.

O `runtest` implementa as mesmas operações sobre um `tasktest.Repository`, usando o `UpdateTask` dele para o efeito na task. É outro mutex, então não há deadlock.

*Alternativa:* o `run.Service` chamar o `task.Service.Update` depois de gravar o run. Isso perde a atomicidade, porque um run pode ficar `running` com a task em `ready`, e passaria pela regra de "status bloqueado com run ativo", que o próprio run precisa contornar.

O sqlc ganha uma segunda entrada em `sqlc.yaml` para `internal/execution/run/postgres/queries`, com o mesmo schema e os mesmos overrides.

### 4. A porta `task.RunGuard`

```go
// em internal/task
type RunGuard interface {
    HasActiveRun(ctx context.Context, taskID uuid.UUID) (bool, error)
    CancelActiveRun(ctx context.Context, taskID uuid.UUID) error
}
```

- `task.NewService(repo, catalog, runs RunGuard)`, implementado por `run.Guard`, que só depende do `run.Repository`. A composição fica sem ciclo: `runRepo → run.Guard → task.NewService → run.NewService(runRepo, taskSvc, ...)`.
- `Service.Update` com `Status` diferente de `cancelled`: se `HasActiveRun`, retorna `ErrConflict` **antes** do `UpdateTask`. A mutação não consulta nada.
- Depois de um `Update` bem-sucedido para `cancelled`, ele chama `CancelActiveRun`. Um run `queued` é finalizado, e um `running` recebe `cancel_requested_at`. O `Finish` posterior não mexe na task, que já não está em `in_progress`.
- `Service.Delete`: se `HasActiveRun`, retorna `ErrConflict`.
- `tasktest.NoRuns()` é um guard que sempre responde "sem run", para os testes que não envolvem runs.

*Alternativa:* checar o run ativo dentro da mutação. Isso a obrigaria a consultar outro repositório.

### 5. Disparo: `run.Service.Start`

1. `GetTask` e `task.Service.CheckRunnable(t)`. Esse é um método novo que expõe `validateExecution` e `checkReady`, e uma falha é `ValidationError` (`422`).
2. Monta o snapshot e a branch `miner/<task-id>/<run-id>`.
3. `repo.Start` (decisão 3).
4. Publica `{run_id}` no exchange `tasks.runs` com publisher confirm. Se a publicação falhar, `repo.Abort` apaga o run e devolve a task para `ready` na mesma transação, e a resposta é `503`.

### 6. Fila e worker

- **Topologia:**
  - exchange direct `tasks.runs`
  - quorum queue `tasks.runs` com `x-delivery-limit` 4 e DLX `tasks.runs.dlx`
  - fanout `tasks.runs.dlx` com a fila `tasks.runs.dlq`
  - A api e o worker declaram as duas topologias, ingestão e runs, compondo as `SetupFunc` numa só.
- **Handler sequencial com slots.** Um semáforo de `MAX_CONCURRENT_RUNS` controla a execução:
  1. Para cada mensagem, o handler espera um slot livre e só então chama `MarkRunning`.
  2. Feito isso, dá **ack imediato** e roda o run numa goroutine, que libera o slot no fim.
  3. Com o handler bloqueado esperando slot, as próximas mensagens ficam na fila, na ordem de chegada (`prefetch` = 1).
  4. Um erro de infraestrutura antes do `MarkRunning` vira nack com requeue (delivery limit e depois DLQ). Um run inexistente ou que já saiu de `queued` vira ack e descarte.
- **Ack cedo:** ele resolve o `consumer_timeout`. Daí em diante, quem garante o run é o banco: recuperação na subida (decisão 9) e cancelamento por polling (decisão 8).

*Alternativa:* segurar o ack até o fim do run. Isso estoura o `consumer_timeout` e reentrega um run longo que já está rodando.

### 7. Execução de um run (worker + runner `host` + entrypoint)

```
worker                                    container miner-agent:default
------                                    ------------------------------
credencial = agent.Credential(env)        miner-job:
  sem credencial -> Finish(failed)          1. GIT_ASKPASS lê GIT_TOKEN; git clone
imagem existe? senão -> Finish(failed)         --branch $BASE_BRANCH $REPO_URL workspace
escreve runs/<id>/prompt.md, git-token   2. apaga git-token; git switch -c $BRANCH
        runs/<id>/command.json             3. agente < prompt.md > events.jsonl
Runner.Run(ctx com RUN_TIMEOUT)            4. git add -A; sem mudanças -> exit 11
  -> exit code                                 git commit -m "$COMMIT_MESSAGE"
lê events.jsonl (init, result) e diff      5. git diff base..HEAD > diff.patch
Finish(succeeded | failed)                 exit 0 | 10 clone | 11 sem mudanças | 12 agente
```

- **Snapshot:** o run guarda o próprio `base_branch`, então o `diff` é relativo ao commit base do clone.
- **Códigos de saída:** cada código vira um `error` da spec: `10` falha no clone, `11` `agent made no changes`, `12` o agente terminou com erro (com o exit code do agente em `agent_exit`), qualquer outro é falha inesperada. O ctx cancelado por timeout vira `run timed out`.
- **Resumo do agente:** sai dos eventos. O evento `system/init` dá o `model`, e o `result` dá `total_cost_usd` e `num_turns`. Um formato desconhecido deixa os campos `null`, sem falhar o run.
- **Diff:** é guardado até 1 MiB. Acima disso, é cortado com um marcador `... diff truncated`, e o completo continua no workspace.
- **Container:**
  - label `miner.run=<id>`
  - usuário 1000, `--cap-drop ALL`, `no-new-privileges`, sem privileged e sem montar `docker.sock`
  - só o diretório do run montado
  - rede bridge própria `miner-runs`, criada pelo worker se não existir, fora da rede do compose
- **Credenciais:** a do agente entra como variável de ambiente do container. O `GIT_TOKEN` **não** entra no ambiente, porque o agente roda com o mesmo uid do processo 1 e leria `/proc/1/environ`. O worker grava o token em `<run>/git-token` (modo 600), o `GIT_ASKPASS` o lê durante o clone, e o `miner-job` apaga o arquivo antes de iniciar o agente. O worker apaga de novo ao fim do run. O remote fica com a URL sem token.
- **Caminho do host:** na subida, o worker inspeciona o próprio container (o hostname é o id) e lê o `Source` do mount de `/runs`. `RUNS_HOST_DIR` sobrescreve essa descoberta, para rodar o worker fora do Docker.
- **Commit:** autor `miner <miner@localhost>`, via `GIT_AUTHOR_*` e `GIT_COMMITTER_*`.

*Alternativa:* chamar o binário `docker` em vez do SDK. É menos dependência Go, mas exige o CLI na imagem do worker e parsing de texto para códigos de saída e inspeção. O SDK oficial do Docker Engine fica só em `internal/execution/host`.

### 8. Cancelamento e timeout

- Enquanto o run roda, uma goroutine consulta `cancel_requested_at` a cada 2 s. Quando ele aparece, ela cancela o ctx do `Runner.Run` (o `ContainerKill`) e o run termina com `Finish(cancelled)`.
- O timeout é o mesmo ctx com deadline e termina com `Finish(failed, "run timed out")`.
- Na parada do worker (SIGTERM), os runs em andamento são encerrados e finalizados como `failed` com `worker stopped`, dentro do `SHUTDOWN_TIMEOUT`.

*Alternativa:* um exchange fanout de controle com uma fila exclusiva por worker. O nome muda a cada reconexão, a mensagem se perde com o worker fora do ar, e o polling custa uma consulta indexada a cada 2 s por run, o que é desprezível.

### 9. Recuperação na subida

Antes de consumir, o worker faz duas coisas:
1. `Runner.Cleanup` remove os containers com label `miner.run`.
2. Cada run em `running` vira `Finish(failed, "worker restarted")`, ou `cancelled` se tiver `cancel_requested_at`.

Os runs em `queued` ficam como estão, porque a mensagem deles ainda está na fila ou na DLQ. O caso da DLQ está em Risks.

### 10. Serviço `worker` no compose

- **Build:** mesma imagem `miner-api:dev`, com `air` usando um `.air.worker.toml` que compila `./cmd/worker`.
- **Volumes:** `/var/run/docker.sock` e `./runs:/runs`.
- **Ambiente:** `DATABASE_URL`, `RABBITMQ_URL`, `MAX_CONCURRENT_RUNS`, `RUN_TIMEOUT`, `AGENT_IMAGE` (padrão `miner-agent:default`), `CLAUDE_CODE_OAUTH_TOKEN`, `ANTHROPIC_API_KEY` e `GIT_TOKEN`. Só o `worker` recebe essas três últimas variáveis no compose.
- **api:** ganha `./runs:/runs:ro` e `RUNS_DIR=/runs`, para servir `GET /runs/{id}/logs`.
- **Configuração:** `config.LoadWorker` lê o ambiente do worker.
- **Makefile:** `make agent-image` faz `docker build -t miner-agent:default build/agent`, e `make up` depende dele.
- **`.gitignore`:** ganha `/runs/`.

Isso vira ADR-0014 (worker com `docker.sock` e o SDK do Docker) e ADR-0015 (Run como fonte da verdade, fila `tasks.runs` com ack cedo e cancelamento por polling).

### 11. Imagem `miner-agent:default`

`build/agent/Dockerfile`:
- base `node:24-slim`, mais `git`, `ca-certificates`, `curl` e `jq`
- `npm i -g @anthropic-ai/claude-code@${CLAUDE_CODE_VERSION}`, com `ARG` fixado no arquivo
- usuário `node` (uid 1000) e `miner-job` em `/usr/local/bin`

O `HOME` fica dentro do diretório do job, para o CLI não precisar de estado persistente.

## Risks / Trade-offs

- **[`docker.sock` = root no host]** → O socket só existe no `worker`, a api não o recebe, e o job não o monta. Aceito para uso local individual e registrado no ADR-0014.
- **[Agente sem permissões pedidas]** → O container é a fronteira: sem privilégios, sem socket, sem o token do git no ambiente e só com o diretório do run montado. Mesmo assim, o agente pode fazer o que quiser dentro do workspace e na internet.
- **[Portas publicadas no host]** → Um container na rede bridge alcança o gateway do host e as portas publicadas pelo compose (Postgres na 5433, RabbitMQ na 5672). Aceito e documentado. A mitigação futura é não publicar essas portas ou usar o runner `k8s` com NetworkPolicy.
- **[Run `queued` preso na DLQ]** → Se a mensagem morrer, o run fica `queued` e bloqueia a task. `POST /runs/{id}/cancel` funciona para `queued` sem passar pela fila, e o README explica isso.
- **[Corrida no status com run ativo]** → Entre o `HasActiveRun` e o `UpdateTask`, o run pode terminar. No pior caso, um `PATCH` de status é recusado com `409` sem necessidade e pode ser repetido. O inverso (o run terminar sobre um status já mudado) é tratado pelo `UPDATE ... WHERE status='in_progress'`.
- **[Formato do stream-json]** → Os campos do resumo dependem do CLI. A versão é fixada na imagem, e o parse é tolerante.
- **[Disco]** → `./runs` cresce sem limpeza automática e é limpo à mão. Uma limpeza fica para depois.
- **[Arquivos do workspace com uid 1000]** → Num host cujo usuário não seja 1000, os arquivos podem não ser editáveis sem `sudo`. Aceito, porque a revisão é só leitura.

## Migration Plan

1. Migração `00003_create_runs.sql`, aditiva, com o `Down` removendo a tabela. Rode `make sqlc` com a nova entrada em `sqlc.yaml`.
2. `make agent-image`, ou `make up`, que já depende dele.
3. Tasks em `in_progress` de antes desta change não têm run. A nova transição `in_progress → ready` permite liberá-las à mão.

Rollback: `make migrate-down` remove os runs, e é só reverter o código. Os workspaces em `./runs` continuam no disco.

## Open Questions

- A versão exata do `@anthropic-ai/claude-code` a fixar no `ARG`. Escolhemos na implementação a mais recente estável que aceite `--effort` e `stream-json`.
- O caminho de import do SDK do Docker Engine (`github.com/docker/docker/client` ou o módulo novo do moby client). Fica a versão estável mais recente compatível com a API do Docker local.
