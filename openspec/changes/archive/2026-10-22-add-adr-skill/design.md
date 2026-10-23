# Design

## Context

Hoje as decisões do bootstrap estão só em `openspec/changes/archive/2026-10-22-bootstrap-go-http-service/design.md`. Esse texto descreve o *plano*, e a implementação divergiu em alguns pontos: porta 5433 no host, middleware de logging por fora do recover, pacote `internal/logger` separado e tag `miner-api:dev` fixa no compose. O repositório já tem skills do Claude Code em `.claude/skills/<nome>/SKILL.md`, com frontmatter `name`/`description`. Algumas são instaladas por `skills-lock.json` e linkadas a partir de `.agents/skills/`; as do OpenSpec ficam direto em `.claude/skills/`. Motivação em `proposal.md`; requisitos em `specs/architecture-decisions/spec.md`.

## Goals / Non-Goals

**Goals:**
- O modelo de ADR tem uma única fonte, usada tanto por pessoas quanto pela skill.
- A skill é determinística nas partes mecânicas (número, nome de arquivo, índice) e pergunta quando falta conteúdo.
- Os ADRs retroativos refletem o código atual, não o plano.

**Non-Goals:**
- Ferramentas externas (`adr-tools`, `log4brains`) ou scripts de geração.
- Lint ou validação automática dos ADRs.
- Reescrever os artefatos arquivados do OpenSpec. Eles continuam como histórico, e os ADRs passam a ser a referência das decisões.

## Decisions

### Formato: MADR enxuto, em português

Seções: `# ADR-NNNN: <Título>`, metadados (`**Status:**`, `**Data:**`, opcionalmente `**Substitui:**`/`**Substituído por:**`), `## Contexto`, `## Decisão`, `## Alternativas consideradas`, `## Consequências` (com subitens positivas/negativas) e `## Referências` opcional.

Alternativas avaliadas: o formato de Michael Nygard (Contexto/Decisão/Status/Consequências) não tem uma seção explícita de alternativas, que é justamente o que mais se perde com o tempo. O MADR completo tem seções demais (drivers, prós e contras por opção) para decisões pequenas. Os valores de status ficam em português para combinar com o resto da documentação.

### Modelo em `docs/adr/template.md`, não dentro da skill

A skill lê o modelo do repositório na hora de criar o ADR. Assim, quem escreve um ADR à mão e a skill usam o mesmo arquivo, e mudar o formato exige editar um lugar só. Alternativa: embutir o modelo no `SKILL.md`. Foi descartada porque os dois formatos acabariam divergindo.

### Skill em `.claude/skills/adr/SKILL.md`

- **Frontmatter:** `name: adr` e uma `description` que dispara em pedidos como "registrar decisão", "criar ADR" ou "documentar por que escolhemos X" (em português e inglês). Sem `allowed-tools` restritivo, porque a skill precisa de Read/Write/Edit e `ls`.
- **Fluxo:**
  1. Entender a decisão a partir dos argumentos e da conversa. Se faltar contexto, decisão ou alternativas, perguntar antes de escrever.
  2. Inspecionar o código e a documentação relevantes para embasar Contexto e Consequências no estado real.
  3. Calcular o número: o maior `NNNN` entre os arquivos `docs/adr/[0-9][0-9][0-9][0-9]-*.md`, mais 1, ou `0001` se não houver nenhum.
  4. Gerar o slug: minúsculas, sem acentos (á→a, ç→c…), qualquer outro caractere vira `-`, hifens repetidos colapsados e nas pontas removidos, com no máximo cerca de 60 caracteres.
  5. Preencher o `template.md`. Status padrão: `Aceito` quando a decisão já está no código, `Proposto` caso contrário. Data no formato `YYYY-MM-DD` do dia.
  6. Em caso de substituição, editar no ADR antigo só a linha de status e adicionar `**Substituído por:**`.
  7. Atualizar a tabela do `docs/adr/README.md`, mantendo a ordem numérica.
  8. Informar o caminho do arquivo criado.
- **Bootstrap:** se `docs/adr/` não existir, a skill cria o diretório, o `template.md` e o `README.md` a partir de um modelo mínimo descrito no próprio `SKILL.md`. É o único conteúdo de modelo dentro da skill, usado só nesse caso.

Nenhum slash command extra em `.claude/commands/`, porque as skills já são invocáveis por `/adr`.

### Índice em `docs/adr/README.md`

Primeiro um parágrafo curto (o que é um ADR, como criar um com `/adr` ou copiando o `template.md`, valores de status). Depois uma tabela `| ADR | Título | Status | Data |` com o link relativo `[0001](0001-....md)`. O `README.md` da raiz ganha uma seção "Decisões de arquitetura" que aponta para `docs/adr/`.

### ADRs retroativos: fontes e conteúdo

Cada ADR retroativo usa como fonte o `design.md` arquivado **e** o código atual. Em caso de conflito, vale o código, e o ADR explica a diferença. Data: `2026-10-22` (dia em que a change do bootstrap foi arquivada). Status: `Aceito`.

| Nº | Decisão | Fontes no repositório |
|----|---------|------------------------|
| 0001 | Registrar decisões arquiteturais com ADRs em `docs/adr/` | esta change |
| 0002 | Layout `cmd/` + `internal/` (sem `pkg/`, sem hexagonal), incluindo `internal/logger` | `cmd/api`, `internal/*`, `go.mod` |
| 0003 | Roteamento com `net/http.ServeMux` (padrões com método); middlewares próprios, com logging por fora do recover | `internal/server/routes.go`, `middleware.go` |
| 0004 | Logs JSON com `log/slog` | `internal/logger/logger.go` |
| 0005 | Configuração só por variáveis de ambiente, com parsing manual e fail fast | `internal/config/config.go` |
| 0006 | `pgx/v5` + `pgxpool`; retry de 30s na subida; interface `Pinger` para readiness | `internal/database/postgres.go`, `internal/handler/health.go` |
| 0007 | Dev com Docker Compose + `air` (polling), Dockerfile multi-stage e runtime distroless, tag `miner-api:dev` | `Dockerfile`, `docker-compose.yml`, `.air.toml`, `Makefile` |
| 0008 | `postgres:17-alpine` com volume nomeado e porta 5433 no host (5432 ocupada por outro projeto) | `docker-compose.yml`, `.env.example`, `README.md` |

## Risks / Trade-offs

- [Os ADRs retroativos podem racionalizar escolhas depois do fato] → cada um cita as alternativas que o design arquivado já listava, sem inventar debates que não existiram.
- [A skill pode criar ADRs rasos se for disparada à toa] → a description restringe o uso a decisões arquiteturais, e a skill pergunta quando falta conteúdo.
- [Numeração em conflito se duas branches criarem ADRs ao mesmo tempo] → aceitável num projeto pequeno; o conflito aparece no merge pelo nome do arquivo e no índice, e se resolve renumerando o mais novo.
- [O índice pode divergir dos arquivos se alguém editar à mão] → o README do índice pede para atualizar a tabela, e a skill reconcilia a tabela com os arquivos existentes sempre que roda.
