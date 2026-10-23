# Tasks

## 1. Estrutura do projeto

- [x] 1.1 Inicializar o módulo com `go mod init github.com/jrgomesdev/miner` (diretiva `go 1.26`), adicionar `golang 1.26.5` ao `.tool-versions` e verificar que `go env GOMOD` aponta para o `go.mod` da raiz
- [x] 1.2 Criar a árvore de diretórios do design (`cmd/api/`, `internal/{config,database,server,handler}/`, `migrations/.gitkeep`) e `.gitignore` (`tmp/`, `.env`, binários) e verificar com `find cmd internal migrations`
- [x] 1.3 Adicionar a dependência `github.com/jackc/pgx/v5` e verificar que `go mod tidy` termina sem erro e o `go.sum` é gerado

## 2. Configuração e logging

- [x] 2.1 Implementar `internal/config` (struct `Config`, `Load()` lendo `APP_ENV`, `HTTP_PORT`, `LOG_LEVEL`, `DATABASE_URL`, `SHUTDOWN_TIMEOUT` com padrões e erros que nomeiam a variável) e verificar com testes de tabela em `config_test.go` (padrões, porta customizada, `DATABASE_URL` ausente, `HTTP_PORT=abc`, `SHUTDOWN_TIMEOUT=xyz`) passando em `go test ./internal/config/...`
- [x] 2.2 Implementar a criação do logger `slog` JSON com nível vindo de `LOG_LEVEL` e verificar com teste que, em `warn`, mensagens `info` não são escritas no buffer

## 3. Banco de dados

- [x] 3.1 Implementar `internal/database` com `Connect(ctx, url)` criando `pgxpool` e fazendo `Ping` com retry (1s) por até 30s, sem logar a URL, e a interface `Pinger`; verificar com teste que um contexto/URL inacessível retorna erro após o tempo limite (usando tempo limite reduzido injetável) e `go vet ./...` limpo

## 4. HTTP: handlers, rotas e middlewares

- [x] 4.1 Implementar `internal/handler/health.go` (`GET /healthz` → 200 `{"status":"ok"}`; `GET /readyz` com timeout de 2s no `Ping` → 200 `{"status":"ready","checks":{"database":"ok"}}` ou 503 `{"status":"not_ready","checks":{"database":"unavailable"}}`) e verificar com `health_test.go` usando `httptest` e `Pinger` fake para sucesso, erro e timeout, incluindo `Content-Type: application/json`
- [x] 4.2 Implementar `internal/server/middleware.go` (logging de método, caminho, status e duração; recover que responde 500 e loga o panic) e verificar com testes que um handler em panic resulta em 500 e que a linha de log contém os campos esperados
- [x] 4.3 Implementar `internal/server/routes.go` com `http.ServeMux` e padrões com método, e verificar com testes que `GET /nao-existe` → 404 e `POST /healthz` → 405
- [x] 4.4 Implementar `internal/server/server.go` (`http.Server` com timeouts de leitura/escrita/idle e `Run(ctx)` com `Shutdown` limitado por `SHUTDOWN_TIMEOUT`, fallback para `Close` com aviso) e verificar com teste que uma requisição lenta em andamento termina com sucesso após o cancelamento do contexto

## 5. Ponto de entrada

- [x] 5.1 Implementar `cmd/api/main.go` com `run(ctx) error` (config → logger → database → server) e `signal.NotifyContext` para `SIGINT`/`SIGTERM`, fechando o pool no encerramento; verificar que `go build ./...` compila e que `go run ./cmd/api` sem `DATABASE_URL` sai com código ≠ 0 e loga erro mencionando `DATABASE_URL`

## 6. Ambiente Docker de desenvolvimento

- [x] 6.1 Criar `Dockerfile` multi-stage (`dev` com `air` em versão fixa, `build`, `runtime` distroless nonroot) e `.dockerignore`, e verificar que `docker build --target runtime -t miner-api .` e `docker build --target dev .` concluem com sucesso
- [x] 6.2 Criar `.air.toml` (observando `.go`, polling habilitado, exclusões de `tmp/`, `vendor/`, `migrations/`) e verificar que o container `dev` inicia o binário via `air`
- [x] 6.3 Criar `docker-compose.yml` (`postgres:17-alpine` com volume `pgdata`, healthcheck `pg_isready`, porta `${POSTGRES_PORT:-5432}`; `api` com target `dev`, bind mount, volumes de cache Go, `env_file` opcional, `DATABASE_URL` padrão, porta `${HTTP_PORT:-8080}`, `depends_on` com `service_healthy`) e `.env.example` documentando todas as variáveis; verificar que `docker compose config` é válido sem `.env` presente
- [x] 6.4 Criar `Makefile` com `up`, `down`, `reset`, `logs`, `ps`, `psql`, `test`, `fmt`, `lint`, `build` e verificar que `make test` e `make build` executam com sucesso
- [x] 6.5 Escrever `README.md` (pré-requisitos, `make up`, endpoints, variáveis, reset do banco, aviso de credenciais apenas para dev e de manter o repo fora de `/mnt/c` no WSL2) e verificar que os comandos documentados rodam como descrito

## 7. Verificação integrada

- [x] 7.1 Com `make up`, verificar que `curl -i localhost:8080/healthz` → 200 e `curl -i localhost:8080/readyz` → 200, e que o container `api` só iniciou após o `postgres` ficar saudável (`docker compose ps` / logs)
- [x] 7.2 Com o ambiente no ar, executar `docker compose stop postgres` e verificar `/readyz` → 503 e `/healthz` → 200; reiniciar o banco e verificar que `/readyz` volta a 200
- [x] 7.3 Editar um arquivo `.go` (ex.: mensagem de log de startup) com o ambiente rodando e verificar nos logs que o `air` recompilou e reiniciou o serviço
- [x] 7.4 Criar uma tabela de teste via `make psql`, executar `make down` e `make up`, verificar que a tabela persiste; executar `make reset` + `make up` e verificar que o banco está vazio
- [x] 7.5 Criar `.env` com `HTTP_PORT=9090`, subir o ambiente e verificar que a API responde em `localhost:9090`; executar `docker compose stop api` e verificar nos logs a mensagem de encerramento gracioso e saída com código 0
- [x] 7.6 Rodar `go test ./...` e `go vet ./...` e verificar que passam sem falhas
