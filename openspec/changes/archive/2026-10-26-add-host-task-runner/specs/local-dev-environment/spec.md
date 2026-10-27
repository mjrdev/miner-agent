# Spec Delta

## MODIFIED Requirements

### Requirement: Single-command local startup
O repositório SHALL permitir subir a API, o worker de execução, o PostgreSQL e o RabbitMQ, com o schema do banco já migrado, usando `docker compose up` (ou `make up`). O serviço SHALL responder em `http://localhost:8080` usando apenas valores padrão de desenvolvimento.

#### Scenario: Primeira execução
- **WHEN** uma pessoa clona o repositório e executa `docker compose up` sem nenhuma configuração extra
- **THEN** os containers `api`, `worker`, `postgres` e `rabbitmq` sobem, as migrações são aplicadas e `GET http://localhost:8080/readyz` responde `200 OK`

#### Scenario: Task local de ponta a ponta
- **WHEN** com o ambiente no ar e sem `.env`, a pessoa envia `POST http://localhost:8080/webhooks/local` com o token de desenvolvimento documentado no `.env.example` e um corpo válido
- **THEN** o serviço responde `202` e, em seguida, a task aparece em `GET http://localhost:8080/tasks`

## ADDED Requirements

### Requirement: Agent image build
`make up` SHALL construir a imagem default de execução antes de subir o ambiente, e `make agent-image` SHALL construí-la de forma isolada. Uma imagem já construída SHALL ser reaproveitada pelo cache.

#### Scenario: Construir a imagem
- **WHEN** a pessoa executa `make agent-image`
- **THEN** a imagem default de execução existe no Docker do host com o CLI do agente na versão fixada

### Requirement: Execution credentials configuration
As credenciais do agente e o token do git SHALL ser opcionais, lidas do `.env` apenas pelo worker e documentadas no `.env.example` sem valor. A ausência delas SHALL não impedir o ambiente de subir.

#### Scenario: Ambiente sem credenciais
- **WHEN** o `.env` não define nenhuma credencial e a pessoa executa `make up`
- **THEN** o ambiente sobe, o worker registra um aviso de que não há credenciais do agente, e os runs falham com `no agent credentials configured`

#### Scenario: API sem segredos
- **WHEN** o `.env` define as credenciais do agente e o token do git
- **THEN** essas variáveis estão presentes só no container `worker`, não no `api`

### Requirement: Runs directory on host
O diretório dos runs SHALL ficar em `./runs` na raiz do repositório, fora do controle de versão, e SHALL sobreviver a `make down` e a `make reset`.

#### Scenario: Inspecionar um run
- **WHEN** um run termina
- **THEN** existe `./runs/<run-id>/` no host com o workspace e os eventos do run
