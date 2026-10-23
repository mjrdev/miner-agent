---
name: spec-git-flow
description: Fluxo git de uma change do OpenSpec — ao começar uma nova spec cria e muda para a branch `spec/<change>` a partir da `main` atualizada; ao arquivar faz os commits, o push e abre o PR para a `main` automaticamente. Use SEMPRE junto com openspec-propose / opsx:propose (antes de criar a change) e openspec-archive-change / opsx:archive (depois de mover a change para o arquivo), ou quando a pessoa pedir "abrir branch da spec", "commitar e abrir PR da change". Não use para commits avulsos fora de uma change do OpenSpec.
---

# Spec Git Flow

Cada change do OpenSpec vive na sua própria branch e termina em um PR. Esta skill tem duas fases, executadas como parte dos fluxos de propose e archive — não espere a pessoa pedir.

**Branch:** `spec/<change-name>` (o mesmo kebab-case usado em `openspec new change`).
**Base:** `main`.

## Fase 1 — Início (ao começar uma nova spec)

Execute **antes** de `openspec new change "<name>"`, assim que o nome da change estiver definido.

1. **Verificar o estado.**
   ```bash
   git status --porcelain
   git branch --show-current
   ```
   - Se já estiver em `spec/<change-name>`: nada a fazer, siga com o propose.
   - Se estiver em outra branch `spec/*` ou com alterações não commitadas que não pertencem a esta change: **pergunte** se deve levá-las para a nova branch, fazer stash ou parar. Nunca descarte alterações.

2. **Atualizar a base.**
   ```bash
   git switch main
   git pull --ff-only origin main   # pule se origin/main ainda não existir
   ```
   Se o `pull --ff-only` falhar (main divergiu), pare e informe — não faça merge nem rebase por conta própria.

3. **Criar a branch.**
   ```bash
   git switch -c spec/<change-name>
   ```
   Se a branch já existir, pergunte se deve reutilizá-la (`git switch spec/<change-name>`) ou escolher outro nome.

4. Informe em uma linha: `Trabalhando na branch spec/<change-name>.` e continue o propose normalmente. O apply também acontece nesta branch.

## Fase 2 — Fim (ao arquivar)

Execute **depois** que o archive mover a change para `openspec/changes/archive/` com sucesso. Se o archive falhou ou foi interrompido, não faça nada.

1. **Conferir a branch.** Deve ser `spec/<change-name>`. Se estiver na `main`, crie a branch agora (`git switch -c spec/<change-name>`) — nunca commite direto na `main`.

2. **Revisar o que vai entrar.**
   ```bash
   git status --porcelain
   git diff --stat
   ```
   Nunca inclua `.env`, credenciais, chaves, binários de build ou arquivos ignorados. Se aparecer algo suspeito ou sem relação com a change, pergunte antes de incluir.

3. **Commitar em commits lógicos**, no padrão do repositório (Conventional Commits, título curto em inglês, corpo em bullets):
   - `feat:` / `fix:` / `refactor:` — código da implementação
   - `test:` — testes
   - `spec:` — specs principais sincronizadas (`openspec/specs/**`)
   - `chore: archive <change-name> change` — a change arquivada (`openspec/changes/archive/**` e a remoção de `openspec/changes/<change-name>`)

   Adicione os arquivos por caminho (`git add <paths>`), não `git add -A`. Use HEREDOC para a mensagem e termine cada commit com a linha de atribuição de co-autoria vigente na sessão. Commits já feitos durante o apply ficam como estão.

4. **Publicar a branch.**
   ```bash
   git push -u origin spec/<change-name>
   ```
   Se o push for rejeitado, pare e informe — nunca use `--force`.

5. **Abrir o PR.** Se já existir um PR aberto para a branch (`gh pr view spec/<change-name>`), só informe o link. Caso contrário:
   ```bash
   gh pr create --base main --head spec/<change-name> \
     --title "<tipo>: <resumo da change>" \
     --body "$(cat <<'EOF'
   ## Resumo
   <o "por quê" e o "o quê" do proposal.md, em 2–4 bullets>

   ## Specs
   <capabilities adicionadas/modificadas/removidas>

   ## Tarefas
   <N/N concluídas; liste as pendentes, se houver>

   Change arquivada em `openspec/changes/archive/<target-name>/`.

   <linha de atribuição de PR vigente na sessão>
   EOF
   )"
   ```
   Tire o conteúdo do `proposal.md`, dos delta specs e do `tasks.md` da change arquivada — não invente. Se o `gh` não estiver autenticado, pare após o push e peça para a pessoa rodar `! gh auth login`.

6. **Resumo final.** Acrescente ao resumo do archive:
   ```
   **Branch:** spec/<change-name>
   **Commits:** <lista curta>
   **PR:** <url>
   ```
   Não faça merge do PR nem volte para a `main` — isso fica com a pessoa.
