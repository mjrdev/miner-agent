# Spec Delta

## Purpose

Define quais agentes de IA, modelos, níveis de esforço e runners podem executar as tasks, servindo de catálogo comum para validar a configuração de execução e montar as opções do CRUD.

## ADDED Requirements

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
