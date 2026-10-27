# project-categories Specification

## Purpose

Mantém o cadastro das categorias de projeto às quais as tasks são associadas, servindo de base para configurações por projeto que o agente autônomo vai herdar no futuro.

## Requirements

### Requirement: Project category representation
As respostas de categoria SHALL ser JSON com `id` (UUID), `name`, `slug`, `description` (texto, possivelmente vazio), `repo_url` (texto ou `null`), `base_branch`, `created_at` e `updated_at`.

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
- `repo_url`: opcional, conforme a regra de repositório
- `base_branch`: opcional, padrão `main`, conforme a regra de branch base

O slug SHALL ser único. Um slug repetido SHALL resultar em `409` com `conflict`.

#### Scenario: Slug derivado do nome
- **WHEN** um cliente envia `POST /project-categories` com `{"name":"Integrações Back-end"}`
- **THEN** o serviço responde `201 Created` com `slug` igual a `integracoes-back-end`, `repo_url` `null` e `base_branch` `main`

#### Scenario: Slug duplicado
- **WHEN** já existe uma categoria com slug `backend` e um cliente envia `POST /project-categories` com `{"name":"Backend"}`
- **THEN** o serviço responde `409 Conflict` com `error.code` igual a `conflict`

#### Scenario: Slug inválido
- **WHEN** um cliente envia `{"name":"X","slug":"Com Espaço"}`
- **THEN** o serviço responde `422 Unprocessable Entity`

#### Scenario: Categoria com repositório
- **WHEN** um cliente envia `{"name":"Miner","repo_url":"https://github.com/mjrdev/miner-agent","base_branch":"develop"}`
- **THEN** o serviço responde `201 Created` com esses `repo_url` e `base_branch`

### Requirement: List project categories
`GET /project-categories` SHALL listar todas as categorias ordenadas por `name`, no formato `{"items":[...]}`.

#### Scenario: Listagem
- **WHEN** existem as categorias `Infra` e `Backend` e um cliente envia `GET /project-categories`
- **THEN** a resposta contém as duas, com `Backend` antes de `Infra`

### Requirement: Update a project category
`PATCH /project-categories/{id}` SHALL alterar apenas os campos enviados (`name`, `slug`, `description`, `repo_url` e `base_branch`), com as mesmas validações e a mesma regra de unicidade da criação, e responder `200 OK`. `repo_url` igual a `null` SHALL remover o repositório. `base_branch` SHALL não aceitar `null`. A alteração SHALL se refletir em todas as tasks associadas.

#### Scenario: Renomear categoria
- **WHEN** um cliente envia `PATCH` com `{"name":"Plataforma"}` para uma categoria associada a tasks
- **THEN** o serviço responde `200 OK` e essas tasks passam a mostrar `project_category.name` igual a `Plataforma`, com o `slug` inalterado

#### Scenario: Trocar a branch base
- **WHEN** um cliente envia `PATCH` com `{"base_branch":"release/1.x"}`
- **THEN** o serviço responde `200 OK` com o novo `base_branch` e os demais campos inalterados

### Requirement: Delete a project category
`DELETE /project-categories/{id}` SHALL remover uma categoria sem tasks associadas e responder `204 No Content`. Se houver tasks associadas, SHALL responder `409` com `conflict`, sem remover nada.

#### Scenario: Categoria sem uso
- **WHEN** um cliente envia `DELETE` para uma categoria sem tasks associadas
- **THEN** o serviço responde `204 No Content`

#### Scenario: Categoria em uso
- **WHEN** um cliente envia `DELETE` para uma categoria com pelo menos uma task associada
- **THEN** o serviço responde `409 Conflict` e a categoria continua existindo

### Requirement: Category repository
`repo_url` SHALL ser uma URL absoluta `https` com host e caminho, com no máximo 500 caracteres e sem credenciais embutidas (usuário ou senha na URL). Um valor inválido SHALL resultar em `422` com `validation_failed`.

#### Scenario: URL com credenciais
- **WHEN** um cliente envia `repo_url` igual a `https://user:token@github.com/org/repo`
- **THEN** o serviço responde `422 Unprocessable Entity`

#### Scenario: URL não HTTPS
- **WHEN** um cliente envia `repo_url` igual a `git@github.com:org/repo.git`
- **THEN** o serviço responde `422 Unprocessable Entity`

### Requirement: Category base branch
`base_branch` SHALL ter de 1 a 255 caracteres, só com letras, dígitos, `.`, `_`, `-` e `/`. Ele SHALL não começar com `-` ou `/`, não terminar com `/` e não conter `..`. Um valor inválido SHALL resultar em `422` com `validation_failed`.

#### Scenario: Branch inválida
- **WHEN** um cliente envia `base_branch` igual a `feature..x`
- **THEN** o serviço responde `422 Unprocessable Entity`

### Requirement: Keep repository of categories with ready tasks
Remover o `repo_url` (`null`) de uma categoria que tem ao menos uma task em `ready` SHALL resultar em `409` com `conflict`, sem alterar nada. Trocar o `repo_url` por outra URL válida SHALL ser permitido.

#### Scenario: Remover repositório em uso
- **WHEN** uma categoria tem uma task em `ready` e um cliente envia `PATCH` com `{"repo_url":null}`
- **THEN** o serviço responde `409 Conflict` e o `repo_url` não muda

#### Scenario: Remover repositório sem tasks em ready
- **WHEN** uma categoria só tem tasks em `draft` e um cliente envia `PATCH` com `{"repo_url":null}`
- **THEN** o serviço responde `200 OK` com `repo_url` `null`
