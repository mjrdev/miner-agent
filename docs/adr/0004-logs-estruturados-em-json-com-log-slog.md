# ADR-0004: Logs estruturados em JSON com `log/slog`

**Status:** Aceito
**Data:** 2026-10-22

## Contexto

O serviço precisa de logs legíveis por máquina (horário, nível, mensagem e campos como método, caminho, status e duração), com nível configurável por `LOG_LEVEL`.

## Decisão

Usamos `log/slog` da biblioteca padrão com `slog.NewJSONHandler` escrevendo em `stdout`. A construção fica em `internal/logger.New(w, level)`, e o logger é passado explicitamente aos componentes (e também definido como `slog.SetDefault`). O nível vem de `LOG_LEVEL` (`debug`, `info`, `warn`, `error`; padrão `info`).

## Alternativas consideradas

- **zap / zerolog**: mais rápidos, mas o ganho de performance não se justifica neste volume e adicionaria uma dependência.
- **`log` padrão em texto**: sem estrutura nem níveis.

## Consequências

### Positivas

- Nenhuma dependência; API estável da biblioteca padrão.
- O formato JSON pode ser consumido por qualquer agregador de logs no futuro.

### Negativas

- Logs em JSON são menos agradáveis de ler no terminal durante o desenvolvimento.
- Durações são serializadas em nanossegundos no log de requisição.

## Referências

- `internal/logger/logger.go`, `internal/server/middleware.go`
