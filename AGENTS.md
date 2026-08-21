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

## CI — o que quebra e por quê

**Runner hospedado do GitHub está BLOQUEADO.** A franquia estourou e jobs em
`ubuntu-latest` morrem em dois segundos com *"recent account payments have
failed"*, sem log e sem step. Isso vale para **todo** job, inclusive o
agregador. Nunca escreva `runs-on: ubuntu-latest`.

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
