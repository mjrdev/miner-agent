# Tasks

## 1. Contratos de execução

- [x] 1.1 Adicionar a `internal/execution` os tipos `Invocation` e `Credential`, os métodos `Command` e `Credential` na interface `Agent`, e os tipos `Job` e `Runner` (`Name`, `Run`, `Cleanup`). Ajustar o agente fake dos testes. Verificar com `go build ./...` e `go list -deps ./internal/execution` sem `internal/task`
- [x] 1.2 Implementar em `internal/execution/claude` o `Command` (`-p`, `--model`, `--effort` só quando definido, `--output-format stream-json`, `--verbose`, `--dangerously-skip-permissions`) e o `Credential` com prioridade `CLAUDE_CODE_OAUTH_TOKEN` > `ANTHROPIC_API_KEY`. Verificar com testes de tabela: argv com e sem esforço, só OAuth, só a chave de API, as duas (vence `oauth`) e nenhuma (`false`)

## 2. Banco e entidade Run

- [x] 2.1 Criar `migrations/00003_create_runs.sql` com a tabela `runs` (colunas do design, `CHECK` de status, FK `ON DELETE CASCADE`, índice único parcial de run ativo por task e índice `(task_id, created_at DESC)`), com `Down`. Verificar com `make migrate`, `make migrate-down`, `make migrate` e `\d runs` no `make psql`
- [x] 2.2 Criar `internal/execution/run` com `Run`, `Status`, as transições do run, o mapa run → status da task e a interface `Repository` (`Start`, `Abort`, `Get`, `ListByTask`, `HasActive`, `MarkRunning`, `RequestCancel`, `Finish`, `ListRunning`). Verificar com testes de unidade da tabela de transições do run e de que o mapa só produz transições válidas de `task.StatusInProgress`
- [x] 2.3 Criar as queries em `internal/execution/run/postgres/queries/`, a segunda entrada em `sqlc.yaml`, rodar `make sqlc` e implementar o repositório Postgres com `Start`/`Finish`/`Abort` transacionais e SQL condicional sobre `tasks`. Verificar com testes de integração (`make test-integration`): `Start` move a task para `in_progress`, um segundo `Start` vira `ErrConflict`, `Start` numa task fora de `ready` vira `ErrInvalidTransition`, `Finish` respeita o mapa e não mexe numa task que já saiu de `in_progress`, e `Abort` devolve a task para `ready`
- [x] 2.4 Criar `internal/execution/run/runtest` com o repositório em memória sobre `tasktest.Repository`, com as mesmas regras. Verificar rodando com ele os mesmos cenários da 2.3 num teste de unidade

## 3. Regras de task com runs

- [x] 3.1 Adicionar `in_progress → ready` às transições de `internal/task` e atualizar `TestStatusTransitions`. Verificar com `go test ./internal/task -run TestStatusTransitions`
- [x] 3.2 Adicionar a porta `task.RunGuard`, mudar para `NewService(repo, catalog, runs)` e criar `tasktest.NoRuns()`. Ajustar todos os chamadores (`cmd/api/main.go` e os testes de `task`, `httpapi`, `postgres` e `ingest`). Verificar com `go build ./...` e `make test`
- [x] 3.3 Aplicar o guard em `task.Service`: `Update` com status diferente de `cancelled` e run ativo → `ErrConflict`; `Update` para `cancelled` chama `CancelActiveRun`; `Delete` com run ativo → `ErrConflict`. Expor `CheckRunnable(Task) error`. Verificar com testes em `service_test.go` com um guard fake, cobrindo os cenários de `Status while a run is active`, `Cancel a task with an active run` e `Delete a task with an active run`
- [x] 3.4 Implementar `run.Guard` sobre o `run.Repository` (`HasActive`; cancelamento que finaliza um run `queued` e marca `cancel_requested_at` num `running`). Verificar com testes de unidade usando o `runtest`

## 4. Serviço de runs e topologia

- [x] 4.1 Criar a topologia de `tasks.runs` (exchange direct, quorum queue com delivery limit 4, DLX `tasks.runs.dlx` e DLQ `tasks.runs.dlq`) e compor com a da ingestão numa única `SetupFunc` usada pela api e pelo worker. Verificar com um teste de integração que declara as duas topologias e confere que uma mensagem morta de runs vai só para `tasks.runs.dlq`
- [x] 4.2 Implementar `run.Service`: `Start` (revalida com `CheckRunnable`, monta o snapshot e a branch `miner/<task-id>/<run-id>`, `repo.Start`, publica com confirm e faz `Abort` + `503` se falhar), `Get`, `ListByTask` e `Cancel` (finaliza um `queued`, marca um `running`, `409 invalid_transition` num run terminado). Também o prompt (título, descrição, instruções adicionais em ordem e a instrução de não fazer commit nem push). Verificar com testes em `service_test.go` do pacote `run` (com `runtest` e publisher fake) que cubram os cenários de `Start a run`, `Revalidate before starting`, `One active run per task`, `Queue unavailable on start`, `Cancel a run` e `Agent prompt`

## 5. Imagem e runner host

- [x] 5.1 Criar `build/agent/Dockerfile` (`node:24-slim`, `git`, `ca-certificates`, `curl`, `jq`, `@anthropic-ai/claude-code` com versão fixa em `ARG`, usuário `node`) e o entrypoint `miner-job` (clone com `GIT_ASKPASS`, `unset GIT_TOKEN`, branch, agente lendo o prompt pelo stdin e gravando `events.jsonl`, commit, `diff.patch` e os códigos de saída 0/10/11/12). Adicionar `make agent-image` e fazer `make up` depender dele. Verificar com `make agent-image`, `docker run --rm miner-agent:default claude --version` mostrando a versão fixada, e um teste de shell que roda `miner-job` contra um repositório local (`file://`) com um agente fake: exit 0 com commit, exit 11 sem mudanças, e `git remote -v` sem token
- [x] 5.2 Implementar `internal/execution/host` com o SDK do Docker Engine: `Run` (imagem inexistente vira erro com o comando `make agent-image`; container com label `miner.run`, uid 1000, `cap-drop ALL`, `no-new-privileges`, rede `miner-runs`, só o diretório do run montado; kill no cancelamento do ctx; retorna o exit code), `Cleanup` por label e a descoberta do caminho de `/runs` no host (inspeção do próprio container, com `RUNS_HOST_DIR` sobrescrevendo). Verificar com um teste de integração (pulado sem `docker.sock`) que roda um container `alpine` com exit 3 e outro cancelado pelo ctx, e confere que `Cleanup` remove os containers com o label

## 6. Worker

- [x] 6.1 Implementar `internal/execution/run/worker`: handler com semáforo de `MAX_CONCURRENT_RUNS` (espera um slot, `MarkRunning`, ack, goroutine), nack com requeue em erro de infraestrutura, descarte de run inexistente ou que saiu de `queued`, escolha da credencial (sem credencial → `failed` sem container), escrita de `job/prompt.md` e `job/command.json`, chamada ao `Runner`, interpretação dos códigos de saída, leitura do resumo de `events.jsonl` (tolerante a formato) e do diff com limite de 1 MiB, e `Finish`. Verificar com testes de unidade com `runtest` e um `Runner` fake: sucesso → `succeeded` com diff e resumo, exit 11 → `agent made no changes`, sem credencial, imagem ausente, e dois runs com limite 1 rodando em sequência
- [x] 6.2 Adicionar ao worker o polling de `cancel_requested_at` a cada 2 s, o timeout por `RUN_TIMEOUT` (`run timed out`), a recuperação na subida (`Cleanup` + `running` → `failed` `worker restarted`, ou `cancelled` com pedido) e a parada graciosa (`worker stopped`). Verificar com testes de unidade com relógio e `Runner` fake para cancelamento, timeout, recuperação e parada
- [x] 6.3 Criar `config.LoadWorker` (`DATABASE_URL`, `RABBITMQ_URL`, `MAX_CONCURRENT_RUNS`, `RUN_TIMEOUT`, `AGENT_IMAGE`, `RUNS_DIR`, `RUNS_HOST_DIR`, `LOG_LEVEL`, `SHUTDOWN_TIMEOUT`) e `cmd/worker/main.go` (config → logger → pool → broker → catálogo e runners → recuperação → consumer → espera de sinal). Na subida, ele verifica que cada runner do catálogo tem implementação e avisa quando não há credenciais. Verificar com testes de `config.LoadWorker` (padrões e valores inválidos) e com `go build ./cmd/worker`

## 7. HTTP

- [x] 7.1 Adicionar a `internal/task/httpapi` os handlers `POST /tasks/{id}/runs`, `GET /tasks/{id}/runs`, `GET /runs/{id}`, `GET /runs/{id}/logs` (NDJSON a partir de `RUNS_DIR`, vazio quando não há arquivo) e `POST /runs/{id}/cancel`, com a `runResponse` da spec (sem `diff` na listagem). Montar em `cmd/api/main.go` o `run.Guard`, o `run.Service` e o publisher. Verificar com testes `httptest` (service real com `runtest` e publisher fake) de cada cenário de `task-runs` exposto por HTTP, incluindo `404`, `409`, `422`, `503` e `Content-Type: application/x-ndjson`
- [x] 7.2 Ajustar os testes de `httpapi` de `PATCH`/`DELETE` de task para os cenários com run ativo (`409`) e de `in_progress → ready` sem run ativo. Verificar com `make test`

## 8. Ambiente local e documentação

- [x] 8.1 Adicionar ao `docker-compose.yml` o serviço `worker` (`air` com `.air.worker.toml`, `docker.sock`, `./runs:/runs`, credenciais e `GIT_TOKEN` só nele) e à api `./runs:/runs:ro` e `RUNS_DIR`. Também `/runs/` no `.gitignore` e as variáveis novas sem valor no `.env.example`. Verificar com `make up`, `docker compose ps` mostrando `worker`, `docker compose exec api env` sem as credenciais e o log do worker avisando da falta de credenciais
- [x] 8.2 Registrar com a skill `adr` o ADR-0014 (worker separado com `docker.sock` e SDK do Docker, risco aceito e portas publicadas) e o ADR-0015 (Run como fonte da verdade, fila `tasks.runs` com ack cedo, cancelamento por polling e recuperação). Verificar que `docs/adr/README.md` os lista
- [x] 8.3 Atualizar o `README.md` (credenciais no `.env`, `make agent-image`, exemplos `curl` de disparo, consulta, logs e cancelamento, `./runs/<id>/`, como destravar um run `queued` preso e o `409` com run ativo) e o `CLAUDE.md` (pacotes `run`, `host` e `worker`, a porta `RunGuard`, o ack cedo e o caminho do host via inspeção). Verificar executando os exemplos documentados contra `make up`

## 9. Verificação integrada

- [x] 9.1 Com `make up` e uma credencial real no `.env`, disparar uma task `local` configurada com `claude`/`sonnet`/`low` e uma categoria apontando para um repositório público pequeno. Verificar que o run passa por `queued` → `running` → `succeeded`, a task chega a `in_review`, `./runs/<id>/workspace` tem o commit na branch `miner/<task-id>/<run-id>`, `GET /runs/{id}` traz `diff`, `effective_model` e `cost_usd`, e `GET /runs/{id}/logs` devolve os eventos. Repetir com um `POST /runs/{id}/cancel` durante a execução e conferir `cancelled`, a task em `ready` e nenhum container sobrando
- [x] 9.2 Rodar `make fmt`, `make lint`, `make test`, `make test-integration` e `openspec validate add-host-task-runner --strict` sem erros

## Workflow follow-up

- Arquivar a change com `/opsx:archive`, que segue o `spec-git-flow` (commits, push e PR).
- Explorar e propor a change `add-run-delivery` (push da branch do run, abertura do PR e link do PR na task em `in_review`, reaproveitando `GIT_TOKEN`).
