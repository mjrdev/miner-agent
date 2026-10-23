# Proposal

## Why

O projeto `miner` ainda não tem código: existe apenas a estrutura do OpenSpec. Precisamos de uma base de serviço HTTP em Go, organizada segundo os padrões de mercado, que rode localmente com Docker e já tenha um PostgreSQL disponível, para que as próximas funcionalidades sejam construídas sobre uma fundação consistente em vez de cada uma reinventar estrutura, configuração e infraestrutura.

## What Changes

- Novo módulo Go (`go.mod`) com layout padrão de mercado: `cmd/` para o binário, `internal/` para código da aplicação (config, servidor HTTP, handlers, acesso ao banco), `migrations/` para futuras migrações SQL.
- Servidor HTTP com endpoints de saúde: liveness (`GET /healthz`) e readiness (`GET /readyz`, que verifica a conexão com o PostgreSQL).
- Configuração via variáveis de ambiente, com valores padrão para desenvolvimento e um `.env.example` documentado.
- Logs estruturados em JSON e encerramento gracioso (graceful shutdown) ao receber `SIGINT`/`SIGTERM`.
- Pool de conexões com PostgreSQL inicializado na subida do serviço, pronto para uso por funcionalidades futuras (nenhuma tabela de domínio nesta mudança).
- Ambiente de desenvolvimento com Docker: `Dockerfile` multi-stage (estágio de dev com hot reload) e `docker-compose.yml` com os serviços `api` e `postgres` (volume persistente e healthcheck).
- `Makefile` com atalhos (`up`, `down`, `logs`, `test`, `lint`, `fmt`) e `README.md` explicando como rodar.

Fora de escopo: deploy/produção, CI/CD, autenticação, endpoints de negócio, tabelas de domínio e ferramenta de migração executando migrações reais.

## Capabilities

### New Capabilities
- `health-checks`: endpoints HTTP que informam se o serviço está vivo e se está pronto para atender (incluindo a dependência do banco).
- `service-lifecycle`: como o serviço é configurado, inicia, registra logs e encerra de forma graciosa.
- `local-dev-environment`: como o serviço e o PostgreSQL sobem localmente via Docker Compose para desenvolvimento.

### Modified Capabilities
<!-- Nenhuma: não há specs existentes no projeto. -->

## Impact

- Código novo na raiz do repositório (`cmd/`, `internal/`, `migrations/`, `Dockerfile`, `docker-compose.yml`, `Makefile`, `.env.example`, `.dockerignore`, `.gitignore`, `README.md`).
- Novas dependências Go: driver/pool PostgreSQL (`github.com/jackc/pgx/v5`); demais funcionalidades usam a biblioteca padrão.
- Ferramentas de desenvolvimento: Docker e Docker Compose (já disponíveis na máquina), `air` para hot reload dentro do container.
- Portas locais usadas: `8080` (API) e `5432` (PostgreSQL), configuráveis.
