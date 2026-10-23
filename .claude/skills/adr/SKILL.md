---
name: adr
description: Cria e mantém Architecture Decision Records (ADRs) em docs/adr/ — numera, preenche o modelo do repositório, atualiza o índice e trata substituição de decisões. Use quando a pessoa pedir para registrar, documentar ou justificar uma decisão de arquitetura ("criar ADR", "registrar decisão", "documentar por que escolhemos X", "record an architecture decision", "write an ADR"), ou para substituir/obsoletar um ADR existente. Não use para documentação geral, READMEs ou specs do OpenSpec.
---

# ADR

Registra uma decisão arquitetural em `docs/adr/NNNN-slug.md` seguindo o modelo do repositório e mantém o índice `docs/adr/README.md` em dia.

**Entrada:** o argumento de `/adr` (título ou descrição da decisão) e o contexto da conversa. Sem argumento, pergunte qual decisão registrar.

## Fluxo

1. **Entender a decisão.** Extraia do pedido e da conversa: o problema (contexto), o que foi decidido, as alternativas descartadas e o motivo de cada uma. Se faltar o contexto ou a decisão, ou se não houver como saber as alternativas, **pergunte antes de escrever** — nunca invente debates, números ou motivações. Se o pedido descrever várias decisões independentes, proponha um ADR para cada uma.

2. **Embasar no repositório.** Leia o código, a configuração e a documentação que a decisão afeta (e ADRs existentes relacionados) para que Contexto e Consequências descrevam o estado *real* do projeto. Quando o planejado e o implementado divergirem, registre o implementado e explique a divergência.

3. **Preparar `docs/adr/`.** Se a pasta, `template.md` ou `README.md` não existirem, crie-os a partir do [modelo mínimo](#modelo-mínimo-para-bootstrap) abaixo. Caso contrário, use sempre o `docs/adr/template.md` do repositório como fonte do formato.

4. **Numerar.** O próximo número é o maior `NNNN` entre os arquivos `docs/adr/[0-9][0-9][0-9][0-9]-*.md` mais 1, com 4 dígitos; `0001` se não houver nenhum. Números nunca são reutilizados, nem de ADRs substituídos ou rejeitados.

   ```bash
   last=$(ls docs/adr 2>/dev/null | grep -E '^[0-9]{4}-.*\.md$' | sort | tail -1 | cut -c1-4)
   printf '%04d\n' $((10#${last:-0} + 1))
   ```

5. **Gerar o slug** a partir do título: minúsculas, acentos removidos (á→a, ç→c, õ→o…), todo outro caractere vira `-`, hifens repetidos colapsados e removidos das pontas. Se passar de 60 caracteres, corte no último hífen antes do limite (nunca no meio de uma palavra) — e, se o slug ficar truncado de forma estranha, prefira um título mais curto.

   ```bash
   s=$(printf '%s' "$titulo" | iconv -f UTF-8 -t ASCII//TRANSLIT | tr '[:upper:]' '[:lower:]' \
     | sed -E 's/[^a-z0-9]+/-/g; s/^-+|-+$//g')
   [ ${#s} -gt 60 ] && { s=${s:0:61}; s=${s%-*}; }
   echo "$s"
   ```

6. **Escrever o ADR** em `docs/adr/NNNN-slug.md`, em português, preenchendo todas as seções do modelo:
   - Título `# ADR-NNNN: <Título>` (título legível, com acentos).
   - `**Status:**` — `Aceito` se a decisão já está em vigor no código; `Proposto` se ainda está em discussão. Outros valores válidos: `Rejeitado`, `Obsoleto`, `Substituído por ADR-NNNN`.
   - `**Data:**` — data de hoje em `AAAA-MM-DD`, salvo se a pessoa indicar a data da decisão.
   - Contexto, Decisão, Alternativas consideradas (uma entrada por opção, com o motivo) e Consequências (positivas e negativas). Referências é opcional — prefira caminhos de arquivos do repositório e links para ADRs relacionados.
   - Remova todos os comentários `<!-- -->` e placeholders do modelo. Seja conciso: um ADR típico cabe em uma tela.

7. **Substituição.** Se o novo ADR substitui um anterior:
   - no novo: `**Substitui:** [ADR-XXXX](XXXX-slug.md)` abaixo da data e menção no Contexto;
   - no antigo: altere **apenas** a linha de status para `**Status:** Substituído por ADR-NNNN` e adicione `**Substituído por:** [ADR-NNNN](NNNN-slug.md)` logo abaixo da data. Não mexa em mais nada do ADR antigo.
   Para marcar um ADR como `Obsoleto` ou `Rejeitado`, altere só o status (e o índice).

8. **Atualizar o índice** `docs/adr/README.md`: a tabela `| ADR | Título | Status | Data |` deve ter uma linha por ADR, em ordem numérica, no formato `| [NNNN](NNNN-slug.md) | Título | Status | AAAA-MM-DD |`. Antes de salvar, reconcilie a tabela com os arquivos existentes: adicione linhas faltantes e corrija status/títulos divergentes, lendo-os dos próprios ADRs.

9. **Reportar** o caminho do ADR criado (e dos alterados, em caso de substituição) com um resumo de uma linha da decisão.

## Verificação a seco

Os passos mecânicos devem produzir exatamente:

| Entrada | Resultado esperado |
|---------|--------------------|
| título "Registrar decisões com ADRs" | slug `registrar-decisoes-com-adrs` |
| título "Configuração por variáveis de ambiente" | slug `configuracao-por-variaveis-de-ambiente` |
| título com mais de 60 caracteres | slug cortado no último hífen antes do 61º caractere |
| maior ADR existente `0008-...` | próximo número `0009` |
| nenhum ADR em `docs/adr/` | próximo número `0001` |

## Modelo mínimo para bootstrap

Use apenas quando `docs/adr/template.md` não existir; depois disso o arquivo do repositório é a fonte do formato.

```markdown
# ADR-NNNN: Título curto da decisão

**Status:** Proposto
**Data:** AAAA-MM-DD

## Contexto

## Decisão

## Alternativas consideradas

- **Alternativa**: motivo de não ter sido escolhida.

## Consequências

### Positivas

### Negativas

## Referências
```

`docs/adr/README.md` mínimo: um parágrafo explicando o que é um ADR, como criar (`/adr` ou copiando o `template.md`), a lista de status válidos e a seção `## Índice` com a tabela `| ADR | Título | Status | Data |`.
