#!/usr/bin/env python3
"""Confere que um pull request não enfraquece a própria esteira.

Roda a partir do guardião (`pull_request_target`), ou seja, sempre a versão da
branch default — um pull request que altere este arquivo não altera o que
executa.

Ele **lê** o conteúdo dos workflows no HEAD do pull request pela API. Ler é
seguro; executar não seria. Nada daqui materializa ou roda código do PR.
"""

import json
import os
import subprocess
import sys

import yaml

REPO = os.environ["REPO"]
PR = os.environ["PR"]
HEAD_SHA = os.environ["HEAD_SHA"]

ESTEIRA = (".github/workflows/", ".github/actions/")


def gh(*args: str) -> str:
    return subprocess.run(
        ["gh", *args], capture_output=True, text=True, check=True
    ).stdout


def erro(arquivo: str, msg: str) -> None:
    print(f"::error file={arquivo}::{msg}")


def main() -> int:
    arquivos = json.loads(
        gh("api", "--paginate", f"/repos/{REPO}/pulls/{PR}/files",
           "--jq", "[.[] | {nome: .filename, estado: .status}]")
    )
    tocados = [a for a in arquivos if a["nome"].startswith(ESTEIRA)]

    if not tocados:
        print("o pull request não toca a esteira — nada a conferir")
        return 0

    print("arquivos de esteira neste pull request:")
    for a in tocados:
        print(f"  {a['estado']:9} {a['nome']}")
    print()

    # Os nomes que o ruleset EXIGE, lidos da API. Hardcodar aqui deixaria o
    # portão dessincronizado da regra que ele existe para proteger.
    branch = gh("api", f"/repos/{REPO}", "--jq", ".default_branch").strip()
    contextos = json.loads(
        gh("api", f"/repos/{REPO}/rules/branches/{branch}",
           "--jq", "[.[] | select(.type == \"required_status_checks\")"
                   " | .parameters.required_status_checks[].context]")
    )
    # O check de um job que chama reusable aparece como "job / job"; o que
    # precisa existir no YAML é o primeiro segmento.
    exigidos = sorted({c.split(" / ")[0] for c in contextos})

    print(f"jobs que o ruleset exige existir: {', '.join(exigidos) or '(nenhum)'}")
    print()

    erros = 0

    # O próprio guardião não pode ser apagado por um pull request.
    for a in tocados:
        if a["nome"].endswith("guardiao.yml") and a["estado"] == "removed":
            erro(a["nome"], "este pull request apaga o próprio portão")
            erros += 1

    for a in tocados:
        nome, estado = a["nome"], a["estado"]
        if estado == "removed" or not nome.endswith((".yml", ".yaml")):
            continue

        try:
            bruto = gh("api", f"/repos/{REPO}/contents/{nome}?ref={HEAD_SHA}",
                       "--jq", ".content")
        except subprocess.CalledProcessError:
            erro(nome, "não consegui ler este arquivo no head do pull request")
            erros += 1
            continue

        import base64
        texto = base64.b64decode(bruto).decode("utf-8", "replace")

        try:
            doc = yaml.safe_load(texto) or {}
        except yaml.YAMLError as e:
            erro(nome, f"YAML inválido: {e}")
            erros += 1
            continue

        jobs = doc.get("jobs") or {}

        # 1. Job exigido pelo ruleset não pode sumir nem ser renomeado. O
        #    GitHub até barra o merge sozinho nesse caso, mas em silêncio: o
        #    check não fica vermelho, fica AUSENTE, e o pull request espera para
        #    sempre sem dizer por quê. Um erro explícito vale mais.
        #
        #    A comparação é contra a versão da BRANCH DEFAULT, que está em disco
        #    por causa do checkout — só acusa quem *tirou* algo que existia.
        base = {}
        if os.path.exists(nome):
            try:
                base = (yaml.safe_load(open(nome)) or {}).get("jobs") or {}
            except yaml.YAMLError:
                base = {}

        for exigido in exigidos:
            if exigido in base and exigido not in jobs:
                erro(nome, f"o job '{exigido}' sumiu deste arquivo, e é exigido "
                           "pelo ruleset — o check não fica vermelho, fica "
                           "ausente, e o merge espera para sempre")
                erros += 1

            if exigido not in jobs:
                continue

            # 2. A regra do `needs` vale só para o AGREGADOR, não para todo job
            #    exigido: `verify` é job de trabalho e não deve depender de quem
            #    o agrega. Quem é agregador se descobre pela versão da branch
            #    default — é o job exigido que já tinha `needs`.
            def lista(v):
                if isinstance(v, str):
                    return [v]
                return list(v or [])

            era_agregador = bool(lista((base.get(exigido) or {}).get("needs")))
            if not era_agregador:
                continue

            irmaos = [j for j in jobs if j != exigido]
            needs = lista(jobs[exigido].get("needs"))

            # Job fora do `needs` é job cujo resultado ninguém lê — e
            # `needs: []` é o caso extremo: desconecta o agregador de tudo e ele
            # fecha verde sem afirmar nada.
            fora = [j for j in irmaos if j not in needs]
            if irmaos and fora:
                erro(nome, f"o agregador '{exigido}' não tem no `needs`: "
                           f"{', '.join(sorted(fora))} — o resultado desses "
                           "jobs não é lido por ninguém"
                           + (" (o `needs` está vazio)" if not needs else ""))
                erros += 1

        for job, corpo in jobs.items():
            if not isinstance(corpo, dict):
                continue
            # 3. Portão com `continue-on-error` não é portão: falha vira verde.
            if corpo.get("continue-on-error") is True:
                erro(nome, f"o job '{job}' tem `continue-on-error: true` — "
                           "falha dele não reprova nada")
                erros += 1
            cond = corpo.get("if")
            if cond is False or str(cond).strip().lower() == "false":
                erro(nome, f"o job '{job}' tem `if: false` — ele nunca roda, "
                           "e job pulado conta como aprovado")
                erros += 1

        # 4. O guardião não pode ganhar condição nenhuma, nem passar a fazer
        #    checkout do código do pull request.
        if nome.endswith("guardiao.yml"):
            gatilho = doc.get(True) or doc.get("on") or {}
            alvo = gatilho.get("pull_request_target") or {}
            if isinstance(alvo, dict) and ("paths" in alvo or "paths-ignore" in alvo):
                erro(nome, "o guardião ganhou `paths:`/`paths-ignore:` — "
                           "um pull request passa a poder pular o portão, e "
                           "job pulado conta como aprovado")
                erros += 1
            for job, corpo in jobs.items():
                if isinstance(corpo, dict) and "if" in corpo:
                    erro(nome, f"o job '{job}' do guardião ganhou `if:` — "
                               "job pulado conta como aprovado")
                    erros += 1
            for proibido in ("head.sha", "head.ref", "github.head_ref"):
                if proibido in texto:
                    erro(nome, f"o guardião referencia `{proibido}` — "
                               "`pull_request_target` roda com token de escrita "
                               "e não pode materializar código do pull request")
                    erros += 1

    if erros:
        print()
        print("::error title=A esteira não pode ser enfraquecida por um pull "
              "request::corrija os pontos acima.")
        return 1

    print("a esteira continua intacta neste pull request")
    return 0


if __name__ == "__main__":
    sys.exit(main())
