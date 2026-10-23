# Tasks

## 1. Estrutura de ADRs

- [x] 1.1 Criar `docs/adr/template.md` (MADR enxuto em português: título `ADR-NNNN`, Status, Data, Contexto, Decisão, Alternativas consideradas, Consequências positivas/negativas, Referências opcional, com comentários-guia) e verificar que contém todas as seções exigidas pela spec
- [x] 1.2 Criar `docs/adr/README.md` com a explicação do processo (como criar via `/adr` ou copiando o modelo, valores de status, regra de substituição) e a tabela `| ADR | Título | Status | Data |` vazia, e verificar que o Markdown renderiza a tabela

## 2. Skill `adr`

- [x] 2.1 Escrever `.claude/skills/adr/SKILL.md` com frontmatter (`name: adr`, description de acionamento) e o fluxo do design (entender/perguntar, inspecionar o repo, numerar, gerar o slug, preencher o modelo, definir o status, substituir, atualizar e reconciliar o índice, reportar), mais o modelo mínimo para bootstrap, e verificar que o arquivo aparece como skill `adr` (frontmatter válido, `name` igual ao diretório)
- [x] 2.2 Verificar a lógica mecânica da skill com um teste a seco descrito no próprio `SKILL.md`: para os títulos "Registrar decisões com ADRs" e "Configuração por variáveis de ambiente", os slugs esperados são `registrar-decisoes-com-adrs` e `configuracao-por-variaveis-de-ambiente`, e com `0008` como maior número o próximo é `0009`

## 3. ADRs retroativos (seguindo a skill)

- [x] 3.1 Escrever `docs/adr/0001-registrar-decisoes-arquiteturais-com-adrs.md` (Aceito, 2026-10-22) e verificar a linha no índice
- [x] 3.2 Escrever `0002` sobre o layout `cmd/` + `internal/` (inclui `internal/logger`; alternativas: hexagonal, `pkg/`) conferindo com `find cmd internal`, e verificar a linha no índice
- [x] 3.3 Escrever `0003` sobre `net/http.ServeMux` e os middlewares (logging por fora do recover; alternativas: chi, gin/echo) conferindo com `internal/server/routes.go`, e verificar a linha no índice
- [x] 3.4 Escrever `0004` sobre `log/slog` em JSON (alternativas: zap, zerolog) conferindo com `internal/logger/logger.go`, e verificar a linha no índice
- [x] 3.5 Escrever `0005` sobre configuração por variáveis de ambiente (variáveis, padrões, fail fast; alternativas: viper, envconfig) conferindo com `internal/config/config.go`, e verificar a linha no índice
- [x] 3.6 Escrever `0006` sobre `pgx/v5` + `pgxpool`, retry de 30s e interface `Pinger` (alternativas: `database/sql` + `lib/pq`, ORM) conferindo com `internal/database/postgres.go`, e verificar a linha no índice
- [x] 3.7 Escrever `0007` sobre Docker Compose + `air` com polling, Dockerfile multi-stage, runtime distroless e tag `miner-api:dev` (alternativa: `docker compose watch`) conferindo com `Dockerfile`, `docker-compose.yml` e `.air.toml`, e verificar a linha no índice
- [x] 3.8 Escrever `0008` sobre `postgres:17-alpine`, volume `pgdata` e porta 5433 no host (motivo: 5432 ocupada; divergência em relação ao plano) conferindo com `docker-compose.yml` e `.env.example`, e verificar a linha no índice

## 4. Integração e verificação

- [x] 4.1 Adicionar ao `README.md` da raiz a seção "Decisões de arquitetura" com link para `docs/adr/` e verificar que o link relativo aponta para um arquivo existente
- [x] 4.2 Verificar a consistência do conjunto: os nomes de arquivo seguem `NNNN-slug.md` em sequência de 0001 a 0008, cada ADR tem todas as seções obrigatórias sem placeholders, os status são valores válidos, o índice tem exatamente 8 linhas em ordem com links que resolvem (checar com `ls` e `grep`), e nenhum ADR cita a porta 5432 como porta do host
