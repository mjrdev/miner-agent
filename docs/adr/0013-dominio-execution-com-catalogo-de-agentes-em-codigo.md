# ADR-0013: Domínio execution com catálogo de agentes em código

**Status:** Aceito
**Data:** 2026-10-26

## Contexto

O próximo passo do `miner` é executar as tasks com um agente de IA dentro de um container: primeiro no host, depois em Jobs do Kubernetes. A change `add-task-execution-config` é a primeira de duas. Ela só entrega a configuração: cada task diz qual agente, modelo, esforço e runner usar, e o serviço valida essa combinação. O executor fica para a change `add-host-task-runner`.

Executar não é parte do domínio Task do [ADR-0009](0009-dominio-task-em-internal-task-com-subpacotes.md), mas o pacote `task` precisa do catálogo para validar a configuração. O padrão de componente plugável já existe na ingestão (`adapter.Adapter` + `Registry`, compostos em `cmd/api/main.go`).

## Decisão

- Criamos o domínio `internal/execution`, **folha** do grafo de dependências: ele não importa `internal/task`. O `task` importa o `execution`, e o `Service` recebe o catálogo em `task.NewService(repo, catalog)`.
- O contrato é a interface `Agent` (`Name()`, `Models()`), e `Model` tem `ID` e a lista ordenada de `Efforts` (vazia quando o modelo não aceita esforço). O `Catalog` indexa os agentes por nome, guarda os runners e expõe `Agent`, `HasRunner` e `Options` (a visão ordenada servida em `GET /execution-options`).
- O catálogo de cada agente é **estático, em código**, no subpacote do adapter. O primeiro é `internal/execution/claude`, com os aliases do CLI (`fable`, `opus`, `sonnet`) e os níveis de `--effort` (`low`, `medium`, `high`, `xhigh`, `max`).
- Runners são **só nomes** numa lista (hoje apenas `host`). A interface `Runner` nasce junto com a primeira implementação, na próxima change.
- A composição fica em `cmd/api/main.go`: `execution.NewCatalog([]execution.Agent{claude.New()}, []string{"host"})`.

Na próxima change, o `Agent` ganha a montagem do comando a partir de dados neutros (prompt, modelo, esforço), não de `task.Task`. O worker que junta task, agente e runner fica num subpacote de `execution` que importa os dois, sem ciclo.

## Alternativas consideradas

- **Agentes em `internal/task/agent/`**: execução não é parte do domínio Task, e o runner da próxima change também não caberia ali.
- **Interface-porta `ExecutionCatalog` com tipos próprios no `task`**: duplicaria `Model` sem ganho. O catálogo não tem dependências, e os testes montam um `Catalog` real com agentes fake (`tasktest.Catalog()`).
- **Catálogo por variável de ambiente ou no banco**: mais configuração e validação para algo que muda junto com o código do adapter.
- **IDs completos de modelo** (`claude-opus-5-5`): travariam o catálogo numa versão e exigiriam manutenção a cada lançamento.
- **Interface `Runner` já nesta change**: seria desenhada sem nenhuma implementação para validá-la.

## Consequências

### Positivas

- Adicionar um agente é registrar um novo adapter em `main.go`, sem mexer nas regras de task.
- O `execution` pode ser estendido pelo executor sem criar ciclo com o `task`.
- Os testes do `task` e do `httpapi` usam o catálogo real, sem mocks.

### Negativas

- Mudar modelos ou esforços exige alterar o código e fazer deploy.
- Os aliases (`opus`, `sonnet`) mudam de versão sem aviso, então dois runs da mesma task podem usar modelos diferentes. A próxima change registra no run o modelo efetivamente usado.
- Uma task em `ready` pode guardar um modelo que saiu do catálogo. O executor revalida antes de rodar, e um `PATCH` com outro modelo corrige.

## Referências

- `internal/execution/`, `internal/execution/claude/`, `internal/task/service.go`
- `openspec/changes/add-task-execution-config/design.md`
- [ADR-0009](0009-dominio-task-em-internal-task-com-subpacotes.md)
