# task-runs Specification

## Purpose

Executa as tasks com um agente de IA num ambiente isolado. Cada run clona o repositório do projeto, roda o agente com a task e guarda o resultado (commit, diff e eventos) para revisão.

## Requirements

### Requirement: Run representation
As respostas de run SHALL ser JSON com:
- `id` (UUID), `task_id` e `status`
- o snapshot `agent`, `model`, `effort`, `runner`, `repo_url`, `base_branch` e `branch`
- `auth_method` (`oauth`, `api_key` ou `null`), `effective_model`, `cost_usd`, `num_turns`, `exit_code` e `error` (ou `null`)
- `created_at`, `started_at` e `finished_at` (RFC 3339 ou `null`)

O diff SHALL ficar fora da listagem e aparecer só na leitura individual.

#### Scenario: Leitura de um run
- **WHEN** um cliente envia `GET /runs/{id}` para um run existente
- **THEN** o serviço responde `200 OK` com todos os campos acima e o campo `diff` (texto ou `null`)

#### Scenario: Run inexistente
- **WHEN** um cliente envia `GET /runs/{id}` com um id que não existe ou que não é UUID
- **THEN** o serviço responde `404 Not Found`

### Requirement: Run status lifecycle
`status` SHALL ser `queued`, `running`, `succeeded`, `failed` ou `cancelled`. As únicas transições SHALL ser `queued → running`, `queued → failed`, `queued → cancelled`, `running → succeeded`, `running → failed` e `running → cancelled`. `succeeded`, `failed` e `cancelled` são terminais. Um run em `queued` ou `running` é um run ativo.

#### Scenario: Run terminado não muda
- **WHEN** um run está em `succeeded` e um cliente envia `POST /runs/{id}/cancel`
- **THEN** o serviço responde `409 Conflict` com `error.code` igual a `invalid_transition` e o run continua `succeeded`

### Requirement: Start a run
`POST /tasks/{id}/runs` SHALL criar um run `queued` para uma task em `ready`, responder `202 Accepted` com o run e mover a task para `in_progress` atomicamente. O snapshot SHALL registrar a configuração da task e o repositório e a branch base da categoria no momento do pedido. A branch criada SHALL ser `miner/<task-id>/<run-id>`.

#### Scenario: Disparo de uma task pronta
- **WHEN** uma task em `ready`, com `agent` `claude`, `model` `opus`, `effort` `high`, `runner` `host` e categoria com `repo_url`, recebe `POST /tasks/{id}/runs`
- **THEN** o serviço responde `202 Accepted` com um run `queued` contendo esse snapshot, e a task passa a `in_progress`

#### Scenario: Task fora de ready
- **WHEN** um cliente envia `POST /tasks/{id}/runs` para uma task em `draft`
- **THEN** o serviço responde `409 Conflict` com `error.code` igual a `invalid_transition` e nenhum run é criado

#### Scenario: Task inexistente
- **WHEN** um cliente envia `POST /tasks/{id}/runs` para um id que não existe
- **THEN** o serviço responde `404 Not Found`

### Requirement: Revalidate before starting
Antes de criar o run, o serviço SHALL revalidar a configuração da task contra o catálogo e os pré-requisitos de `ready`, incluindo o `repo_url` da categoria. Uma falha SHALL resultar em `422` com `validation_failed`, citar o problema e não criar o run nem mudar a task.

#### Scenario: Modelo removido do catálogo
- **WHEN** uma task em `ready` usa um modelo que não existe mais no catálogo e recebe `POST /tasks/{id}/runs`
- **THEN** o serviço responde `422 Unprocessable Entity` e a task continua em `ready`

### Requirement: One active run per task
Uma task SHALL ter no máximo um run ativo. Um pedido de disparo com um run ativo na task SHALL resultar em `409` com `conflict`, sem criar outro run.

#### Scenario: Disparo duplicado
- **WHEN** uma task tem um run em `queued` e um cliente envia `POST /tasks/{id}/runs`
- **THEN** o serviço responde `409 Conflict` com `error.code` igual a `conflict`

### Requirement: Queue unavailable on start
Se o run não puder ser enfileirado, o serviço SHALL responder `503` com `unavailable`, não manter o run e deixar a task em `ready`.

#### Scenario: RabbitMQ fora do ar
- **WHEN** o RabbitMQ está indisponível e um cliente envia `POST /tasks/{id}/runs` para uma task em `ready`
- **THEN** o serviço responde `503 Service Unavailable`, nenhum run fica registrado e a task continua em `ready`

### Requirement: List runs of a task
`GET /tasks/{id}/runs` SHALL responder `200 OK` com `{"items":[...]}`, com os runs da task do mais recente para o mais antigo. Uma task inexistente SHALL resultar em `404`.

#### Scenario: Histórico de runs
- **WHEN** uma task teve dois runs e um cliente envia `GET /tasks/{id}/runs`
- **THEN** a resposta contém os dois runs, o mais recente primeiro, sem o campo `diff`

### Requirement: Run events
`GET /runs/{id}/logs` SHALL responder `200 OK` com `Content-Type: application/x-ndjson` e os eventos emitidos pelo agente durante o run, um JSON por linha, na ordem em que foram emitidos. Um run sem eventos SHALL responder com corpo vazio. Os eventos SHALL não conter as credenciais do agente nem o token do git.

#### Scenario: Eventos de um run terminado
- **WHEN** um run terminou e um cliente envia `GET /runs/{id}/logs`
- **THEN** o serviço responde `200 OK` com os eventos do agente em NDJSON

#### Scenario: Run ainda na fila
- **WHEN** um run está em `queued` e um cliente envia `GET /runs/{id}/logs`
- **THEN** o serviço responde `200 OK` com corpo vazio

### Requirement: Cancel a run
`POST /runs/{id}/cancel` SHALL cancelar um run ativo e responder `202 Accepted` com o run. Um run em `queued` SHALL ficar `cancelled` imediatamente. Num run em `running`, o ambiente de execução SHALL ser encerrado e o run SHALL ficar `cancelled`. Em ambos os casos, a task SHALL voltar para `ready`.

#### Scenario: Cancelar run na fila
- **WHEN** um run está em `queued` e um cliente envia `POST /runs/{id}/cancel`
- **THEN** o serviço responde `202 Accepted`, o run fica `cancelled`, nunca chega a rodar e a task volta para `ready`

#### Scenario: Cancelar run em execução
- **WHEN** um run está em `running` e um cliente envia `POST /runs/{id}/cancel`
- **THEN** o ambiente do run é encerrado, o run fica `cancelled` com `finished_at` preenchido e a task volta para `ready`

### Requirement: Run job
Ao executar um run, o serviço SHALL, num ambiente isolado e descartável:
1. clonar o `repo_url` do snapshot na `base_branch`
2. criar a branch do run
3. rodar o agente sem interação, com o modelo e o esforço do snapshot
4. fazer o commit de todas as mudanças com o título da task como mensagem

O agente SHALL ser instruído a não fazer commit nem push.

#### Scenario: Run com mudanças
- **WHEN** o agente termina com sucesso e alterou arquivos do repositório
- **THEN** o run fica `succeeded`, a branch do run tem um commit com essas mudanças sobre a branch base e `diff` contém essas mudanças

### Requirement: Agent prompt
O prompt entregue ao agente SHALL conter o título, a `description` e, em ordem, as `additional_instructions` da task, além da instrução de não fazer commit nem push.

#### Scenario: Instruções adicionais
- **WHEN** uma task tem `additional_instructions` `["passo 2","passo 1"]` e é executada
- **THEN** o prompt do run contém a descrição seguida de `passo 2` e depois de `passo 1`

### Requirement: Run outcome
Um run SHALL ficar `failed` com `error` descrevendo a causa quando o clone falhar, o agente terminar com erro ou o agente não alterar nenhum arquivo. `exit_code` SHALL registrar o código de saída do agente, quando houver. O workspace do run SHALL ser mantido no host nos dois casos.

#### Scenario: Agente sem mudanças
- **WHEN** o agente termina com sucesso sem alterar nenhum arquivo
- **THEN** o run fica `failed` com `error` igual a `agent made no changes`

#### Scenario: Repositório inacessível
- **WHEN** o `repo_url` do snapshot não pode ser clonado
- **THEN** o run fica `failed` com um `error` que cita o clone e sem expor o token do git

### Requirement: Agent summary
Quando o agente informar, o run SHALL registrar o modelo efetivamente usado (`effective_model`), o custo em dólares (`cost_usd`) e o número de turnos (`num_turns`).

#### Scenario: Resumo do claude
- **WHEN** um run com o agente `claude` e o modelo `opus` termina
- **THEN** `effective_model` contém o identificador completo do modelo que o CLI informou, e `cost_usd` e `num_turns` estão preenchidos

### Requirement: Task status from runs
O fim de um run SHALL mover a task, se ela ainda estiver em `in_progress`: `succeeded` → `in_review`, `failed` → `failed` e `cancelled` → `ready`. Se a task não estiver mais em `in_progress`, por exemplo porque foi cancelada, o fim do run SHALL não mudá-la.

#### Scenario: Sucesso vai para revisão
- **WHEN** um run termina `succeeded`
- **THEN** a task passa de `in_progress` para `in_review`

#### Scenario: Falha
- **WHEN** um run termina `failed`
- **THEN** a task passa de `in_progress` para `failed`

### Requirement: Run timeout
Um run em `running` por mais tempo que o limite configurado (padrão 60 minutos) SHALL ter o ambiente encerrado e ficar `failed` com `error` igual a `run timed out`.

#### Scenario: Run longo demais
- **WHEN** o limite é de 1 minuto e o agente continua rodando depois disso
- **THEN** o ambiente é encerrado, o run fica `failed` com `error` `run timed out` e a task passa a `failed`

### Requirement: Concurrency limit
O serviço SHALL executar no máximo o número configurado de runs ao mesmo tempo (padrão 1). Os demais SHALL esperar em `queued`, na ordem em que foram disparados.

#### Scenario: Dois disparos com limite 1
- **WHEN** o limite é 1 e duas tasks diferentes são disparadas em seguida
- **THEN** o primeiro run fica `running` e o segundo continua `queued` até o primeiro terminar

### Requirement: Agent credentials
O serviço SHALL autenticar o agente com a credencial configurada, seguindo a ordem de prioridade do agente, e SHALL entregar ao ambiente do run apenas uma credencial. O run SHALL registrar o método em `auth_method`, nunca o valor. Sem nenhuma credencial configurada, o run SHALL ficar `failed` com `error` igual a `no agent credentials configured`, sem criar o ambiente.

#### Scenario: Sem credencial
- **WHEN** nenhuma credencial do agente está configurada e um run é executado
- **THEN** o run fica `failed` com `error` `no agent credentials configured` e a task passa a `failed`

#### Scenario: Duas credenciais configuradas
- **WHEN** as duas credenciais do `claude` estão configuradas e um run é executado
- **THEN** o ambiente do run recebe só o token OAuth e o run registra `auth_method` igual a `oauth`

### Requirement: Private repository access
Se um token do git estiver configurado, o serviço SHALL usá-lo para clonar o repositório do run, sem gravá-lo na URL do remote, no banco, nos eventos ou nos logs do serviço.

#### Scenario: Repositório privado com token
- **WHEN** um token do git com acesso ao repositório está configurado e o `repo_url` é privado
- **THEN** o clone funciona e a configuração do remote no workspace não contém o token

### Requirement: Run workspace
O workspace de cada run, com o clone e o commit, e os eventos do agente SHALL ficar no host, num diretório próprio do run, depois que o ambiente de execução terminar.

#### Scenario: Revisar o resultado localmente
- **WHEN** um run termina `succeeded`
- **THEN** o diretório do run no host contém o clone na branch do run, com o commit, e os eventos do agente

### Requirement: Isolated run environment
O ambiente de execução SHALL rodar sem privilégios, com um usuário não-root, sem acesso ao Docker do host e fora da rede interna do compose. Ele SHALL ter acesso à internet para falar com o provedor do agente e com o host do git. O token do git SHALL não estar disponível ao processo do agente.

#### Scenario: Rede interna inacessível ao agente
- **WHEN** o agente tenta se conectar a `postgres:5432` ou a `rabbitmq:5672` durante um run
- **THEN** os nomes não resolvem e a conexão não é possível

#### Scenario: Token do git fora do alcance do agente
- **WHEN** um token do git está configurado e o agente lista as variáveis de ambiente do seu processo
- **THEN** o token do git não aparece

### Requirement: Worker recovery
Ao iniciar, o processo de execução SHALL marcar como `failed`, com `error` igual a `worker restarted`, os runs que estavam em `running`, mover as tasks deles conforme a regra de status e remover os ambientes de execução que tiverem sobrado. Os runs em `queued` SHALL continuar na fila.

#### Scenario: Worker reiniciado no meio de um run
- **WHEN** o worker é reiniciado enquanto um run está em `running`
- **THEN** depois de subir, o run está `failed` com `error` `worker restarted`, a task está em `failed` e não há ambiente de execução sobrando

### Requirement: Default agent image
Todo run SHALL usar a imagem default de execução, com Node 24, `git` e o CLI do agente numa versão fixa. Se a imagem não existir no host, o run SHALL ficar `failed` com um `error` que indica como construí-la.

#### Scenario: Imagem ausente
- **WHEN** a imagem default não foi construída e um run é executado
- **THEN** o run fica `failed` com um `error` que cita o comando para construir a imagem
