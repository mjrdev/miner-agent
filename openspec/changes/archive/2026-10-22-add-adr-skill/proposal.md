# Proposal

## Why

As decisões arquiteturais do bootstrap do serviço (layout, roteador, logs, configuração, driver do banco, ambiente Docker, porta do Postgres) estão hoje espalhadas no `design.md` de uma change arquivada, que já ficou desatualizado (cita a porta 5432 em vez de 5433). Precisamos de um lugar durável e versionado para registrar *por que* cada escolha foi feita, e de uma forma padronizada e rápida de criar esses registros, para que novas decisões sejam documentadas no momento em que acontecem.

## What Changes

- Nova skill do Claude Code `adr` em `.claude/skills/adr/SKILL.md`, invocável por `/adr` ou automaticamente quando alguém pede para registrar uma decisão arquitetural. Ela numera, preenche e indexa ADRs, e também trata a substituição (supersede) de um ADR antigo.
- Novo diretório `docs/adr/` com:
  - `template.md`: modelo de ADR em português (estilo MADR enxuto: Status, Data, Contexto, Decisão, Alternativas consideradas, Consequências, Referências);
  - `README.md`: índice de todos os ADRs com número, título, status e data, e uma explicação curta do processo.
- ADRs retroativos documentando o que já foi feito, um por decisão:
  - `0001` uso de ADRs para registrar decisões;
  - `0002` layout de diretórios (`cmd/` + `internal/`);
  - `0003` roteamento com `net/http.ServeMux`;
  - `0004` logs estruturados com `log/slog`;
  - `0005` configuração por variáveis de ambiente;
  - `0006` PostgreSQL com `pgx/v5` e conexão com retry;
  - `0007` ambiente de desenvolvimento com Docker Compose e hot reload com `air`;
  - `0008` PostgreSQL 17 no compose com a porta 5433 no host.
- Link para `docs/adr/` no `README.md` da raiz.

Fora de escopo: design docs longos (RFCs), geração de site de documentação, validação automática de ADRs em CI e qualquer mudança no código Go.

## Capabilities

### New Capabilities
- `architecture-decisions`: como as decisões arquiteturais do projeto são registradas, numeradas, indexadas e substituídas em `docs/adr/`, incluindo a skill que automatiza a criação.

### Modified Capabilities
<!-- Nenhuma: as specs existentes (health-checks, local-dev-environment, service-lifecycle) não mudam de comportamento. -->

## Impact

- Arquivos novos: `.claude/skills/adr/SKILL.md`, `docs/adr/{README.md,template.md,0001-…0008-*.md}`.
- `README.md` da raiz ganha uma seção curta apontando para `docs/adr/`.
- Nenhuma dependência nova; nenhuma mudança em código, Docker ou specs existentes.
- A imagem de runtime não é afetada: ela só recebe o binário compilado. O `.dockerignore` exclui apenas os `*.md` da raiz, então `docs/` entra no contexto do estágio de build, sem efeito prático. A skill fica em `.claude/`, ao lado das skills do OpenSpec, e não entra no `skills-lock.json`, que lista só skills externas instaladas.
