# ADR-0003: Roteamento com `net/http.ServeMux` e middlewares próprios

**Status:** Aceito
**Data:** 2026-10-22

## Contexto

O serviço expõe por enquanto apenas `GET /healthz` e `GET /readyz`, e as specs exigem `404` para rotas desconhecidas e `405` para métodos não suportados. Também são necessários logging por requisição e recuperação de panics.

## Decisão

Usamos o `http.ServeMux` da biblioteca padrão com padrões com método (Go 1.22+, ex.: `mux.HandleFunc("GET /healthz", ...)`), que já responde `404` e `405` sozinho. Os middlewares são funções próprias em `internal/server/middleware.go`, encadeadas como `logRequests(log, recoverPanics(log, mux))`.

O logging fica **por fora** do recover — o inverso do que o plano previa — para que uma requisição que entrou em panic seja registrada com status `500` na linha de log da requisição. O recover relança `http.ErrAbortHandler`, preservando a semântica do `net/http`.

## Alternativas consideradas

- **chi**: compatível com `net/http` e útil para grupos de rotas e middlewares por subárvore; desnecessário com duas rotas. A migração continua barata porque os handlers são `http.Handler` comuns.
- **gin / echo**: APIs e contextos próprios, mais acoplamento ao framework.

## Consequências

### Positivas

- Zero dependências para HTTP; comportamento de 404/405 vem pronto.
- Handlers padrão facilitam testes com `httptest` e uma eventual troca de roteador.

### Negativas

- Sem agrupamento de rotas nem middlewares por prefixo; se isso fizer falta, um novo ADR deve avaliar `chi`.

## Referências

- `internal/server/routes.go`, `internal/server/middleware.go`, `internal/server/server_test.go`
