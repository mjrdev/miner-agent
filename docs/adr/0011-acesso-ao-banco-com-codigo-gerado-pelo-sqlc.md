# ADR-0011: Acesso ao banco com código gerado pelo sqlc

**Status:** Aceito
**Data:** 2026-10-22

## Contexto

O domínio Task é o primeiro a gravar dados ([ADR-0009](0009-dominio-task-em-internal-task-com-subpacotes.md)). Ele usa recursos específicos do PostgreSQL:
- `text[]` para as instruções adicionais;
- `jsonb` para o payload bruto;
- `ON CONFLICT DO NOTHING` na ingestão idempotente;
- `FOR UPDATE` nas edições;
- listagem com filtros opcionais.

O serviço já usa um pool `pgx/v5` ([ADR-0006](0006-postgresql-com-pgx-v5-e-conexao-com-retry.md)), e o ambiente local exige apenas Docker ([ADR-0007](0007-ambiente-de-desenvolvimento-com-docker-compose-e-hot-reload.md)).

## Decisão

Escrevemos as queries em SQL em `internal/task/postgres/queries/*.sql` e geramos código Go tipado com o **sqlc**:

- **Configuração:** `sqlc.yaml` na raiz, com `engine: postgresql` e `sql_package: pgx/v5` (o código gerado usa o pool existente). O schema vem de `migrations/`, que o sqlc lê direto dos arquivos do goose.
- **Tipos:** overrides mapeiam `uuid` para `github.com/google/uuid.UUID` (ponteiro quando nulo) e `timestamptz` para `time.Time`. `emit_pointers_for_null_types` gera ponteiros para as demais colunas nulas.
- **Execução:** o sqlc roda pela imagem `sqlc/sqlc:1.30.0` via `make sqlc`, sem entrar no `go.mod`. O código gerado em `sqlcdb/` é commitado e não é editado à mão.
- **Filtros opcionais:** usam `sqlc.narg(...) IS NULL OR ...`. O total da listagem vem de uma query `CountTasks` separada.
- **Transações:** as escritas compostas (task e labels) rodam em transação via `Queries.WithTx`.

## Alternativas consideradas

- **pgx puro**: mapeamento linha a linha manual, verboso e propenso a erro.
- **sqlx / scany**: erros de nome ou tipo de coluna só aparecem em runtime.
- **ORMs (GORM, Bun, ent)**: escondem o SQL e lidam mal com `text[]`, `jsonb` e `ON CONFLICT`.

## Consequências

### Positivas

- O SQL fica explícito e revisável, e erros de schema aparecem na geração, não em produção.
- Sem reflexão em runtime, sobre o mesmo pool `pgx/v5`.

### Negativas

- Toda mudança de query ou de schema exige rodar `make sqlc`. Ainda não há CI para detectar código gerado desatualizado (`sqlc diff`).
- Filtros dinâmicos ficam verbosos. A listagem repete os filtros na query de contagem.
- O sqlc gera um tipo de linha por query, mesmo quando as colunas são iguais. O repositório converte entre elas.

## Referências

- `sqlc.yaml`, `internal/task/postgres/`, `Makefile` (alvo `sqlc`)
- `openspec/changes/add-task-ingestion/design.md` (decisão 6)
