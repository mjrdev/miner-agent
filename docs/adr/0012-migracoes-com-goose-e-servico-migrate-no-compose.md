# ADR-0012: Migrações com goose e serviço `migrate` no compose

**Status:** Aceito
**Data:** 2026-10-22

## Contexto

A pasta `migrations/` estava reservada desde o bootstrap, sem ferramenta escolhida. O domínio Task cria as primeiras tabelas. O `docker compose up` precisa continuar subindo tudo em um comando ([ADR-0007](0007-ambiente-de-desenvolvimento-com-docker-compose-e-hot-reload.md)), já com o schema aplicado. Os arquivos de migração também servem de schema para o sqlc ([ADR-0011](0011-acesso-ao-banco-com-codigo-gerado-pelo-sqlc.md)).

## Decisão

Usamos o **goose** (`github.com/pressly/goose/v3`) como biblioteca:

- **Arquivos:** migrações em SQL, `migrations/NNNNN_descricao.sql` com numeração sequencial, e as seções `-- +goose Up` e `-- +goose Down` no mesmo arquivo.
- **Embed:** `migrations/embed.go` embute os arquivos (`migrations.FS`).
- **Comando:** `cmd/migrate` aceita `up`, `down` e `status`. Ele usa `goose.NewProvider` sobre `database/sql` com o driver `pgx/v5/stdlib` e lê `DATABASE_URL`.
- **No compose:** um serviço `migrate` roda `go run ./cmd/migrate up` depois que o `postgres` fica saudável. A `api` só inicia com `migrate: service_completed_successfully`.
- **Fora da subida:** `make migrate` aplica as pendentes e `make migrate-down` reverte a última.

## Alternativas consideradas

- **golang-migrate**: dois arquivos por migração (`up`/`down`). Quando uma migração falha, o estado "dirty" exige intervenção manual.
- **tern**: do mesmo autor do pgx, mas com comunidade menor.
- **Atlas**: migrações declarativas por diff de schema; pesado para o tamanho do projeto.
- **Migrar no startup da API**: com várias réplicas, todas tentariam migrar ao mesmo tempo. O passo separado evita isso e ainda permite embutir no startup no futuro.

## Consequências

### Positivas

- `docker compose up` em um banco vazio já sobe com as tabelas, e uma migração com erro impede a API de iniciar.
- O mesmo arquivo serve de migração e de schema para o sqlc.
- Reaplicar sem pendências não faz nada.

### Negativas

- O serviço `migrate` compila o comando a cada subida. O cache de build do Go reduz esse custo, mas a subida fica alguns segundos mais lenta.
- Ambientes fora do compose precisam rodar `cmd/migrate up` antes de subir uma versão nova.

## Referências

- `migrations/`, `cmd/migrate/main.go`, `docker-compose.yml`, `Makefile`
- `openspec/changes/add-task-ingestion/design.md` (decisão 7)
