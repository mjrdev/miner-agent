# ADR-0001: Registrar decisões arquiteturais com ADRs

**Status:** Aceito
**Data:** 2026-10-22

## Contexto

As decisões do bootstrap do serviço (layout, roteador, logs, configuração, banco, ambiente Docker) ficaram registradas apenas no `design.md` de uma change do OpenSpec já arquivada. Esse documento descreve o *plano* e já divergiu da implementação em alguns pontos (por exemplo, a porta do PostgreSQL no host). Artefatos de change são históricos de uma mudança, não um registro vivo de por que o sistema é como é.

## Decisão

Cada decisão arquitetural é registrada em um ADR próprio em `docs/adr/NNNN-slug.md`, no formato MADR enxuto em português definido em [`template.md`](template.md), com status (`Proposto`, `Aceito`, `Rejeitado`, `Obsoleto`, `Substituído por ADR-NNNN`) e índice em [`README.md`](README.md). ADRs aceitos não são reescritos: mudanças de decisão geram um ADR novo que substitui o anterior. A skill `/adr` do Claude Code (`.claude/skills/adr/`) automatiza numeração, preenchimento e índice.

Os ADRs 0002–0008 documentam retroativamente as decisões do bootstrap, descrevendo o estado implementado.

## Alternativas consideradas

- **Manter as decisões só nos `design.md` do OpenSpec**: ficam presas a uma change arquivada e não são atualizadas quando a implementação diverge.
- **Formato de Michael Nygard**: não tem seção explícita de alternativas, justamente o que mais se perde com o tempo.
- **MADR completo**: seções demais (drivers, prós/contras por opção) para decisões pequenas.
- **Ferramentas como `adr-tools` ou `log4brains`**: dependência extra para algo que um modelo e uma skill resolvem.

## Consequências

### Positivas

- Um lugar único e versionado para entender o porquê das escolhas.
- Criar um ADR custa pouco (`/adr <decisão>`), o que aumenta a chance de ele ser escrito na hora certa.

### Negativas

- Exige disciplina para registrar decisões novas e manter o índice; a skill reconcilia o índice com os arquivos a cada uso para reduzir esse risco.
- Os ADRs retroativos foram escritos depois do fato e se limitam às alternativas que já haviam sido avaliadas no planejamento.

## Referências

- `openspec/changes/archive/2026-10-22-bootstrap-go-http-service/design.md`
- `.claude/skills/adr/SKILL.md`
