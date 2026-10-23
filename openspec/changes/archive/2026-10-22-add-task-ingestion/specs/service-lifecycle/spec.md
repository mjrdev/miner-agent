# Spec Delta

## MODIFIED Requirements

### Requirement: Configuration from environment variables
O serviço SHALL ler sua configuração de variáveis de ambiente: `APP_ENV`, `HTTP_PORT` (padrão `8080`), `LOG_LEVEL` (padrão `info`), `DATABASE_URL` (obrigatória), `RABBITMQ_URL` (obrigatória), `LOCAL_WEBHOOK_TOKEN` (opcional; vazia desativa a autenticação bem-sucedida do provider `local`) e `SHUTDOWN_TIMEOUT` (padrão `10s`).

#### Scenario: Valores padrão aplicados
- **WHEN** o serviço inicia com apenas `DATABASE_URL` e `RABBITMQ_URL` definidas
- **THEN** ele escuta na porta `8080`, usa nível de log `info` e tempo limite de encerramento de 10 segundos

#### Scenario: Porta customizada
- **WHEN** o serviço inicia com `HTTP_PORT=9090`
- **THEN** ele aceita conexões HTTP na porta `9090`

#### Scenario: Token local ausente
- **WHEN** o serviço inicia sem `LOCAL_WEBHOOK_TOKEN`
- **THEN** ele sobe normalmente e registra um aviso de que o webhook `local` rejeitará todas as requisições

### Requirement: Fail fast on invalid configuration
O serviço SHALL encerrar com código de saída diferente de zero e uma mensagem de log de erro identificando a variável problemática quando uma configuração obrigatória estiver ausente ou um valor for inválido.

#### Scenario: DATABASE_URL ausente
- **WHEN** o serviço inicia sem `DATABASE_URL`
- **THEN** ele encerra com código de saída diferente de zero e registra um erro mencionando `DATABASE_URL`

#### Scenario: RABBITMQ_URL ausente
- **WHEN** o serviço inicia sem `RABBITMQ_URL`
- **THEN** ele encerra com código de saída diferente de zero e registra um erro mencionando `RABBITMQ_URL`

#### Scenario: Valor inválido
- **WHEN** o serviço inicia com `HTTP_PORT=abc` ou `SHUTDOWN_TIMEOUT=xyz`
- **THEN** ele encerra com código de saída diferente de zero e registra um erro mencionando a variável inválida

### Requirement: Graceful shutdown
Ao receber `SIGINT` ou `SIGTERM`, o serviço SHALL:
1. Parar de aceitar novas conexões HTTP e aguardar as requisições em andamento por até `SHUTDOWN_TIMEOUT`.
2. Parar de consumir novas mensagens da fila e aguardar a mensagem em processamento terminar dentro do mesmo tempo limite.
3. Fechar a conexão com o RabbitMQ e o pool do banco.
4. Encerrar com código de saída zero.

#### Scenario: Encerramento com requisição em andamento
- **WHEN** o serviço recebe `SIGTERM` enquanto atende uma requisição que termina dentro do tempo limite
- **THEN** a requisição é concluída com sucesso e o processo encerra com código zero

#### Scenario: Encerramento com mensagem em processamento
- **WHEN** o serviço recebe `SIGTERM` enquanto o consumer grava uma task
- **THEN** a gravação termina, a mensagem é confirmada ao broker e o processo encerra com código zero

#### Scenario: Tempo limite excedido
- **WHEN** requisições ou mensagens em andamento não terminam dentro de `SHUTDOWN_TIMEOUT`
- **THEN** o serviço força o encerramento, registra um aviso e sai mesmo assim; mensagens não confirmadas voltam para a fila

## ADDED Requirements

### Requirement: Message broker connection at startup
O serviço SHALL conectar ao RabbitMQ na inicialização, tentando novamente por até 30 segundos, e SHALL declarar as exchanges e filas de que precisa antes de aceitar requisições. Se não conseguir conectar nesse período, SHALL encerrar com código de saída diferente de zero.

#### Scenario: Broker sobe depois do serviço
- **WHEN** o serviço inicia e o RabbitMQ só fica acessível alguns segundos depois, dentro de 30 segundos
- **THEN** o serviço conecta, registra sucesso e passa a aceitar requisições e consumir mensagens

#### Scenario: Broker nunca disponível
- **WHEN** o RabbitMQ permanece inacessível por mais de 30 segundos após o início
- **THEN** o serviço encerra com código de saída diferente de zero e registra um erro de conexão sem expor a senha

### Requirement: Message broker reconnection
Se a conexão com o RabbitMQ cair durante a execução, o serviço SHALL continuar rodando e tentar reconectar indefinidamente, com intervalo crescente limitado a 30 segundos. Enquanto estiver desconectado, o readiness responde `503` e os webhooks respondem `503`. Ao reconectar, o consumo SHALL ser retomado sem reiniciar o processo.

#### Scenario: Queda e retorno do broker
- **WHEN** o RabbitMQ é reiniciado com o serviço em execução
- **THEN** durante a queda `GET /readyz` responde `503`; depois que o broker volta, o serviço reconecta, `GET /readyz` volta a responder `200` e novos webhooks são processados
