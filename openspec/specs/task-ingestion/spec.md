# task-ingestion Specification

## Purpose

Recebe eventos de tickets vindos de providers externos (Jira, GitHub, Trello…) ou da origem genérica `local` por webhook, enfileira-os e os transforma de forma assíncrona e idempotente em tasks persistidas, por meio de um contrato de adapter comum a todos os providers.

## Requirements

### Requirement: Webhook endpoint per provider
O serviço SHALL expor `POST /webhooks/{provider}` para cada provider registrado. Para um provider não registrado, SHALL responder `404 Not Found` sem enfileirar nada.

#### Scenario: Provider registrado
- **WHEN** um cliente envia `POST /webhooks/local` autenticado e com corpo JSON válido
- **THEN** o serviço aceita a requisição e não responde `404`

#### Scenario: Provider desconhecido
- **WHEN** um cliente envia `POST /webhooks/desconhecido`
- **THEN** o serviço responde `404 Not Found` e nenhuma mensagem é publicada na fila

### Requirement: Webhook authentication by the provider adapter
Antes de enfileirar, o serviço SHALL verificar a autenticidade da requisição com a regra do adapter do provider. Se a verificação falhar, SHALL responder `401 Unauthorized` sem enfileirar e sem revelar o segredo esperado.

#### Scenario: Credencial inválida
- **WHEN** um cliente envia `POST /webhooks/local` com o token errado ou sem token
- **THEN** o serviço responde `401 Unauthorized` e nenhuma mensagem é publicada na fila

### Requirement: Webhook payload limits
O serviço SHALL rejeitar com `413 Payload Too Large` os corpos de webhook maiores que 1 MiB e com `400 Bad Request` os corpos vazios ou que não sejam JSON válido, sem enfileirar nada.

#### Scenario: Corpo muito grande
- **WHEN** um cliente autenticado envia um corpo de 2 MiB para `POST /webhooks/local`
- **THEN** o serviço responde `413 Payload Too Large` e nada é publicado

#### Scenario: Corpo não é JSON
- **WHEN** um cliente autenticado envia `POST /webhooks/local` com o corpo `isto não é json`
- **THEN** o serviço responde `400 Bad Request` e nada é publicado

### Requirement: Asynchronous acceptance
Para um webhook autenticado e válido, o serviço SHALL publicar uma mensagem com o provider, um identificador de entrega e o corpo bruto. Só depois de o broker confirmar a publicação, SHALL responder `202 Accepted` com o corpo `{"delivery_id":"<id>"}`. Se a publicação não for confirmada, SHALL responder `503 Service Unavailable`, para que o provider reenvie.

#### Scenario: Publicação confirmada
- **WHEN** um cliente autenticado envia um webhook válido e o broker confirma a publicação
- **THEN** o serviço responde `202 Accepted` com um `delivery_id` não vazio

#### Scenario: Broker indisponível
- **WHEN** um cliente autenticado envia um webhook válido e o broker não confirma a publicação
- **THEN** o serviço responde `503 Service Unavailable` e não responde `202`

### Requirement: Common adapter event
Cada adapter SHALL converter o corpo bruto do seu provider em um evento normalizado com: tipo (`created`, `updated` ou `closed`), identificador externo, URL externa opcional, título e descrição. Esse evento SHALL ser a única entrada da gravação de tasks, independentemente do provider.

#### Scenario: Evento normalizado do adapter local
- **WHEN** o consumer processa uma mensagem do provider `local` com `title` e `description`
- **THEN** a gravação recebe um evento do tipo `created` com esses título e descrição, sem depender do formato bruto

### Requirement: Local adapter
O serviço SHALL incluir o provider `local`, autenticado pelo header `X-Webhook-Token` comparado com `LOCAL_WEBHOOK_TOKEN`. O corpo é um objeto JSON com `title` e `description` obrigatórios e `type`, `external_id` e `url` opcionais. Sem `type`, assume-se `created`. Sem `external_id`, usa-se o `delivery_id` da entrega.

#### Scenario: Task local criada pelo webhook
- **WHEN** um cliente envia `POST /webhooks/local` com o token correto e o corpo `{"title":"Corrigir login","description":"Implemente ... Critérios de aceite: ..."}`
- **THEN** o serviço responde `202` e, após o processamento, existe uma task com `provider` `local`, `external_id` igual ao `delivery_id`, esse título e essa descrição

#### Scenario: Token não configurado
- **WHEN** `LOCAL_WEBHOOK_TOKEN` está vazio e um cliente envia `POST /webhooks/local` com qualquer token
- **THEN** o serviço responde `401 Unauthorized`

### Requirement: Task creation from created events
Ao processar um evento `created`, o serviço SHALL criar uma task com status `draft`, sem instruções adicionais, labels nem categoria, guardando o corpo bruto original. Se já existir uma task com o mesmo `(provider, external_id)`, SHALL manter a existente inalterada.

#### Scenario: Primeiro evento cria a task
- **WHEN** o consumer processa um evento `created` do provider `local` com `external_id` `abc`
- **THEN** existe exatamente uma task `(local, abc)` com status `draft`

#### Scenario: Reentrega não duplica nem sobrescreve
- **WHEN** a task `(local, abc)` já existe, foi editada pelo CRUD e o consumer processa outro evento `created` para `(local, abc)`
- **THEN** continua existindo exatamente uma task `(local, abc)` e as edições feitas pelo CRUD são preservadas

### Requirement: Unsupported event types are acknowledged
O serviço SHALL confirmar, sem alterar nenhuma task, os eventos dos tipos `updated` e `closed`, registrando em log que foram ignorados.

#### Scenario: Evento updated ignorado
- **WHEN** o consumer processa um evento `local` com `"type":"updated"`
- **THEN** nenhuma task é criada ou alterada, a mensagem sai da fila e um log informa que o evento foi ignorado

### Requirement: Failed messages go to a dead-letter queue
Uma mensagem cujo corpo o adapter não consegue converter em evento (por exemplo, sem `title`) SHALL ir direto para a dead-letter queue. Uma mensagem que falha por erro transitório SHALL ser reentregue e, depois de 5 entregas sem sucesso, também SHALL ir para a dead-letter queue. Nos dois casos, o serviço SHALL registrar o motivo em log.

#### Scenario: Payload inválido
- **WHEN** um cliente envia `POST /webhooks/local` autenticado com o corpo `{"description":"sem título"}`
- **THEN** o serviço responde `202`, nenhuma task é criada e a mensagem termina na dead-letter queue

#### Scenario: Banco indisponível durante o consumo
- **WHEN** o PostgreSQL fica indisponível enquanto o consumer processa uma mensagem válida
- **THEN** a mensagem é reentregue e, se o banco voltar antes da quinta entrega, a task é criada

### Requirement: At-least-once processing
O serviço SHALL confirmar uma mensagem ao broker somente depois que o efeito dela no banco estiver gravado (ou depois que ela for deliberadamente ignorada). Assim, nenhuma mensagem aceita com `202` se perde por uma interrupção do serviço.

#### Scenario: Serviço reinicia durante o processamento
- **WHEN** o serviço é encerrado depois de receber uma mensagem e antes de gravar a task
- **THEN** a mensagem volta para a fila e a task é criada quando o serviço voltar
