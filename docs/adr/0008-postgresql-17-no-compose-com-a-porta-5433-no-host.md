# ADR-0008: PostgreSQL 17 no compose com a porta 5433 no host

**Status:** Aceito
**Data:** 2026-10-22

## Contexto

O ambiente local precisa de um PostgreSQL com dados persistentes e acessível também por clientes no host (psql, DBeaver). O plano previa expor a porta padrão `5432` no host, mas na primeira subida ela já estava ocupada por um container de outro projeto (`urlshortener-database-1`) e o `docker compose up` falhou com `port is already allocated`.

## Decisão

- Imagem `postgres:17-alpine`, credenciais de desenvolvimento `miner`/`miner`/banco `miner` (sobrescrevíveis por `POSTGRES_USER`, `POSTGRES_PASSWORD`, `POSTGRES_DB`).
- Dados no volume nomeado `pgdata` (`miner_pgdata`), preservados por `make down` e removidos por `make reset`.
- Healthcheck com `pg_isready`, usado pela API em `depends_on`.
- A porta exposta no host é **`5433`** por padrão (`${POSTGRES_PORT:-5433}:5432`). Dentro da rede do compose o banco continua em `5432`, então a `DATABASE_URL` da API (`postgres://miner:miner@postgres:5432/miner`) não muda. Isso diverge do plano original, que citava `5432`.

## Alternativas consideradas

- **Manter `5432` e exigir `POSTGRES_PORT` no `.env`**: quebraria a promessa de subir sem configuração em máquinas que já têm um PostgreSQL local, como esta.
- **Não expor a porta no host**: evitaria conflitos, mas impediria clientes gráficos e ferramentas locais; `make psql` sozinho não cobre esses casos.
- **Parar o container que ocupava a 5432**: pertence a outro projeto e não deve ser afetado.

## Consequências

### Positivas

- `make up` funciona sem configuração mesmo com um PostgreSQL local em execução.
- A porta continua configurável por `POSTGRES_PORT`.

### Negativas

- Quem espera a porta padrão precisa lembrar de `localhost:5433` ao conectar pelo host (documentado no README e no `.env.example`).
- As credenciais fixas servem apenas para desenvolvimento local.

## Referências

- `docker-compose.yml`, `.env.example`, `README.md`
- [ADR-0007](0007-ambiente-de-desenvolvimento-com-docker-compose-e-hot-reload.md)
