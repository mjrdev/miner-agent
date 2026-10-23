# Design

## Context

Repositório greenfield: só existem `openspec/`, `.claude/`, `.agents/` e `.tool-versions` (que hoje fixa apenas `nodejs`). Na máquina estão disponíveis Go 1.26.5, Docker 29 e Docker Compose v5. O repositório ainda não é um repositório git. Motivação e escopo em `proposal.md`; requisitos em `specs/`.

## Goals / Non-Goals

**Goals:**
- Layout de diretórios reconhecível por qualquer pessoa que trabalhe com Go (inspirado em `golang-standards/project-layout`, sem pastas vazias desnecessárias).
- Dependências mínimas: biblioteca padrão para HTTP, logs e config; apenas `pgx` como dependência externa.
- Código testável: handlers recebem dependências por interface, permitindo testes sem banco real.
- Ciclo de desenvolvimento rápido dentro do Docker (hot reload).

**Non-Goals:**
- Imagem de produção otimizada, CI/CD, observabilidade (métricas/tracing).
- Ferramenta de migrações executando no boot — apenas a pasta `migrations/` é criada.
- ORM, query builder ou camada de repositórios genérica.

## Decisions

### Layout de diretórios

```
miner/
├── cmd/api/main.go              # composição: config → logger → db → server → run
├── internal/
│   ├── config/config.go         # leitura/validação de env vars (+ config_test.go)
│   ├── database/postgres.go     # criação do pgxpool com retry; interface Pinger
│   ├── server/
│   │   ├── server.go            # http.Server, timeouts, Run com graceful shutdown
│   │   ├── routes.go            # registro de rotas no http.ServeMux
│   │   └── middleware.go        # request logging + recover
│   └── handler/
│       ├── health.go            # /healthz e /readyz
│       └── health_test.go
├── migrations/.gitkeep          # reservado para migrações SQL futuras
├── Dockerfile                   # multi-stage: dev (air) e build/runtime
├── docker-compose.yml
├── .air.toml
├── .env.example
├── .dockerignore / .gitignore
├── Makefile
└── README.md
```

`internal/` impede importação externa e é a convenção da comunidade. Alternativa considerada: layout "hexagonal" completo (`domain/`, `ports/`, `adapters/`) — rejeitado por ser cerimônia demais sem nenhuma regra de negócio ainda; pode evoluir quando houver domínio. `pkg/` não é criado porque não há código pensado para consumo externo.

Nome do módulo: `github.com/jrgomesdev/miner` (assumido; trocável facilmente com `go mod edit -module` antes de existir código importando).

### Roteamento: `net/http.ServeMux` (Go 1.22+)

O mux padrão já suporta padrões com método (`GET /healthz`) e retorna 404/405 automaticamente, cobrindo o requisito de rotas desconhecidas. Alternativas: `chi` (compatível com `net/http`, ótimo se precisarmos de grupos de rota/middlewares por subárvore) e `gin`/`echo` (APIs próprias, mais acoplamento). Começamos com stdlib; migrar para `chi` depois é barato porque os handlers continuam sendo `http.Handler`.

### Logs: `log/slog` com `JSONHandler`

Stdlib, estruturado, nível configurável via `slog.LevelVar`. Alternativas `zap`/`zerolog` só se justificariam com requisitos de performance que não existem.

### Configuração: env vars com parsing manual em `internal/config`

Struct `Config` populada por `os.Getenv` + `strconv`/`time.ParseDuration`, retornando erro que nomeia a variável. Evita dependências como `envconfig`/`viper` para cinco variáveis. `DATABASE_URL` no formato `postgres://user:pass@host:5432/db?sslmode=disable`.

### Banco: `pgx/v5` com `pgxpool`

Driver PostgreSQL mais usado e mantido em Go, com pool nativo. Na inicialização, `database.Connect(ctx, url)` cria o pool e faz `Ping` em loop com backoff (1s) até 30s. O handler de readiness depende de uma interface mínima `Pinger { Ping(ctx) error }`, satisfeita por `*pgxpool.Pool` e por um fake nos testes. Erros de conexão são logados sem a URL (pgx não inclui a senha na mensagem; ainda assim não logamos `DATABASE_URL`). Alternativa `database/sql` + `lib/pq`: `lib/pq` está em modo manutenção.

### Servidor e graceful shutdown

`http.Server` com `ReadHeaderTimeout`, `ReadTimeout`, `WriteTimeout` e `IdleTimeout` definidos (evita Slowloris). `main` usa `signal.NotifyContext(ctx, SIGINT, SIGTERM)`; ao cancelar, chama `srv.Shutdown(ctx com SHUTDOWN_TIMEOUT)`, depois `pool.Close()`. Se `Shutdown` retornar `context.DeadlineExceeded`, loga aviso e chama `srv.Close()`. `main` delega a uma função `run(ctx) error` para que o código seja testável e `os.Exit(1)` aconteça em um único lugar.

### Middlewares

Encadeamento simples (`recoverer(logger(mux))`): `logger` envolve o `ResponseWriter` para capturar status e mede duração; `recoverer` usa `recover()`, loga o panic com stack e responde 500. Sem dependência externa.

### Docker

- `Dockerfile` multi-stage com base `golang:1.26-alpine`:
  - estágio `dev`: instala `air` (`go install github.com/air-verse/air@<versão fixa>`), `WORKDIR /app`, `CMD ["air", "-c", ".air.toml"]`; o código é montado via bind mount.
  - estágio `build`: `go build -trimpath -ldflags="-s -w" -o /bin/api ./cmd/api`.
  - estágio `runtime`: `gcr.io/distroless/static-debian12:nonroot` com o binário — não usado no compose agora, mas deixa o caminho pronto e serve para validar que a build compila.
- `docker-compose.yml`:
  - `postgres`: `postgres:17-alpine`, env `POSTGRES_USER/PASSWORD/DB` (padrão `miner`/`miner`/`miner`), volume nomeado `pgdata`, healthcheck `pg_isready`, porta `${POSTGRES_PORT:-5432}:5432`.
  - `api`: `build.target: dev`, bind mount `.:/app`, volumes nomeados para cache de módulos e build (`gomod`, `gocache`) para acelerar rebuilds, `env_file: [{path: .env, required: false}]`, `environment` com padrões (`DATABASE_URL=postgres://miner:miner@postgres:5432/miner?sslmode=disable`), porta `${HTTP_PORT:-8080}:${HTTP_PORT:-8080}`, `depends_on: postgres: condition: service_healthy`.
- `.air.toml`: observa `.go`, exclui `tmp/`, `vendor/`, `migrations/`; usa polling (`poll = true`) porque eventos inotify não propagam de forma confiável em bind mounts no WSL2/Docker Desktop.

Alternativa considerada para hot reload: `docker compose watch` com `sync+restart` — requer rebuild/restart do container e é menos imediato que `air`; pode ser adicionado depois.

### Makefile

Alvos: `up` (`docker compose up --build`), `down`, `reset` (`down -v`), `logs`, `ps`, `psql` (abre shell no banco), `test` (`go test ./...`), `fmt` (`gofmt`/`go vet`), `lint` (`golangci-lint run` se instalado), `build` (`docker build --target runtime`).

### Testes

- `internal/config`: tabela de casos para padrões, valores customizados e erros por variável.
- `internal/handler`: `httptest` com `Pinger` fake (sucesso, erro, timeout) verificando status e corpo JSON.
- `internal/server`: teste de middleware recover (handler que dá panic → 500) e de 404/405 nas rotas.
- Verificação end-to-end manual/scriptada via `docker compose up` + `curl` nos endpoints.

## Risks / Trade-offs

- [Polling do `air` consome CPU] → intervalo de ~500ms e exclusão de diretórios grandes; aceitável para dev.
- [Bind mount no WSL2 é lento se o repo estiver em `/mnt/c`] → o repo está em `/home` (filesystem Linux), documentar no README.
- [Credenciais fixas `miner/miner` no compose] → apenas desenvolvimento local; documentar que não servem para outros ambientes e permitir sobrescrita via `.env`.
- [Porta 5432 pode conflitar com Postgres local] → porta do host configurável por `POSTGRES_PORT`.
- [`ServeMux` pode ficar limitado com crescimento de rotas] → handlers padrão `http.Handler` permitem trocar por `chi` sem reescrever handlers.

## Open Questions

- Ferramenta de migrações futura (`golang-migrate`, `goose` ou `tern`) — decidir quando a primeira tabela for criada; a pasta `migrations/` já fica reservada.
- Nome definitivo do módulo Go, caso o repositório remoto não seja `github.com/jrgomesdev/miner`.
