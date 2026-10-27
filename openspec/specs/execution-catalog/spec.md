# execution-catalog Specification

## Purpose

Define quais agentes de IA, modelos, níveis de esforço e runners podem executar as tasks, servindo de catálogo comum para validar a configuração de execução e montar as opções do CRUD.

## Requirements

### Requirement: Agent catalog
O serviço SHALL manter um catálogo de agentes registrados. Cada agente tem um `name` único e uma lista de modelos, e cada modelo tem um `id` único dentro do agente e uma lista, possivelmente vazia, de níveis de esforço (`efforts`). O catálogo SHALL ser fixo durante a execução do processo, e adicionar um agente SHALL exigir apenas registrar um novo adapter, sem alterar as regras de task.

#### Scenario: Agente registrado aparece no catálogo
- **WHEN** o serviço sobe com o agente `claude` registrado
- **THEN** `claude` e seus modelos aparecem no catálogo exposto por `GET /execution-options`

### Requirement: Claude agent
O agente `claude` SHALL oferecer os modelos `fable`, `opus` e `sonnet`, cada um com os níveis de esforço `low`, `medium`, `high`, `xhigh` e `max`, nessa ordem.

#### Scenario: Modelos e esforços do claude
- **WHEN** um cliente envia `GET /execution-options`
- **THEN** o agente `claude` lista os modelos `fable`, `opus` e `sonnet`, e o modelo `opus` lista os esforços `["low","medium","high","xhigh","max"]`

### Requirement: Available runners
O serviço SHALL manter a lista de runners disponíveis, identificados por nome. Nesta versão a lista SHALL conter apenas `host`.

#### Scenario: Runner k8s indisponível
- **WHEN** um cliente envia `GET /execution-options`
- **THEN** `runners` é `["host"]` e não contém `k8s`

### Requirement: List execution options
`GET /execution-options` SHALL responder `200 OK` com `{"agents":[{"name","models":[{"id","efforts":[...]}]}],"runners":[...]}`. Agentes SHALL vir ordenados por `name`, modelos e esforços na ordem definida pelo agente, e runners por nome.

#### Scenario: Leitura das opções
- **WHEN** um cliente envia `GET /execution-options`
- **THEN** o serviço responde `200 OK` com `agents` contendo `claude` e `runners` contendo `host`

### Requirement: Agent invocation
Cada agente SHALL definir como é executado sem interação a partir do prompt, do modelo e do esforço, e SHALL emitir os eventos de execução em JSON, um por linha. O esforço SHALL ser repassado só quando definido. O `claude` SHALL ser executado em modo não interativo, com o modelo e o esforço do run, saída em `stream-json` e sem pedir permissão para usar ferramentas.

#### Scenario: Comando do claude
- **WHEN** um run usa o agente `claude` com o modelo `sonnet` e o esforço `low`
- **THEN** o CLI do Claude Code é executado sem interação com `--model sonnet`, `--effort low` e `--output-format stream-json`, recebendo o prompt da task

#### Scenario: Modelo sem esforço
- **WHEN** um run usa um modelo sem níveis de esforço
- **THEN** o comando do agente não recebe nenhum esforço

### Requirement: Agent credential priority
Cada agente SHALL definir as credenciais que aceita e a ordem de prioridade entre elas. O `claude` SHALL aceitar um token OAuth da assinatura (`oauth`) e uma chave de API (`api_key`), nessa ordem de prioridade.

#### Scenario: Só a chave de API
- **WHEN** só a chave de API do `claude` está configurada
- **THEN** o run usa a chave de API e registra `auth_method` igual a `api_key`
