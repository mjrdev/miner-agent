# miner

Serviço HTTP em Go com PostgreSQL, preparado para desenvolvimento local com Docker.

## Pré-requisitos

- Docker com Docker Compose v2+
- `make`
- Go 1.26 (opcional, só para rodar testes fora do container)

> **WSL2:** mantenha o repositório no filesystem Linux (ex.: `~/dev/miner`), não em `/mnt/c/...`. Bind mounts a partir do disco do Windows deixam o hot reload e o build muito lentos.

## Como rodar

```bash
make up        # sobe api + postgres em background
make logs      # acompanha os logs da api
```

A API fica em <http://localhost:8080>. Ela só inicia depois que o PostgreSQL passa no healthcheck.

O container da API roda com [air](https://github.com/air-verse/air): ao salvar qualquer arquivo `.go`, o serviço é recompilado e reiniciado automaticamente.

| Comando      | O que faz                                          |
|--------------|----------------------------------------------------|
| `make up`    | Builda e sobe a stack                              |
| `make down`  | Para a stack mantendo os dados do banco            |
| `make reset` | Para a stack e **apaga** os dados do banco (e o cache do Go) |
| `make logs`  | Segue os logs da API                               |
| `make ps`    | Status dos containers                              |
| `make psql`  | Abre um shell `psql` no banco                      |
| `make test`  | Roda `go test ./...`                               |
| `make fmt`   | `gofmt` + `go vet`                                 |
| `make lint`  | `golangci-lint run` (precisa estar instalado)      |
| `make build` | Builda a imagem de runtime (`miner-api`)           |

## Endpoints

| Método | Caminho    | Resposta                                                                   |
|--------|------------|----------------------------------------------------------------------------|
| GET    | `/healthz` | `200 {"status":"ok"}` enquanto o processo estiver de pé                     |
| GET    | `/readyz`  | `200 {"status":"ready","checks":{"database":"ok"}}` ou `503` se o banco não responder em 2s |

```bash
curl -i localhost:8080/healthz
curl -i localhost:8080/readyz
```

## Configuração

Os valores padrão já funcionam. Para sobrescrever, copie `.env.example` para `.env` (não versionado) e ajuste:

| Variável           | Padrão                                                   | Descrição                                   |
|--------------------|----------------------------------------------------------|---------------------------------------------|
| `APP_ENV`          | `development`                                            | Nome do ambiente, aparece nos logs          |
| `HTTP_PORT`        | `8080`                                                   | Porta da API (no container e no host)       |
| `LOG_LEVEL`        | `info`                                                   | `debug`, `info`, `warn` ou `error`          |
| `SHUTDOWN_TIMEOUT` | `10s`                                                    | Tempo máximo para encerramento gracioso     |
| `DATABASE_URL`     | `postgres://miner:miner@postgres:5432/miner?sslmode=disable` | Conexão com o banco (obrigatória fora do compose) |
| `POSTGRES_USER`    | `miner`                                                  | Usuário do banco                            |
| `POSTGRES_PASSWORD`| `miner`                                                  | Senha do banco                              |
| `POSTGRES_DB`      | `miner`                                                  | Nome do banco                               |
| `POSTGRES_PORT`    | `5433`                                                   | Porta do banco **no host** (5433 evita conflito com um Postgres local na 5432) |

Para conectar um cliente externo (DBeaver, psql local): `postgres://miner:miner@localhost:5433/miner`.

> As credenciais `miner/miner` servem **apenas para desenvolvimento local**. Não as use em nenhum outro ambiente.

## Banco de dados

Os dados ficam no volume `miner_pgdata` e sobrevivem a `make down`. Para começar do zero (isso também remove os volumes de cache do Go, então o próximo build demora um pouco mais):

```bash
make reset && make up
```

A pasta `migrations/` está reservada para migrações SQL futuras.

## Estrutura

```
cmd/api/            ponto de entrada do binário
internal/config/    leitura e validação das variáveis de ambiente
internal/logger/    logger JSON (slog)
internal/database/  pool de conexões PostgreSQL (pgx)
internal/handler/   handlers HTTP
internal/server/    rotas, middlewares e servidor com graceful shutdown
migrations/         migrações SQL (futuro)
docs/adr/           decisões de arquitetura (ADRs)
```

## Decisões de arquitetura

O porquê de cada escolha técnica (layout, roteador, logs, configuração, banco, ambiente Docker) está registrado em ADRs em [`docs/adr/`](docs/adr/README.md). Para registrar uma decisão nova, use `/adr <decisão>` no Claude Code ou copie [`docs/adr/template.md`](docs/adr/template.md).
