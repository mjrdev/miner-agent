# ADR-0007: Ambiente de desenvolvimento com Docker Compose e hot reload com `air`

**Status:** Aceito
**Data:** 2026-10-22

## Contexto

Qualquer pessoa deve conseguir subir API e banco com um comando, sem instalar Go ou PostgreSQL, e ver alterações no código refletidas sem reiniciar containers. O desenvolvimento acontece no WSL2, onde eventos de filesystem nem sempre atravessam bind mounts.

## Decisão

- **Dockerfile multi-stage** (`golang:1.26-alpine`):
  - `dev` — instala `air` em versão fixa (`v1.67.4`) e roda `air -c .air.toml`; o código é montado em `/app`;
  - `build` — `CGO_ENABLED=0 go build -trimpath -ldflags="-s -w"`;
  - `runtime` — `gcr.io/distroless/static-debian12:nonroot`, só com o binário (usado por `make build`, não pelo compose).
- **`docker-compose.yml`**: serviço `api` com `target: dev`, imagem com tag fixa `miner-api:dev` (sem ela, o compose geraria `miner-api` e sobrescreveria a imagem de runtime do `make build`), bind mount do repositório, volumes nomeados de cache do Go (`gomod`, `gocache`), `env_file` opcional `.env`, porta `${HTTP_PORT:-8080}` e `depends_on` com `condition: service_healthy` no PostgreSQL.
- **`.air.toml`** com `poll = true` (500ms), observando só `.go` (exceto testes), `send_interrupt = true` para que cada reload passe pelo graceful shutdown.
- **`Makefile`** como interface: `up`, `down`, `reset`, `logs`, `ps`, `psql`, `test`, `fmt`, `lint`, `build`.

## Alternativas consideradas

- **`docker compose watch` com sync + restart**: reinicia o container a cada mudança e é menos imediato que o `air`; pode complementar no futuro.
- **Rodar Go no host e só o banco no Docker**: exige Go instalado e versão alinhada em cada máquina.
- **Imagem única para dev e produção**: imagem de dev fica grande (~800MB) e com toolchain; a de runtime tem ~18MB.

## Consequências

### Positivas

- `make up` é o único passo para começar; rebuilds rápidos graças aos caches nomeados.
- O caminho para uma imagem de produção mínima e não-root já existe.

### Negativas

- O polling do `air` consome um pouco de CPU.
- Bind mounts a partir de `/mnt/c` no WSL2 são lentos; o README orienta manter o repositório no filesystem Linux.
- `make reset` também remove os caches do Go, deixando o próximo build mais lento.

## Referências

- `Dockerfile`, `docker-compose.yml`, `.air.toml`, `Makefile`, `README.md`
