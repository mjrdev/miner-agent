# local-dev-environment Specification

## Purpose

Garante que qualquer pessoa desenvolvedora consiga subir o serviço HTTP e o PostgreSQL localmente com um único comando via Docker, sem instalar Go ou PostgreSQL na máquina.

## Requirements

### Requirement: Single-command local startup
O repositório SHALL permitir subir a API, o PostgreSQL e o RabbitMQ, com o schema do banco já migrado, usando `docker compose up` (ou `make up`). O serviço SHALL responder em `http://localhost:8080` usando apenas valores padrão de desenvolvimento.

#### Scenario: Primeira execução
- **WHEN** uma pessoa clona o repositório e executa `docker compose up` sem nenhuma configuração extra
- **THEN** os containers `api`, `postgres` e `rabbitmq` sobem, as migrações são aplicadas e `GET http://localhost:8080/readyz` responde `200 OK`

#### Scenario: Task local de ponta a ponta
- **WHEN** com o ambiente no ar e sem `.env`, a pessoa envia `POST http://localhost:8080/webhooks/local` com o token de desenvolvimento documentado no `.env.example` e um corpo válido
- **THEN** o serviço responde `202` e, em seguida, a task aparece em `GET http://localhost:8080/tasks`

### Requirement: API waits for a healthy database
O ambiente local SHALL iniciar a API somente depois que o PostgreSQL e o RabbitMQ estiverem saudáveis, conforme seus healthchecks, e as migrações tiverem terminado com sucesso.

#### Scenario: Ordem de inicialização
- **WHEN** `docker compose up` é executado
- **THEN** o container `api` só inicia depois que os healthchecks de `postgres` e `rabbitmq` reportam saudável e as migrações terminam com sucesso

#### Scenario: Migração falha
- **WHEN** uma migração falha durante `docker compose up`
- **THEN** o container `api` não inicia e o erro da migração aparece nos logs

### Requirement: Persistent database data
O ambiente local SHALL armazenar os dados do PostgreSQL em um volume nomeado, preservando-os entre reinicializações, e SHALL permitir removê-los explicitamente.

#### Scenario: Reinício preserva dados
- **WHEN** a pessoa executa `docker compose down` e depois `docker compose up`
- **THEN** os dados gravados anteriormente no banco continuam disponíveis

#### Scenario: Reset explícito
- **WHEN** a pessoa executa `docker compose down -v` (ou `make reset`)
- **THEN** o volume do banco é removido e a próxima subida começa com um banco vazio

### Requirement: Hot reload in development
O container da API em desenvolvimento SHALL recompilar e reiniciar o serviço automaticamente quando arquivos `.go` do projeto forem alterados no host.

#### Scenario: Alteração de código
- **WHEN** a pessoa edita e salva um arquivo `.go` com o ambiente rodando
- **THEN** o serviço é recompilado e reiniciado dentro do container sem reiniciar o `docker compose`

### Requirement: Overridable local configuration
O ambiente local SHALL ler sobrescritas opcionais de um arquivo `.env` na raiz (não versionado) e SHALL fornecer um `.env.example` versionado documentando todas as variáveis.

#### Scenario: Sobrescrever porta
- **WHEN** a pessoa cria `.env` com `HTTP_PORT=9090` e executa `docker compose up`
- **THEN** a API fica acessível em `http://localhost:9090`

#### Scenario: Sem arquivo .env
- **WHEN** não existe arquivo `.env`
- **THEN** o ambiente sobe normalmente com os valores padrão

### Requirement: Database migrations
O repositório SHALL versionar as migrações SQL em `migrations/` e SHALL permitir aplicar as pendentes com `make migrate` e reverter a última com `make migrate-down`, contra o banco do ambiente local. Aplicar migrações já aplicadas SHALL ser uma operação sem efeito.

#### Scenario: Aplicar migrações
- **WHEN** a pessoa executa `make migrate` com o ambiente no ar
- **THEN** todas as migrações pendentes são aplicadas e as tabelas de tasks, labels e categorias existem

#### Scenario: Reaplicar sem pendências
- **WHEN** a pessoa executa `make migrate` sem migrações pendentes
- **THEN** o comando termina com sucesso sem alterar o banco

### Requirement: Local message broker
O ambiente local SHALL incluir um RabbitMQ com o painel de administração acessível em `http://localhost:15672`, credenciais de desenvolvimento documentadas no `.env.example` e dados guardados em um volume nomeado. O volume SHALL ser removido junto com o do banco por `docker compose down -v` (ou `make reset`).

#### Scenario: Painel de administração
- **WHEN** o ambiente está no ar e a pessoa abre `http://localhost:15672` com as credenciais documentadas
- **THEN** ela vê as filas de ingestão de tasks e a dead-letter queue
