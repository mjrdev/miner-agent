# project-categories Specification

## Purpose

Mantém o cadastro das categorias de projeto às quais as tasks são associadas, servindo de base para configurações por projeto que o agente autônomo vai herdar no futuro.

## Requirements

### Requirement: Project category representation
As respostas de categoria SHALL ser JSON com `id` (UUID), `name`, `slug`, `description` (texto, possivelmente vazio), `created_at` e `updated_at`.

#### Scenario: Leitura de uma categoria
- **WHEN** um cliente envia `GET /project-categories/{id}` para uma categoria existente
- **THEN** o serviço responde `200 OK` com todos esses campos

#### Scenario: Categoria inexistente
- **WHEN** um cliente envia `GET /project-categories/{id}` com um id que não existe ou que não é UUID
- **THEN** o serviço responde `404 Not Found`

### Requirement: Create a project category
`POST /project-categories` SHALL criar uma categoria e responder `201 Created`. Os campos são:
- `name`: obrigatório, de 1 a 100 caracteres depois do trim
- `slug`: opcional e, se enviado, só com minúsculas, dígitos e hifens, de 1 a 100 caracteres. Se omitido, é derivado do `name` (minúsculas, sem acentos, outros caracteres viram hifens)
- `description`: opcional

O slug SHALL ser único. Um slug repetido SHALL resultar em `409` com `conflict`.

#### Scenario: Slug derivado do nome
- **WHEN** um cliente envia `POST /project-categories` com `{"name":"Integrações Back-end"}`
- **THEN** o serviço responde `201 Created` com `slug` igual a `integracoes-back-end`

#### Scenario: Slug duplicado
- **WHEN** já existe uma categoria com slug `backend` e um cliente envia `POST /project-categories` com `{"name":"Backend"}`
- **THEN** o serviço responde `409 Conflict` com `error.code` igual a `conflict`

#### Scenario: Slug inválido
- **WHEN** um cliente envia `{"name":"X","slug":"Com Espaço"}`
- **THEN** o serviço responde `422 Unprocessable Entity`

### Requirement: List project categories
`GET /project-categories` SHALL listar todas as categorias ordenadas por `name`, no formato `{"items":[...]}`.

#### Scenario: Listagem
- **WHEN** existem as categorias `Infra` e `Backend` e um cliente envia `GET /project-categories`
- **THEN** a resposta contém as duas, com `Backend` antes de `Infra`

### Requirement: Update a project category
`PATCH /project-categories/{id}` SHALL alterar apenas os campos enviados (`name`, `slug` e `description`), com as mesmas validações e a mesma regra de unicidade da criação, e responder `200 OK`. A alteração SHALL se refletir em todas as tasks associadas.

#### Scenario: Renomear categoria
- **WHEN** um cliente envia `PATCH` com `{"name":"Plataforma"}` para uma categoria associada a tasks
- **THEN** o serviço responde `200 OK` e essas tasks passam a mostrar `project_category.name` igual a `Plataforma`, com o `slug` inalterado

### Requirement: Delete a project category
`DELETE /project-categories/{id}` SHALL remover uma categoria sem tasks associadas e responder `204 No Content`. Se houver tasks associadas, SHALL responder `409` com `conflict`, sem remover nada.

#### Scenario: Categoria sem uso
- **WHEN** um cliente envia `DELETE` para uma categoria sem tasks associadas
- **THEN** o serviço responde `204 No Content`

#### Scenario: Categoria em uso
- **WHEN** um cliente envia `DELETE` para uma categoria com pelo menos uma task associada
- **THEN** o serviço responde `409 Conflict` e a categoria continua existindo
