# Spec Delta

## MODIFIED Requirements

### Requirement: Status transitions
As únicas transições permitidas SHALL ser:
- `draft` → `ready`
- `ready` → `in_progress` ou `draft`
- `in_progress` → `in_review`, `failed` ou `ready`
- `in_review` → `done` ou `ready`
- `failed` → `ready`
- qualquer estado, exceto `done` e `cancelled`, → `cancelled`

`done` e `cancelled` são terminais. Outra transição SHALL resultar em `409` com `invalid_transition`, sem alterar nada.

#### Scenario: Liberar para o agente
- **WHEN** um cliente envia `PATCH` com `{"status":"ready"}` para uma task em `draft` que cumpre os pré-requisitos de `ready`
- **THEN** o serviço responde `200 OK` e a task fica em `ready`

#### Scenario: Transição inválida
- **WHEN** um cliente envia `PATCH` com `{"status":"done"}` para uma task em `draft`
- **THEN** o serviço responde `409 Conflict` com `error.code` igual a `invalid_transition` e a task continua em `draft`

#### Scenario: Estado terminal
- **WHEN** um cliente envia `PATCH` com `{"status":"ready"}` para uma task em `cancelled`
- **THEN** o serviço responde `409 Conflict`

#### Scenario: Mesmo status
- **WHEN** um cliente envia `PATCH` com o status atual da task
- **THEN** o serviço responde `200 OK` sem tratar isso como transição

#### Scenario: Recuperar uma task sem run ativo
- **WHEN** uma task está em `in_progress` sem run ativo e um cliente envia `PATCH` com `{"status":"ready"}`
- **THEN** o serviço responde `200 OK` e a task fica em `ready`

## ADDED Requirements

### Requirement: Status while a run is active
Enquanto a task tiver um run ativo, o status dela SHALL ser controlado pelo run. Um `PATCH` com `status` diferente de `cancelled` SHALL resultar em `409` com `conflict`, sem alterar nada. Os demais campos editáveis em `in_progress` continuam editáveis.

#### Scenario: Mudar status durante o run
- **WHEN** uma task tem um run em `running` e um cliente envia `PATCH` com `{"status":"failed"}`
- **THEN** o serviço responde `409 Conflict` com `error.code` igual a `conflict` e a task continua em `in_progress`

#### Scenario: Editar labels durante o run
- **WHEN** uma task tem um run em `running` e um cliente envia `PATCH` com `{"labels":["urgente"]}`
- **THEN** o serviço responde `200 OK` com as labels atualizadas

### Requirement: Cancel a task with an active run
Cancelar uma task (`PATCH` com `{"status":"cancelled"}`) que tem um run ativo SHALL cancelar também esse run, encerrando o ambiente de execução se ele estiver em `running`. A task SHALL ficar em `cancelled`.

#### Scenario: Cancelar task em execução
- **WHEN** uma task tem um run em `running` e um cliente envia `PATCH` com `{"status":"cancelled"}`
- **THEN** o serviço responde `200 OK` com a task em `cancelled`, e o run fica `cancelled` depois que o ambiente é encerrado

### Requirement: Delete a task with an active run
`DELETE /tasks/{id}` de uma task com um run ativo SHALL resultar em `409` com `conflict`, sem remover nada. Uma task sem run ativo SHALL ser removida junto com o histórico dos seus runs.

#### Scenario: Remover task em execução
- **WHEN** uma task tem um run em `running` e um cliente envia `DELETE /tasks/{id}`
- **THEN** o serviço responde `409 Conflict` e a task continua existindo

#### Scenario: Remover task com runs terminados
- **WHEN** uma task só tem runs terminados e um cliente envia `DELETE /tasks/{id}`
- **THEN** o serviço responde `204 No Content` e `GET /runs/{id}` desses runs responde `404`
