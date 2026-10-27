# Proposal

## Why

Com a change `add-task-execution-config`, uma task em `ready` já diz com qual agente, modelo, esforço e runner deve rodar e em qual repositório. Mas nada a executa: ela fica parada em `ready`. Esta change cria a execução, em que cada disparo sobe um container descartável no host, clona o projeto, roda o agente com a task e entrega um commit numa branch para revisão.

É a primeira de duas changes de execução. O push e o PR ficam para a próxima (`add-run-delivery`), que reaproveita o workspace e o token desta.

## What Changes

- **Entidade Run:** cada execução de uma task é um run, com status (`queued`, `running`, `succeeded`, `failed`, `cancelled`), um snapshot da configuração usada (agente, modelo, esforço, runner, repositório, branch base e branch criada), o método de autenticação, o resumo do agente (modelo efetivo, custo, turnos), o diff, o erro e as datas.
- **Disparo manual:** `POST /tasks/{id}/runs` exige a task em `ready`, revalida o catálogo e o `repo_url`, cria o run, move a task para `in_progress` e publica o run na fila `tasks.runs`. Um segundo disparo com um run ativo resulta em `409`.
- **Consulta e controle:** `GET /tasks/{id}/runs`, `GET /runs/{id}`, `GET /runs/{id}/logs` (eventos do agente) e `POST /runs/{id}/cancel`.
- **Worker:** um serviço novo, `worker`, separado da API. É o único com acesso ao Docker do host. Ele consome a fila, executa até `MAX_CONCURRENT_RUNS` runs ao mesmo tempo (padrão 1) e encerra um run que passar de `RUN_TIMEOUT` (padrão 60 minutos). Ao subir, marca como `failed` os runs que ficaram órfãos.
- **Runner `host`:** cria o container a partir da imagem default `miner-agent:default` (`node:24-slim`, `git` e o Claude CLI com versão fixa, usuário sem root). No container, o job clona o repositório na branch base, cria `miner/<task-id>/<run-id>`, roda o agente sem interação e faz o commit das mudanças. Um run sem mudanças falha.
- **Agente:** o contrato `Agent` ganha a montagem do comando a partir do prompt, do modelo e do esforço, e a escolha da credencial. O `claude` usa `CLAUDE_CODE_OAUTH_TOKEN` e, na falta dele, `ANTHROPIC_API_KEY`. Só uma credencial entra no container, e nenhuma é gravada no banco ou nos logs.
- **Repositórios privados:** um `GIT_TOKEN` opcional no worker é usado só no clone.
- **Status da task:** um run bem-sucedido leva a task para `in_review`, e uma falha ou timeout para `failed`. Cancelar o run devolve a task para `ready`, e cancelar a task com run ativo encerra o container. **BREAKING (comportamento):** enquanto houver um run ativo, o `PATCH` de `status` só aceita `cancelled`, e `in_progress → ready` passa a ser uma transição válida (usada pelo cancelamento do run e para recuperar uma task sem run ativo).
- **Workspace e logs:** ficam no host em `./runs/<run-id>/` (o clone com o commit e `events.jsonl`). A API lê esse diretório só para leitura.
- **Migração** `00003` com a tabela `runs`, o código do sqlc regenerado e dois ADRs: o worker com acesso ao Docker e a entidade Run com a fila de execução.

Fora de escopo: push, PR e a ida da revisão para o provider (próxima change). Também ficam fora a imagem e o ambiente por projeto, o runner `k8s`, o disparo automático ao entrar em `ready`, a limpeza automática de `./runs` e a interface web.

## Capabilities

### New Capabilities
- `task-runs`: execução das tasks. Cobre a entidade Run, os endpoints de disparo, consulta, logs e cancelamento, o worker com fila, concorrência, timeout e recuperação, o runner `host` com a imagem default e o job (clone, agente, commit e diff), além das credenciais.

### Modified Capabilities
- `task-management`: as transições de status passam a ser dirigidas pelos runs (`in_progress → ready` no cancelamento do run, status bloqueado enquanto houver run ativo) e o cancelamento da task encerra o run ativo.
- `execution-catalog`: o agente passa a montar o comando de execução e a escolher a credencial.
- `local-dev-environment`: o compose ganha o serviço `worker` e o diretório `./runs`, e o `.env` ganha as credenciais opcionais.

## Impact

- **Código novo:**
  - `internal/execution/run/` (entidade Run, regras, service e repositório)
  - `internal/execution/run/postgres`, `internal/execution/run/runtest` e o worker (`internal/execution/run/worker`)
  - `internal/execution/host` (runner Docker)
  - `cmd/worker`
  - `build/agent/` (Dockerfile e entrypoint do job)
- **Código alterado:**
  - `internal/execution` (contratos `Agent` e `Runner`)
  - `internal/execution/claude`
  - `internal/task` (transições, regra de status com run ativo e cancelamento)
  - `internal/task/httpapi` (endpoints de runs)
  - `internal/config`, `cmd/api/main.go`, `docker-compose.yml`, `Makefile` e `.env.example`
- **Dependências:** o SDK do Docker Engine para Go (`github.com/docker/docker/client`).
- **Banco:** `migrations/00003_create_runs.sql`, aditiva e com `Down`.
- **Mensageria:** exchange `tasks.runs` e quorum queue `tasks.runs`, com DLX e DLQ próprios. O cancelamento passa pelo banco, não pela fila.
- **Segurança:** o worker monta `/var/run/docker.sock`, o que equivale a root no host, e o agente roda com `--dangerously-skip-permissions` dentro de um container sem privilégios. O risco é aceito por ser um serviço local de uso individual e fica registrado em ADR.
- **APIs:** os cinco endpoints novos e a mudança de comportamento no `PATCH` e no `DELETE /tasks/{id}` com run ativo.
