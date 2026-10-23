# task-management Specification

## Purpose

Permite consultar, criar e editar as tasks que alimentam o agente de desenvolvimento autônomo. Cobre instruções adicionais, labels, categoria de projeto e um ciclo de vida de status validado, respeitando quais campos pertencem ao provider de origem e quais pertencem à pessoa usuária.

## Requirements

### Requirement: Task representation
As respostas de task SHALL ser JSON com os campos:
- `id` (UUID), `provider`, `external_id` e `external_url` (ou `null`)
- `title` e `description` (o prompt)
- `additional_instructions` (lista ordenada de textos) e `labels` (lista de nomes em ordem alfabética)
- `project_category` (objeto `{id, slug, name}` ou `null`) e `status`
- `created_at` e `updated_at` (RFC 3339)

O payload bruto do provider SHALL ficar fora da resposta.

#### Scenario: Leitura de uma task
- **WHEN** um cliente envia `GET /tasks/{id}` para uma task existente
- **THEN** o serviço responde `200 OK` com todos os campos acima e sem o payload bruto

#### Scenario: Task inexistente
- **WHEN** um cliente envia `GET /tasks/{id}` com um UUID que não existe ou com um id que não é UUID
- **THEN** o serviço responde `404 Not Found`

### Requirement: Error responses
Os erros dos endpoints de tasks, categorias e labels SHALL usar o corpo JSON `{"error":{"code":"<código>","message":"<texto>"}}`, sem expor detalhes internos. Os códigos são:
- `validation_failed` para `400` e `422`
- `not_found` para `404`
- `conflict` ou `invalid_transition` para `409`

#### Scenario: Corpo inválido
- **WHEN** um cliente envia `POST /tasks` com JSON malformado ou com um campo desconhecido
- **THEN** o serviço responde `400 Bad Request` com `error.code` igual a `validation_failed`

### Requirement: Create a local task
`POST /tasks` SHALL criar uma task de forma síncrona, com `provider` `local`, `external_id` igual ao `id` gerado e status `draft`, respondendo `201 Created` com a task. São obrigatórios:
- `title`: de 1 a 200 caracteres depois do trim
- `description`: não vazia

São opcionais `additional_instructions`, `labels` e `project_category_id`.

#### Scenario: Criação mínima
- **WHEN** um cliente envia `POST /tasks` com `{"title":"Adicionar paginação","description":"Implemente ..."}`
- **THEN** o serviço responde `201 Created` com uma task `local` em `draft`, sem instruções adicionais, labels nem categoria

#### Scenario: Campos obrigatórios ausentes
- **WHEN** um cliente envia `POST /tasks` sem `description`
- **THEN** o serviço responde `422 Unprocessable Entity` com `error.code` igual a `validation_failed`

#### Scenario: Categoria inexistente
- **WHEN** um cliente envia `POST /tasks` com um `project_category_id` que não existe
- **THEN** o serviço responde `422 Unprocessable Entity` e nenhuma task é criada

### Requirement: List tasks with filters
`GET /tasks` SHALL listar as tasks ordenadas da mais recente para a mais antiga, no formato `{"items":[...],"total":<n>}`.

Filtros opcionais, combinados com E: `status`, `provider`, `project_category_id` e `label` (task que contém a label).

Paginação: `limit` (padrão 50, máximo 200) e `offset` (padrão 0). Valores inválidos de filtro ou paginação SHALL resultar em `400`.

#### Scenario: Filtro por status e label
- **WHEN** existem tasks em `draft` e `ready`, algumas com a label `backend`, e um cliente envia `GET /tasks?status=ready&label=backend`
- **THEN** a resposta contém apenas as tasks em `ready` que têm a label `backend`, e `total` conta só essas

#### Scenario: Limite acima do máximo
- **WHEN** um cliente envia `GET /tasks?limit=500`
- **THEN** o serviço responde `400 Bad Request`

### Requirement: Field ownership on update
`PATCH /tasks/{id}` SHALL alterar apenas os campos enviados e responder `200 OK` com a task atualizada.

Campos editáveis em qualquer task: `additional_instructions`, `labels`, `project_category_id` (`null` remove a categoria) e `status`.

`title` e `description` só SHALL ser editáveis em tasks `local`. Enviá-los para uma task de outro provider SHALL resultar em `422` sem alterar nada.

#### Scenario: Editar instruções de uma task externa
- **WHEN** um cliente envia `PATCH` com `{"additional_instructions":["Use a lib X"]}` para uma task de provider `jira`
- **THEN** o serviço responde `200 OK` e só as instruções adicionais mudam

#### Scenario: Editar descrição de uma task externa
- **WHEN** um cliente envia `PATCH` com `{"description":"..."}` para uma task de provider diferente de `local`
- **THEN** o serviço responde `422 Unprocessable Entity` e a task permanece inalterada

#### Scenario: Editar descrição de uma task local
- **WHEN** um cliente envia `PATCH` com `{"description":"novo prompt"}` para uma task `local`
- **THEN** o serviço responde `200 OK` com a descrição atualizada e um `updated_at` mais recente

### Requirement: Additional instructions
`additional_instructions` SHALL ser uma lista ordenada de até 50 textos não vazios, cada um com no máximo 10.000 caracteres. Num `PATCH`, a lista enviada SHALL substituir a lista inteira, preservando a ordem.

#### Scenario: Substituição preserva a ordem
- **WHEN** um cliente envia `PATCH` com `{"additional_instructions":["passo 2","passo 1"]}`
- **THEN** a task passa a ter exatamente `["passo 2","passo 1"]`, nessa ordem

#### Scenario: Instrução vazia
- **WHEN** um cliente envia `additional_instructions` contendo `"   "`
- **THEN** o serviço responde `422 Unprocessable Entity`

### Requirement: Task labels
Ao serem atribuídas, as labels SHALL ser normalizadas (trim e minúsculas), deduplicadas e criadas caso ainda não existam. Cada nome tem de 1 a 50 caracteres. Num `PATCH`, a lista enviada SHALL substituir o conjunto inteiro (`[]` remove todas). `GET /labels` SHALL listar todas as labels existentes em ordem alfabética.

#### Scenario: Normalização e deduplicação
- **WHEN** um cliente envia `labels` igual a `["Backend"," backend ","API"]`
- **THEN** a task fica com as labels `["api","backend"]`, que passam a aparecer em `GET /labels`

#### Scenario: Remover todas as labels
- **WHEN** um cliente envia `PATCH` com `{"labels":[]}`
- **THEN** a task fica sem labels

### Requirement: Status values
`status` SHALL ser um destes valores: `draft`, `ready`, `in_progress`, `in_review`, `done`, `failed` ou `cancelled`. Um valor diferente enviado no `PATCH` SHALL resultar em `422` com `validation_failed`, sem alterar nada.

#### Scenario: Status desconhecido
- **WHEN** um cliente envia `PATCH` com `{"status":"archived"}`
- **THEN** o serviço responde `422 Unprocessable Entity` e o status da task não muda

### Requirement: Status transitions
As únicas transições permitidas SHALL ser:
- `draft` → `ready`
- `ready` → `in_progress` ou `draft`
- `in_progress` → `in_review` ou `failed`
- `in_review` → `done` ou `ready`
- `failed` → `ready`
- qualquer estado, exceto `done` e `cancelled`, → `cancelled`

`done` e `cancelled` são terminais. Outra transição SHALL resultar em `409` com `invalid_transition`, sem alterar nada.

#### Scenario: Liberar para o agente
- **WHEN** um cliente envia `PATCH` com `{"status":"ready"}` para uma task em `draft`
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

### Requirement: Delete a task
`DELETE /tasks/{id}` SHALL remover a task, junto com suas associações de labels, e responder `204 No Content`. Se a task não existir, SHALL responder `404`.

#### Scenario: Remoção
- **WHEN** um cliente envia `DELETE /tasks/{id}` para uma task existente
- **THEN** o serviço responde `204 No Content` e um `GET /tasks/{id}` seguinte responde `404`
