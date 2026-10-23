# Spec Delta

## Purpose

Garante que qualquer pessoa desenvolvedora consiga subir o serviço HTTP e o PostgreSQL localmente com um único comando via Docker, sem instalar Go ou PostgreSQL na máquina.

## ADDED Requirements

### Requirement: Single-command local startup
O repositório SHALL permitir subir a API e o PostgreSQL com `docker compose up` (ou `make up`), com o serviço respondendo em `http://localhost:8080` usando apenas valores padrão de desenvolvimento.

#### Scenario: Primeira execução
- **WHEN** uma pessoa clona o repositório e executa `docker compose up` sem nenhuma configuração extra
- **THEN** os containers `api` e `postgres` sobem e `GET http://localhost:8080/readyz` responde `200 OK`

### Requirement: API waits for a healthy database
O ambiente local SHALL iniciar a API somente depois que o PostgreSQL estiver aceitando conexões, conforme seu healthcheck.

#### Scenario: Ordem de inicialização
- **WHEN** `docker compose up` é executado
- **THEN** o container `api` só inicia após o healthcheck do `postgres` reportar saudável

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
