# ADR-0009: Domínio Task em `internal/task` com subpacotes

**Status:** Aceito
**Data:** 2026-10-22

## Contexto

O [ADR-0002](0002-layout-de-diretorios-com-cmd-e-internal.md) deixou para depois a decisão de onde ficariam entidades e regras quando surgisse um domínio. Com a change `add-task-ingestion` surge o primeiro: as tasks que alimentam o agente autônomo. Elas entram por webhook e fila, são gravadas no PostgreSQL e editadas por CRUD. As mesmas regras (donos dos campos, ciclo de vida do status, normalização de labels) precisam valer na ingestão e no CRUD, e devem ser testáveis sem banco nem broker.

## Decisão

Cada domínio vive em `internal/<domínio>/`. No pacote raiz ficam as entidades, as regras e o service. Os detalhes de infraestrutura ficam em subpacotes que dependem do pacote raiz, nunca o contrário:

- `internal/task`: `Task`, `Status` e suas transições, `ProjectCategory`, `Event`, os erros de domínio, a validação, o `Service` e a interface `Repository`.
- `internal/task/postgres`: implementação do `Repository` (queries do sqlc em `queries/` e código gerado em `sqlcdb/`).
- `internal/task/adapter`: contrato `Adapter` (`Verify`/`Parse`) e `Registry`; um subpacote por provider (`local/`).
- `internal/task/ingest`: topologia do RabbitMQ, publisher e consumer.
- `internal/task/httpapi`: handlers de `/webhooks`, `/tasks`, `/project-categories` e `/labels`.
- `internal/task/tasktest`: repositório em memória para testes.

O que não pertence a um domínio continua em pacotes próprios de `internal/` (`broker`, `database`, `server`…). A composição acontece em `cmd/api/main.go`.

## Alternativas consideradas

- **Pacotes por camada no topo** (`internal/handler/task.go`, `internal/repository/…`): espalha um único domínio por vários pacotes e incentiva dependências cruzadas entre eles.
- **Hexagonal completo** (`domain/`, `ports/`, `adapters/`): cerimônia demais para um domínio; a regra "subpacotes dependem do pacote raiz" já dá o isolamento necessário.
- **Categorias e labels como pacotes próprios**: hoje só existem em função das tasks; viram pacotes quando ganharem regras próprias.

## Consequências

### Positivas

- O service é testado com o repositório em memória, sem infraestrutura.
- Um provider novo é um subpacote de `adapter/`, sem tocar em fila, banco ou HTTP.
- Tudo o que pertence ao domínio Task fica num único diretório.

### Negativas

- O pacote `task` concentra entidades, validação e service; se crescer demais, será preciso dividi-lo.
- O `httpapi` depende do `*task.Service` concreto, não de uma interface. Os testes de handler usam o service real com o repositório em memória.

## Referências

- `internal/task/`, `cmd/api/main.go`
- [ADR-0002](0002-layout-de-diretorios-com-cmd-e-internal.md)
- `openspec/changes/add-task-ingestion/design.md` (decisão 1)
