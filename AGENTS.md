# Leia antes de agir

Este arquivo é **autossuficiente de propósito**. Agente de nuvem roda com token
que enxerga só este repositório — `jacu-dev/infra` responde 404 para ele. Um
ponteiro para lá seria pior que nada, porque ensina a desistir.

## Regras que não se negociam

**Pare no dono só pelo que gera custo recorrente.** Máquina nova, plano pago,
mensalidade. Bucket, token escopado, namespace, fila e banco são de graça — o
caminho existe, use.

**Nunca suba Docker, Colima, Postgres ou container na máquina do Erick.** Nem
para validar, nem "só um minuto". Validação é no CI ou na VPS. Se não dá para
provar sem container local, **não prove**: abra PR e deixe o CI provar.

**Commit não leva trailer de IA.** Sem `Co-Authored-By`, sem "Generated with",
sem menção a Claude, Cursor ou agente. Alguns repositórios têm lint que reprova;
nos outros é regra do dono do mesmo jeito.

**Quando faltar acesso, peça — não improvise e não desista em silêncio.** Diga
exatamente qual recurso, para quê, e o que você faria com ele.

## Começando um projeto novo — o ponto zero

Não invente esteira. Existe um molde, ele é maioria na organização, e sair dele
sem motivo escrito é como se abre buraco. Copie e ajuste só o que o projeto
exige.

`.github/workflows/ci.yml`, o arquivo inteiro:

```yaml
name: CI
on:
  pull_request:
  workflow_dispatch:
permissions:
  contents: read
concurrency:
  group: ci-${{ github.workflow }}-${{ github.ref }}
  cancel-in-progress: ${{ github.ref != 'refs/heads/main' }}
jobs:
  verify:
    permissions:          # OBRIGATORIO aqui. Reusable NAO eleva permissao.
      contents: read
      packages: read
    uses: jacu-dev/jacu-dev-ci/.github/workflows/verify.yml@v1
    with:
      runner: jacu-runner
  all-checks-passed:
    if: always()
    needs: [verify]
    runs-on: [self-hosted, linux, jacu-runner]
    timeout-minutes: 2
    permissions: {}
    steps:
      - name: Agrega o verify
        env:
          RESULT: ${{ needs.verify.result }}
        run: |
          set -euo pipefail
          echo "verify: ${RESULT}"
          case "${RESULT}" in
            success) exit 0 ;;
            *) echo "::error::verify result=${RESULT}"; exit 1 ;;
          esac
```

Por que cada pedaço, para você não "simplificar" o que segura alguma coisa:

- **`all-checks-passed` é o único check exigido pelo ruleset**, e ele agrega os
  outros. É o nome em 11 dos repositórios da casa; os que fogem disso — `infra`
  com `secrets-cifrados`, `jacu-cli` com `quality` — fogem por razão escrita no
  cabeçalho do próprio `ci.yml`, não por descuido. Se você renomear o job, o
  check exigido some e o merge trava para sempre: ele não fica vermelho, fica
  *ausente*, e o merge box espera sem dizer o porquê.
- **`skipped` nunca é aprovação.** O `case` só aceita `success`. Se um dia você
  precisar tolerar `skipped`, tolere o **motivo**, não o estado — veja o job
  `mudou-codigo` no `ecouto` e no `bem-te-vi`.
- **`timeout-minutes` no agregador** porque o padrão do GitHub é 6 horas, e um
  agregador travado prende um dos poucos executores da casa até de noite.
- **`permissions: {}`** porque ele não lê nada; só olha resultado de job.

O que o repositório precisa ter, e a **ordem importa**:

1. o `ci.yml` acima, com um run **verde na `main`** — provado, não presumido;
2. **só então** um ruleset exigindo `all-checks-passed`, com
   `strict_required_status_checks_policy: true`;
3. `allow_auto_merge` e `delete_branch_on_merge` ligados;
4. um PR sonda que **quebra de propósito**, para provar que o portão morde.

Inverter 1 e 2 trava o repositório: check exigido que ninguém produz deixa o
merge esperando para sempre. Já aconteceu aqui.

**Repositório novo precisa entrar na allowlist do runner group** (`jacu-ci`, que
é `visibility: selected`). Sem isso o job fica `queued` para sempre, sem erro —
a fila que nunca anda. Se o seu primeiro run não sai do `queued`, é isto.

**Nada de lista do que RODA.** `paths:` no gatilho é allowlist de execução: no
dia em que o código passar a depender de um caminho fora da lista, o portão
emudece e o merge acontece sem ninguém saber. Se precisar economizar, faça o
inverso — uma lista do que **não** roda (prosa), e um agregador que confere o
*motivo* do skip.

## CI — o que quebra e por quê

**Em repositório PRIVADO, runner hospedado do GitHub está BLOQUEADO.** A
franquia da organização estourou e jobs em `ubuntu-latest` morrem em dois
segundos com *"recent account payments have failed"*, sem log e sem step. Isso
vale para **todo** job, inclusive o agregador.

**Em repositório PÚBLICO, `ubuntu-latest` é grátis e continua sendo o certo.**
`jacu-harness` e `homebrew-jacu` usam e passam. Não "conserte" isso — e não
aponte repositório público para runner self-hosted, que é regra de segurança
separada: pull request de fork é código de terceiro.

Na dúvida sobre onde você está: `gh api /repos/jacu-dev/<repo> --jq .private`.

Os runners que existem:

| label | para |
|---|---|
| `[self-hosted, linux, jacu-runner]` | verify, teste, e2e — executa código de PR |
| `[self-hosted, linux, jacu-deploy]` | **só deploy com segredo**, nunca código de PR |

**O reusable central é `jacu-dev/jacu-dev-ci/.github/workflows/verify.yml@v1`** e
o contrato dele é estreito:

```yaml
permissions:          # OBRIGATORIO no chamador. Reusable NAO eleva permissao:
  contents: read      # sem isto o GitHub recusa o arquivo e o run morre em
  packages: read      # startup_failure — sem job, sem log, sem check anexado.

jobs:
  verify:
    uses: jacu-dev/jacu-dev-ci/.github/workflows/verify.yml@v1
    with:
      runner: jacu-runner   # UNICO input aceito. Qualquer outro derruba o arquivo.
```

Sempre inclua `concurrency`, e não cancele na `main` — lá todo merge tem o mesmo
`github.ref`, e cancelar mata a verificação do merge anterior, que era de outro
commit:

```yaml
concurrency:
  group: ci-${{ github.workflow }}-${{ github.ref }}
  cancel-in-progress: ${{ github.ref != 'refs/heads/main' }}
```

**`.github/workflows/**` é protegido por ruleset da organização.** Push que
toque esses caminhos é recusado com `GH013`. Merge de PR passa. Se precisar
mesmo, **peça ao dono** — existe uma porta, e ela é ato dele, não seu.

## Pull request

### O que merece ser um pull request

**PR é unidade de valor, não de edição.** Desenhou uma tela, sobe a tela — não o
botão. Corrigiu um bug, sobe a correção inteira. Fez uma feature, sobe pronta.
Mexeu em cinco arquivos pelo mesmo motivo? **Um** PR.

Trabalhe em branch, commite quanto quiser para ter histórico, e abra o PR **no
fim**, quando a unidade pedida está completa. Um trabalho de cada vez: várias
frentes abertas se atropelam na hora de atualizar contra a `main`.

Antes de abrir, responda três perguntas. Falhou em qualquer uma, não é PR ainda:

- **Dá para observar o resultado?** Ou um teste consegue afirmar? "Depois vai
  funcionar" é fragmento.
- **Se parasse aqui para sempre, a `main` fica pior?** Botão sem tela deixa.
- **O que fica vermelho se você errou?** Se nada fica, o PR não prova nada.

**Task de SDD não é ponto de merge.** Um SDD de oito tasks mergeado task a task
deixa a `main` em oito estados pela metade. O ponto de merge é o menor conjunto
de tasks que deixa a `main` funcionando **e** entrega algo. Commit por task para
ter rastro; PR por entrega.

**Doc e comentário andam junto com a mudança que descrevem.** PR só de prosa se
justifica quando a prosa estava errada e enganando alguém — não como rotina.
Documento propagado para vários repositórios multiplica o erro pelo número de
repositórios e vira instrução que outros agentes obedecem: **confira cada
afirmação com um comando antes de propagar.** Custou 47 pull requests num dia só.

**Não abra PR para consertar o que você quebrou no PR anterior.** Corrija antes
de empurrar — `git commit --amend` existe.

### Como abrir

**Nunca abra como draft.** Draft **bloqueia** o auto-merge — o GitHub recusa
enquanto o PR não estiver pronto para revisão. Se sua ferramenta abre em draft
por padrão, marque como pronto (`gh pr ready <n>`) logo em seguida.

**O método de merge se deriva, não se adivinha.** As flags do repositório
(`allow_*_merge`) são metade da história: quando um ruleset de branch declara
`allowed_merge_methods`, **quem manda é o ruleset** — e aqui os dois lados
divergem. Em `homebrew-jacu` as três flags são `true` e ainda assim só squash
entra, porque o ruleset `main-tap-protect` restringe. Quem lê só as flags pede
`--merge` e toma recusa **depois** de o PR já estar aberto.

O método efetivo é a interseção dos dois lados. Este comando lê os dois e
imprime a flag pronta:

```bash
repo=jacu-dev/<repo>
metodo=$(gh api "/repos/$repo/rules/branches/$(gh api "/repos/$repo" --jq .default_branch)" \
  --jq '[.[] | select(.type == "pull_request") | (.parameters.allowed_merge_methods // ["merge","squash","rebase"])]' \
  | jq -r --argjson flags "$(gh api "/repos/$repo" \
      --jq '[{m:"merge",v:.allow_merge_commit},{m:"squash",v:.allow_squash_merge},{m:"rebase",v:.allow_rebase_merge}]
             | map(select(.v).m)')" \
    'reduce .[] as $r ($flags; . - (. - $r))
     | ["squash","merge","rebase"] - (["squash","merge","rebase"] - .)
     | "--" + (.[0] // "NENHUM-METODO-PERMITIDO")')
```

Com ele na mão, o fluxo é abrir e deixar a esteira decidir:

```bash
gh pr create --title "..." --body "..."   # sem --draft
gh pr merge --auto "$metodo"              # entra sozinho quando os checks passarem
```

**Não tente "deixar o gh escolher".** `gh pr merge --auto` sem flag de método
não cai no padrão do repositório: fora de terminal interativo ele morre com
*"--merge, --rebase, or --squash required when not running interactively"*
(medido no gh 2.97). Só fila de merge dispensa o método, e nenhum repositório da
organização usa fila.

Hoje o comando responde `--merge` no `ecouto` — o único sem squash — e
`--squash` em todos os outros. **Rode o comando mesmo assim**: isto aqui é foto
do dia, o ruleset é a verdade.

Vários repositórios exigem branch atualizada (`strict_required_status_checks_policy`).
Se o PR ficar `BEHIND`, atualize contra a `main` com `gh pr update-branch <n>` —
não force merge e não desabilite check. Ele **não precisa do bypass** do ruleset
de `.github/workflows/**`, e a razão importa: quem cria o commit de merge é o
GitHub, do lado do servidor, não um `push` seu. Ruleset de push não é acionado.

Com auto-merge armado o GitHub atualiza a branch **sozinho** quando ela fica
para trás. Isso é bom, e tem um efeito colateral: a branch se move debaixo de
quem está trabalhando nela, e um `push` preparado antes da atualização é
recusado com *"stale info"*. Se acontecer, busque o remoto e reaplique — não
force por cima.

## Banco em teste

Teste de integração **nunca** alcança banco de produção. Use service container,
que nasce e morre com o job:

```yaml
    services:
      postgres:
        image: postgres:16
        env: { POSTGRES_USER: app, POSTGRES_PASSWORD: app, POSTGRES_DB: app }
        ports: ["5432:5432"]
        # `--health-interval 2s` e medido: o Docker so roda o PRIMEIRO check
        # `interval` segundos depois de subir, e o Postgres fica pronto em ~2s.
        options: >-
          --health-cmd "pg_isready -U app -d app"
          --health-interval 2s --health-timeout 5s --health-retries 15
```

Migração contra o banco real é **deploy**, não teste — e deve parar no dono.

## Segredos

A origem é `jacu-dev/infra/secrets/*.enc.yaml`, cifrado com SOPS. **Você
provavelmente não alcança esse repositório**, e isso é o desenho.

Se o seu ambiente tem `SOPS_AGE_KEY`, ela é uma chave **escopada ao seu
projeto** — decifra o arquivo dele e é negada nos outros. Não peça a chave
mestra: ela abre nove arquivos e ninguém precisa dos nove.

Se não tem, **pare e peça**, dizendo qual arquivo e quais chaves. Não tente
recriar segredo em serviço externo achando que "falta criar" — quase sempre já
existe no cofre.

Nunca imprima valor de credencial em log ou em resposta. Classifique por
formato, tamanho ou por um teste contra a API — nunca exibindo. Credencial que
apareceu em claro precisa ser rotacionada.

## Publicação

Preview **nunca** é público — fica atrás do Cloudflare Access. Promoção é da
**versão já construída** (`wrangler versions deploy`), não rebuild a partir da
tag. O que foi revisado é o que vai ao ar.

## Antes de dizer que terminou

- Os checks estão verdes? `startup_failure` não é "vermelho normal" — é arquivo
  recusado na compilação, e o log não existe.
- Você provou, ou presumiu? Doc desatualizado já enganou agente aqui mais de uma
  vez. **A realidade ganha do documento** — e o documento é corrigido no mesmo
  trabalho.
- Sobrou container, processo ou arquivo temporário na máquina do dono?

O mapa completo — inventário, credenciais, armadilhas medidas — está em
`jacu-dev/infra/docs/WORKSPACE.md`, para quem alcança aquele repositório.
