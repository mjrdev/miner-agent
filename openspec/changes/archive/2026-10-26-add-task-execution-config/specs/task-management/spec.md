# Spec Delta

## MODIFIED Requirements

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

## ADDED Requirements

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
