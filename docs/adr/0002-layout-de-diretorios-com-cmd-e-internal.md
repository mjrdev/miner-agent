# ADR-0002: Layout de diretórios com `cmd/` e `internal/`

**Status:** Aceito
**Data:** 2026-10-22

## Contexto

O repositório começou vazio e precisava de uma estrutura que qualquer pessoa acostumada com Go reconhecesse, sem cerimônia excessiva para um serviço que ainda não tem regras de negócio.

## Decisão

Seguimos o layout comunitário (inspirado em `golang-standards/project-layout`), criando só as pastas que têm uso:

- `cmd/api/main.go` — composição da aplicação (config → logger → banco → servidor) em uma função `run(ctx) error`; `os.Exit(1)` acontece em um único lugar.
- `internal/config`, `internal/logger`, `internal/database`, `internal/handler`, `internal/server` — um pacote por responsabilidade, cada um com seus testes.
- `migrations/` — reservada para migrações SQL futuras.

O módulo é `github.com/jrgomesdev/miner`. O pacote `internal/logger` não estava no plano original; foi criado durante a implementação para isolar a construção do logger (ver [ADR-0004](0004-logs-estruturados-em-json-com-log-slog.md)).

## Alternativas consideradas

- **Arquitetura hexagonal completa (`domain/`, `ports/`, `adapters/`)**: cerimônia demais sem domínio ainda; pode evoluir quando surgirem regras de negócio.
- **Pasta `pkg/`**: não há código pensado para ser importado por outros módulos; `internal/` impede esse uso por acidente.
- **Tudo em `package main`**: rápido no início, mas dificulta testes e cresce mal.

## Consequências

### Positivas

- Estrutura familiar e pacotes testáveis de forma isolada.
- `internal/` garante que nada vaze como API pública do módulo.

### Negativas

- Quando houver domínio, será preciso decidir onde ficam entidades e regras (provável novo ADR).

## Referências

- `cmd/api/main.go`, `internal/`, `go.mod`
