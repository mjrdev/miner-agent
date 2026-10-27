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
- `agent`, `model`, `effort` e `runner` (texto ou `null`)
- `created_at` e `updated_at` (RFC 3339)

O payload bruto do provider SHALL ficar fora da resposta.

#### Scenario: Leitura de uma task
- **WHEN** um cliente envia `GET /tasks/{id}` para uma task existente
- **THEN** o serviço responde `200 OK` com todos os campos acima e sem o payload bruto

#### Scenario: Task inexistente
- **WHEN** um cliente envia `GET /tasks/{id}` com um UUID que não existe ou com um id que não é UUID
- **THEN** o serviço responde `404 Not Found`

#### Scenario: Task sem configuração de execução
- **WHEN** uma task chega por webhook e um cliente envia `GET /tasks/{id}`
- **THEN** `agent`, `model`, `effort` e `runner` são `null`

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

São opcionais `additional_instructions`, `labels`, `project_category_id`, `agent`, `model`, `effort` e `runner`.

#### Scenario: Criação mínima
- **WHEN** um cliente envia `POST /tasks` com `{"title":"Adicionar paginação","description":"Implemente ..."}`
- **THEN** o serviço responde `201 Created` com uma task `local` em `draft`, sem instruções adicionais, labels, categoria nem configuração de execução

#### Scenario: Criação com configuração de execução
- **WHEN** um cliente envia `POST /tasks` com título, descrição e `{"agent":"claude","model":"opus","effort":"high","runner":"host"}`
- **THEN** o serviço responde `201 Created` com a task em `draft` e esses quatro valores

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

Campos editáveis em qualquer task: `additional_instructions`, `labels`, `project_category_id` (`null` remove a categoria), `agent`, `model`, `effort` e `runner` (`null` limpa o campo) e `status`.

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

#### Scenario: Configurar execução de uma task externa
- **WHEN** um cliente envia `PATCH` com `{"agent":"claude","model":"sonnet","effort":"medium","runner":"host"}` para uma task de provider `jira` em `draft`
- **THEN** o serviço responde `200 OK` com esses quatro valores

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

### Requirement: Delete a task
`DELETE /tasks/{id}` SHALL remover a task, junto com suas associações de labels, e responder `204 No Content`. Se a task não existir, SHALL responder `404`.

#### Scenario: Remoção
- **WHEN** um cliente envia `DELETE /tasks/{id}` para uma task existente
- **THEN** o serviço responde `204 No Content` e um `GET /tasks/{id}` seguinte responde `404`

### Requirement: Execution configuration
`agent`, `model`, `effort` e `runner` SHALL ser validados contra o catálogo de execução depois de aplicados os campos enviados:
- `agent`, se definido, é um agente registrado
- `model`, se definido, exige `agent` e é um modelo desse agente
- `effort`, se definido, exige `model` e é um esforço desse modelo
- `runner`, se definido, é um runner disponível

Uma combinação inválida SHALL resultar em `422` com `validation_failed`, sem alterar nada.

#### Scenario: Agente desconhecido
- **WHEN** um cliente envia `PATCH` com `{"agent":"gpt"}`
- **THEN** o serviço responde `422 Unprocessable Entity` e a task permanece inalterada

#### Scenario: Modelo de outro agente
- **WHEN** um cliente envia `PATCH` com `{"agent":"claude","model":"gpt-5"}`
- **THEN** o serviço responde `422 Unprocessable Entity`

#### Scenario: Esforço sem modelo
- **WHEN** a task não tem `model` e um cliente envia `PATCH` com `{"effort":"high"}`
- **THEN** o serviço responde `422 Unprocessable Entity`

#### Scenario: Runner indisponível
- **WHEN** um cliente envia `PATCH` com `{"runner":"k8s"}`
- **THEN** o serviço responde `422 Unprocessable Entity`

#### Scenario: Trocar o agente deixando um modelo incompatível
- **WHEN** a task tem `agent` `claude` e `model` `opus`, e um cliente envia `PATCH` só com `{"agent":"outro"}`, sendo `outro` um agente registrado sem o modelo `opus`
- **THEN** o serviço responde `422 Unprocessable Entity` e a task permanece inalterada

### Requirement: Execution configuration lock
`agent`, `model`, `effort` e `runner` SHALL ficar imutáveis enquanto a task estiver em `in_progress`, `done` ou `cancelled`. Enviar qualquer um deles, mesmo com o valor atual, para uma task nesses estados SHALL resultar em `422` com `validation_failed`, sem alterar nada.

#### Scenario: Trocar o modelo durante a execução
- **WHEN** um cliente envia `PATCH` com `{"model":"sonnet"}` para uma task em `in_progress`
- **THEN** o serviço responde `422 Unprocessable Entity` e o modelo não muda

#### Scenario: Trocar o modelo antes de tentar de novo
- **WHEN** um cliente envia `PATCH` com `{"model":"sonnet","effort":"high"}` para uma task em `failed`
- **THEN** o serviço responde `200 OK` com o novo modelo

### Requirement: Ready prerequisites
Uma task SHALL só poder estar em `ready` com `agent`, `model` e `runner` definidos, `effort` definido quando o modelo tem esforços, `project_category` definida e uma categoria com `repo_url`. A regra SHALL valer na transição para `ready` e em qualquer `PATCH` de uma task que já está em `ready`. Se ela não for cumprida, o serviço SHALL responder `422` com `validation_failed`, citar os itens faltantes e não alterar nada.

#### Scenario: Liberar task configurada
- **WHEN** uma task em `draft` tem `agent` `claude`, `model` `opus`, `effort` `high`, `runner` `host` e uma categoria com `repo_url`, e um cliente envia `PATCH` com `{"status":"ready"}`
- **THEN** o serviço responde `200 OK` e a task fica em `ready`

#### Scenario: Liberar task sem configuração
- **WHEN** uma task em `draft` não tem `agent` e um cliente envia `PATCH` com `{"status":"ready"}`
- **THEN** o serviço responde `422 Unprocessable Entity`, a mensagem cita `agent` e a task continua em `draft`

#### Scenario: Categoria sem repositório
- **WHEN** uma task em `draft` está totalmente configurada, mas a categoria dela não tem `repo_url`, e um cliente envia `PATCH` com `{"status":"ready"}`
- **THEN** o serviço responde `422 Unprocessable Entity` e a task continua em `draft`

#### Scenario: Configurar e liberar no mesmo pedido
- **WHEN** um cliente envia, para uma task em `draft` com categoria que tem `repo_url`, `PATCH` com `{"agent":"claude","model":"opus","effort":"low","runner":"host","status":"ready"}`
- **THEN** o serviço responde `200 OK` e a task fica em `ready` com essa configuração

#### Scenario: Limpar configuração de uma task em ready
- **WHEN** um cliente envia `PATCH` com `{"runner":null}` ou `{"project_category_id":null}` para uma task em `ready`
- **THEN** o serviço responde `422 Unprocessable Entity` e a task permanece inalterada

#### Scenario: Voltar para draft sem configuração
- **WHEN** um cliente envia `PATCH` com `{"status":"draft","runner":null}` para uma task em `ready`
- **THEN** o serviço responde `200 OK` e a task fica em `draft` sem `runner`

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
