# health-checks Specification

## Purpose

Permite que pessoas desenvolvedoras, orquestradores de containers e ferramentas de monitoramento saibam se o serviço está vivo e se está pronto para atender requisições, incluindo o estado do banco de dados.

## Requirements

### Requirement: Liveness endpoint
O serviço SHALL expor `GET /healthz`, que responde `200 OK` com corpo JSON `{"status":"ok"}` sempre que o processo estiver em execução e aceitando conexões HTTP, sem depender do estado do banco de dados.

#### Scenario: Serviço em execução
- **WHEN** um cliente envia `GET /healthz`
- **THEN** o serviço responde `200 OK` com `Content-Type: application/json` e corpo `{"status":"ok"}`

#### Scenario: Banco indisponível não afeta liveness
- **WHEN** o PostgreSQL está inacessível e um cliente envia `GET /healthz`
- **THEN** o serviço ainda responde `200 OK` com corpo `{"status":"ok"}`

### Requirement: Readiness endpoint
O serviço SHALL expor `GET /readyz`, que verifica a conectividade com o PostgreSQL dentro de um tempo limite de no máximo 2 segundos e responde `200 OK` quando o banco responde, ou `503 Service Unavailable` quando não responde.

#### Scenario: Banco disponível
- **WHEN** o PostgreSQL está acessível e um cliente envia `GET /readyz`
- **THEN** o serviço responde `200 OK` com corpo JSON `{"status":"ready","checks":{"database":"ok"}}`

#### Scenario: Banco indisponível
- **WHEN** o PostgreSQL está inacessível ou não responde em até 2 segundos e um cliente envia `GET /readyz`
- **THEN** o serviço responde `503 Service Unavailable` com corpo JSON `{"status":"not_ready","checks":{"database":"unavailable"}}`
- **AND** a resposta não expõe detalhes internos do erro (como credenciais ou strings de conexão)

### Requirement: Unknown routes and methods
O serviço SHALL responder `404 Not Found` para rotas não registradas e `405 Method Not Allowed` para métodos não suportados em rotas existentes.

#### Scenario: Rota inexistente
- **WHEN** um cliente envia `GET /nao-existe`
- **THEN** o serviço responde `404 Not Found`

#### Scenario: Método não suportado
- **WHEN** um cliente envia `POST /healthz`
- **THEN** o serviço responde `405 Method Not Allowed`
