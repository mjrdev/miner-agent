# ADR-0006: PostgreSQL com `pgx/v5` e conexão com retry

**Status:** Aceito
**Data:** 2026-10-22

## Contexto

O serviço precisa de um pool de conexões PostgreSQL pronto para funcionalidades futuras. No ambiente local o banco pode ficar disponível alguns segundos depois da API, e o endpoint `/readyz` precisa verificar o banco sem acoplar os handlers ao driver.

## Decisão

- Usamos `github.com/jackc/pgx/v5` (v5.11.0) com `pgxpool`.
- `internal/database.Connect` cria o pool e faz `Ping` (timeout de 2s por tentativa) a cada 1s por até **30s**; se não conseguir, a subida falha com erro. Erros de parsing da URL não são propagados com o conteúdo, para não vazar credenciais, e a `DATABASE_URL` nunca é logada.
- O readiness depende da interface mínima `database.Pinger { Ping(ctx) error }`, satisfeita por `*pgxpool.Pool` e por fakes nos testes; `/readyz` aplica timeout de 2s e responde `503` sem detalhes do erro.
- Sem ORM nem camada de repositórios por enquanto.

## Alternativas consideradas

- **`database/sql` + `lib/pq`**: `lib/pq` está em modo de manutenção.
- **`database/sql` + driver `pgx/stdlib`**: possível, mas perde recursos nativos do pgx sem ganho atual.
- **ORM (GORM, ent)**: não há modelo de dados ainda; adicionaria abstração antes da necessidade.

## Consequências

### Positivas

- Driver mais usado e mantido do ecossistema, com pool nativo.
- A API tolera o banco subindo depois dela; handlers testáveis sem banco real.

### Negativas

- Uma indisponibilidade do banco por mais de 30s na subida derruba o processo (comportamento desejado, mas depende de um orquestrador para reiniciar).
- A ferramenta de migrações ainda não foi escolhida; `migrations/` está apenas reservada (decisão futura).

## Referências

- `internal/database/postgres.go`, `internal/handler/health.go`, `go.mod`
