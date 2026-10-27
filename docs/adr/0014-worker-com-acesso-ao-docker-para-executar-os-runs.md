# ADR-0014: Worker com acesso ao Docker para executar os runs

**Status:** Aceito
**Data:** 2026-10-26

## Contexto

A change `add-host-task-runner` executa cada task num ambiente descartável onde o agente de IA clona o projeto e trabalha. O runner `host` usa containers do Docker da própria máquina. A api roda num container do compose. Para criar outros containers, algum processo precisa do socket do Docker (`/var/run/docker.sock`), e quem tem o socket tem, na prática, root no host.

Há uma segunda restrição: o agente roda com `--dangerously-skip-permissions`, sem ninguém para aprovar as ações dele. Quem limita o que ele alcança é o container, não o agente.

## Decisão

- **Serviço próprio:** um `worker` separado (`cmd/worker`, serviço `worker` no compose) é o **único** processo com o socket do Docker e com os segredos: as credenciais do agente e o `GIT_TOKEN`. A api não recebe nenhum dos dois. O `env_file` saiu da api, e cada serviço recebe só as variáveis que lista em `environment:`.
- **SDK oficial:** o runner `host` (`internal/execution/host`) usa o SDK do Docker Engine para Go (`github.com/moby/moby/client`), o único pacote que importa o SDK.
- **Cada job** é um container descartável da imagem `miner-agent:default`, com:
  - usuário uid 1000, `cap-drop ALL` e `no-new-privileges`, sem modo privilegiado e sem o socket montado
  - apenas o diretório do run montado em `/job`
  - a rede bridge `miner-runs`, fora da rede do compose, então `postgres` e `rabbitmq` não resolvem
  - o label `miner.run`, que permite remover os órfãos
- **Credenciais no job:** a do agente entra como variável de ambiente. O `GIT_TOKEN` vai **por arquivo** (`<run>/git-token`, modo 600). O `GIT_ASKPASS` o lê só durante o clone, o entrypoint o apaga antes de iniciar o agente, e o worker apaga de novo no fim. Ele não vai por variável de ambiente porque o agente roda com o mesmo uid do processo 1 do container e leria `/proc/1/environ`.
- **Caminho do host:** os bind mounts pedidos pelo socket usam caminhos do host. O worker descobre o caminho de `./runs` inspecionando o próprio container, e `RUNS_HOST_DIR` sobrescreve a descoberta.

## Alternativas consideradas

- **Dar o socket à api:** colocaria o equivalente a root no processo exposto em HTTP, sem autenticação, junto com o CRUD.
- **Chamar o binário `docker`:** exigiria o CLI na imagem do worker e leitura de texto para os códigos de saída e a inspeção. O SDK dá tipos e erros (`IsNotFound`) diretamente.
- **Passar o `GIT_TOKEN` por variável de ambiente e removê-lo com `unset`:** ele continuaria legível em `/proc/1/environ` pelo agente.
- **Docker-in-Docker ou rootless:** isolam melhor, mas exigem container privilegiado ou um daemon a mais. É complexidade demais para um serviço local de uso individual.

## Consequências

### Positivas

- A api continua sem segredos e sem acesso ao Docker, e o worker pode ser reiniciado sem derrubar o CRUD.
- O agente não alcança o banco, o broker, o Docker nem o token do git.
- O contrato `execution.Runner` isola o Docker. Um runner `k8s` futuro não muda o worker.

### Negativas

- Quem compromete o worker compromete o host. O risco é aceito por ser um serviço local de uso individual.
- O container alcança o gateway do host e as portas publicadas pelo compose (Postgres na 5433, RabbitMQ na 5672). A mitigação futura é não publicar essas portas ou usar o runner `k8s` com NetworkPolicy.
- Dentro do workspace e na internet, o agente pode fazer qualquer coisa.
- Os arquivos do workspace ficam com o uid 1000, o que pode exigir `sudo` num host com outro uid.

## Referências

- `internal/execution/host/host.go`, `internal/execution/run/worker/worker.go`, `cmd/worker/main.go`
- `build/agent/Dockerfile`, `build/agent/miner-job`, `docker-compose.yml`
- [ADR-0013](0013-dominio-execution-com-catalogo-de-agentes-em-codigo.md), [ADR-0015](0015-run-como-fonte-da-verdade-da-execucao.md)
