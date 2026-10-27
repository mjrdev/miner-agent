# Spec Delta

## ADDED Requirements

### Requirement: Agent invocation
Cada agente SHALL definir como é executado sem interação a partir do prompt, do modelo e do esforço, e SHALL emitir os eventos de execução em JSON, um por linha. O esforço SHALL ser repassado só quando definido. O `claude` SHALL ser executado em modo não interativo, com o modelo e o esforço do run, saída em `stream-json` e sem pedir permissão para usar ferramentas.

#### Scenario: Comando do claude
- **WHEN** um run usa o agente `claude` com o modelo `sonnet` e o esforço `low`
- **THEN** o CLI do Claude Code é executado sem interação com `--model sonnet`, `--effort low` e `--output-format stream-json`, recebendo o prompt da task

#### Scenario: Modelo sem esforço
- **WHEN** um run usa um modelo sem níveis de esforço
- **THEN** o comando do agente não recebe nenhum esforço

### Requirement: Agent credential priority
Cada agente SHALL definir as credenciais que aceita e a ordem de prioridade entre elas. O `claude` SHALL aceitar um token OAuth da assinatura (`oauth`) e uma chave de API (`api_key`), nessa ordem de prioridade.

#### Scenario: Só a chave de API
- **WHEN** só a chave de API do `claude` está configurada
- **THEN** o run usa a chave de API e registra `auth_method` igual a `api_key`
