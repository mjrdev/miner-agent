# Spec Delta

## MODIFIED Requirements

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
