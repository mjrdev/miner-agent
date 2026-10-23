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
O serviço SHALL expor `GET /readyz`, que verifica a conectividade com o PostgreSQL e com o RabbitMQ dentro de um tempo limite de no máximo 2 segundos. Responde `200 OK` quando as duas dependências estão disponíveis e `503 Service Unavailable` quando qualquer uma delas não está, informando o estado de cada uma em `checks`.

#### Scenario: Banco disponível
- **WHEN** o PostgreSQL e o RabbitMQ estão acessíveis e um cliente envia `GET /readyz`
- **THEN** o serviço responde `200 OK` com corpo JSON `{"status":"ready","checks":{"broker":"ok","database":"ok"}}`

#### Scenario: Banco indisponível
- **WHEN** o PostgreSQL está inacessível ou não responde em até 2 segundos e um cliente envia `GET /readyz`
- **THEN** o serviço responde `503 Service Unavailable` com corpo JSON em que `status` é `not_ready` e `checks.database` é `unavailable`
- **AND** a resposta não expõe detalhes internos do erro (como credenciais ou strings de conexão)

#### Scenario: Broker indisponível
- **WHEN** o PostgreSQL está acessível, mas a conexão com o RabbitMQ está fechada, e um cliente envia `GET /readyz`
- **THEN** o serviço responde `503 Service Unavailable` com corpo JSON `{"status":"not_ready","checks":{"broker":"unavailable","database":"ok"}}`
- **AND** a resposta não expõe detalhes internos do erro

### Requirement: Unknown routes and methods
O serviço SHALL responder `404 Not Found` para rotas não registradas e `405 Method Not Allowed` para métodos não suportados em rotas existentes.

#### Scenario: Rota inexistente
- **WHEN** um cliente envia `GET /nao-existe`
- **THEN** o serviço responde `404 Not Found`

#### Scenario: Método não suportado
- **WHEN** um cliente envia `POST /healthz`
- **THEN** o serviço responde `405 Method Not Allowed`
