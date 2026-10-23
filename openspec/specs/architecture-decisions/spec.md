# architecture-decisions Specification

## Purpose

Garante que as decisões arquiteturais do projeto fiquem registradas de forma durável, padronizada e rastreável em `docs/adr/`, e que criar um novo registro seja um passo rápido e consistente.

## Requirements

### Requirement: ADR location and naming
Cada decisão arquitetural MUST ser registrada em um arquivo próprio em `docs/adr/`, nomeado `NNNN-<titulo-em-kebab-case>.md`, onde `NNNN` é um número sequencial de 4 dígitos que nunca é reutilizado.

#### Scenario: Primeiro ADR
- **WHEN** `docs/adr/` não contém nenhum ADR numerado e um novo ADR é criado com o título "Registrar decisões com ADRs"
- **THEN** o arquivo criado é `docs/adr/0001-registrar-decisoes-com-adrs.md`

#### Scenario: Próximo número
- **WHEN** o maior ADR existente é `0008-...` e um novo ADR é criado
- **THEN** o novo arquivo recebe o número `0009`, mesmo que algum ADR anterior tenha sido substituído

#### Scenario: Título com acentos e símbolos
- **WHEN** o título é "Configuração por variáveis de ambiente"
- **THEN** o nome do arquivo usa apenas letras minúsculas sem acento, dígitos e hifens, por exemplo `0005-configuracao-por-variaveis-de-ambiente.md`

### Requirement: ADR content structure
Todo ADR MUST seguir o modelo de `docs/adr/template.md`, escrito em português, contendo título com número, Status, Data, Contexto, Decisão, Alternativas consideradas e Consequências; Referências é opcional.

#### Scenario: ADR completo
- **WHEN** um ADR é criado
- **THEN** ele contém todas as seções obrigatórias preenchidas, sem comentários ou placeholders do modelo

#### Scenario: Alternativas registradas
- **WHEN** a decisão teve opções descartadas
- **THEN** a seção Alternativas consideradas lista cada opção com o motivo de não ter sido escolhida

### Requirement: ADR status lifecycle
O Status de um ADR MUST ser um dos valores `Proposto`, `Aceito`, `Rejeitado`, `Obsoleto` ou `Substituído por ADR-NNNN`.

#### Scenario: Decisão já implementada
- **WHEN** um ADR documenta uma decisão que já está em vigor no código
- **THEN** seu status é `Aceito`

#### Scenario: Decisão em discussão
- **WHEN** um ADR é criado para uma decisão ainda não acordada
- **THEN** seu status é `Proposto`

### Requirement: Superseding a decision
Quando uma nova decisão substituir uma anterior, o novo ADR MUST referenciar o antigo, e o ADR antigo MUST ter o status alterado para `Substituído por ADR-NNNN`, mantendo intacto o restante de seu conteúdo.

#### Scenario: Troca de decisão
- **WHEN** o ADR `0009` substitui o ADR `0003`
- **THEN** o `0009` cita `ADR-0003` em Contexto ou Referências, o status do `0003` passa a ser `Substituído por ADR-0009` e as demais seções do `0003` permanecem inalteradas

### Requirement: ADR index
`docs/adr/README.md` MUST listar todos os ADRs em ordem numérica com número, título (com link para o arquivo), status e data, e MUST ser atualizado sempre que um ADR for criado ou mudar de status.

#### Scenario: Novo ADR aparece no índice
- **WHEN** um ADR é criado
- **THEN** o índice ganha uma linha para ele com link relativo funcional

#### Scenario: Status refletido no índice
- **WHEN** o status de um ADR muda
- **THEN** a linha correspondente no índice mostra o novo status

### Requirement: ADR creation skill
O repositório MUST oferecer uma skill do Claude Code chamada `adr`, invocável por `/adr <título ou descrição>` ou acionada quando alguém pedir para registrar uma decisão arquitetural, que cria o ADR e atualiza o índice conforme os requisitos acima.

#### Scenario: Criação via comando
- **WHEN** uma pessoa executa `/adr usar chi como roteador`
- **THEN** a skill cria o próximo ADR numerado a partir do modelo, preenchido com o contexto disponível, e adiciona a linha correspondente ao índice

#### Scenario: Informação insuficiente
- **WHEN** a skill não consegue determinar o contexto ou a decisão a partir do pedido e do repositório
- **THEN** ela pergunta à pessoa antes de escrever o arquivo, em vez de inventar o conteúdo

#### Scenario: Substituição via skill
- **WHEN** a pessoa pede um ADR que substitui uma decisão existente
- **THEN** a skill cria o novo ADR e atualiza o status do antigo e o índice conforme o requisito de substituição

### Requirement: Existing decisions documented
As decisões arquiteturais já em vigor no projeto MUST estar registradas como ADRs com status `Aceito`, e cada uma MUST descrever o estado real do repositório, não o planejado.

#### Scenario: Divergência entre plano e implementação
- **WHEN** a implementação diverge do que foi planejado (por exemplo, a porta 5433 do Postgres no host em vez de 5432)
- **THEN** o ADR registra o valor implementado e explica o motivo da divergência
