# ADR-0005: Configuração por variáveis de ambiente

**Status:** Aceito
**Data:** 2026-10-22

## Contexto

O serviço roda em container e precisa de poucas configurações. Erros de configuração devem aparecer na subida, não durante o atendimento de requisições.

## Decisão

A configuração vem apenas de variáveis de ambiente, lidas e validadas em `internal/config.Load()` com `os.Getenv`, `strconv` e `time.ParseDuration`:

| Variável | Padrão |
|----------|--------|
| `APP_ENV` | `development` |
| `HTTP_PORT` | `8080` (1–65535) |
| `LOG_LEVEL` | `info` |
| `DATABASE_URL` | obrigatória |
| `SHUTDOWN_TIMEOUT` | `10s` |

Todos os erros são reunidos com `errors.Join`, cada um nomeando a variável, e o processo encerra com código diferente de zero (fail fast). No ambiente local, o `docker-compose.yml` fornece valores padrão e lê sobrescritas opcionais de `.env`, documentado em `.env.example`.

## Alternativas consideradas

- **viper**: suporta arquivos, flags e recarga, mas é uma dependência grande para cinco variáveis.
- **envconfig / env (struct tags)**: menos código, mas outra dependência e mensagens de erro menos controladas.
- **Arquivo de configuração (YAML/TOML)**: menos idiomático em containers.

## Consequências

### Positivas

- Sem dependências; falhas de configuração aparecem na hora, com a variável identificada.
- Fácil de testar: `load` recebe a função de leitura do ambiente.

### Negativas

- O parsing manual cresce linearmente com o número de variáveis; se passarem de algumas dezenas, vale reavaliar uma biblioteca.

## Referências

- `internal/config/config.go`, `internal/config/config_test.go`, `.env.example`
