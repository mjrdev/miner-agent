# Architecture Decision Records (ADRs)

Cada arquivo nesta pasta registra **uma** decisão arquitetural do projeto: o contexto em que foi tomada, o que foi decidido, as alternativas descartadas e as consequências. O objetivo é que qualquer pessoa entenda *por que* o sistema é como é sem depender da memória de quem decidiu.

## Como criar um ADR

- **Com o Claude Code:** `/adr <título ou descrição da decisão>`. A skill numera, preenche o modelo e atualiza esta tabela.
- **À mão:** copie [`template.md`](template.md) para `NNNN-titulo-em-kebab-case.md`, usando o próximo número livre (4 dígitos, nunca reutilizado), preencha todas as seções, apague os comentários do modelo e adicione uma linha na tabela abaixo.

Nomes de arquivo usam só minúsculas sem acento, dígitos e hifens (ex.: `0005-configuracao-por-variaveis-de-ambiente.md`).

## Status

| Status | Significado |
|--------|-------------|
| `Proposto` | Em discussão, ainda não acordado |
| `Aceito` | Em vigor |
| `Rejeitado` | Avaliado e descartado |
| `Obsoleto` | Deixou de se aplicar, sem substituto |
| `Substituído por ADR-NNNN` | Trocado por uma decisão mais nova |

ADRs aceitos não são reescritos. Para mudar uma decisão, crie um ADR novo que cite o antigo e altere **apenas** a linha de status do antigo para `Substituído por ADR-NNNN`, adicionando `**Substituído por:**` com o link.

## Índice

| ADR | Título | Status | Data |
|-----|--------|--------|------|
| [0001](0001-registrar-decisoes-arquiteturais-com-adrs.md) | Registrar decisões arquiteturais com ADRs | Aceito | 2026-10-22 |
| [0002](0002-layout-de-diretorios-com-cmd-e-internal.md) | Layout de diretórios com `cmd/` e `internal/` | Aceito | 2026-10-22 |
| [0003](0003-roteamento-com-net-http-servemux-e-middlewares-proprios.md) | Roteamento com `net/http.ServeMux` e middlewares próprios | Aceito | 2026-10-22 |
| [0004](0004-logs-estruturados-em-json-com-log-slog.md) | Logs estruturados em JSON com `log/slog` | Aceito | 2026-10-22 |
| [0005](0005-configuracao-por-variaveis-de-ambiente.md) | Configuração por variáveis de ambiente | Aceito | 2026-10-22 |
| [0006](0006-postgresql-com-pgx-v5-e-conexao-com-retry.md) | PostgreSQL com `pgx/v5` e conexão com retry | Aceito | 2026-10-22 |
| [0007](0007-ambiente-de-desenvolvimento-com-docker-compose-e-hot-reload.md) | Ambiente de desenvolvimento com Docker Compose e hot reload com `air` | Aceito | 2026-10-22 |
| [0008](0008-postgresql-17-no-compose-com-a-porta-5433-no-host.md) | PostgreSQL 17 no compose com a porta 5433 no host | Aceito | 2026-10-22 |
| [0009](0009-dominio-task-em-internal-task-com-subpacotes.md) | Domínio Task em `internal/task` com subpacotes | Aceito | 2026-10-22 |
| [0010](0010-rabbitmq-com-quorum-queue-e-dlq-para-a-ingestao-de-tasks.md) | RabbitMQ com quorum queue e DLQ para a ingestão de tasks | Aceito | 2026-10-22 |
| [0011](0011-acesso-ao-banco-com-codigo-gerado-pelo-sqlc.md) | Acesso ao banco com código gerado pelo sqlc | Aceito | 2026-10-22 |
| [0012](0012-migracoes-com-goose-e-servico-migrate-no-compose.md) | Migrações com goose e serviço `migrate` no compose | Aceito | 2026-10-22 |
| [0013](0013-dominio-execution-com-catalogo-de-agentes-em-codigo.md) | Domínio execution com catálogo de agentes em código | Aceito | 2026-10-26 |
