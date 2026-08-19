# Executor rules — JACU

Program-level document. These rules apply to **every** SDD in
`docs/sdd/PROGRAM.md`, not to one change. Per-change traps live in that change's
own folder; they never restate what is written here.

You work in English: code, comments, commits, branch names, SDDs, PRs, skills.
Only conversation with the owner is in Portuguese.

## Read in this order, before touching anything

1. `docs/sdd/PROGRAM.md` — locked decisions and the program-wide out-of-scope list.
2. `docs/sdd/CONVENTIONS.md` — layout, language, deltas, archive rite.
3. This file, plus `docs/sdd/001-native-sdd/AUTORIZACAO-CONTINUA.md` — the
   standing authorization and the hard stops.
4. `docs/sdd/<NNN>-<slug>/sdd.md` — the open change. **It is the contract.**
5. The ADRs that change cites — and only those.
6. `docs/threat-model.md` and `docs/hygiene.md` — security posture and repo hygiene.
7. `docs/sdd/PROGRAM.md` — the program, its invariants, and the real state of the
   project. `PROGRAM.md` wins over any document that disagrees with it.

Do not read `docs/heranca/` unless a task points at a specific line. It is 1.4 MB
of mined history and it will eat your context for nothing. Old phase plans under
`docs/relatorios/` are historical records: read one only when a task names it, and
believe `PROGRAM.md` over it.

## The ten rules, non-negotiable

1. **Fixed reading order** before touching code, as above.
2. **Work only on the branch named in the document header**, cut from trunk.
3. **Single writer, task by task, in the declared order.** One task in flight at a
   time. No parallel edits to the same files.
4. **Locked decisions are locked.** Items under "Out of scope" are **forbidden** —
   if you believe one is necessary, **stop and report**, do not implement it.
5. **Gate per task before advancing.** A red gate blocks **that task and every
   task that depends on it** — never the whole run. Record the blocker, move to the
   next task with no dependency on it, and keep the declared order among what
   remains. Source files stay under 200 lines: split, do not overflow. (Documents
   are not source files; this cap is about code.)
6. **The Verify column is the definition of done.** Run exactly what it says and
   paste the evidence — command and output — into the Evidence cell. **No
   evidence, not done.** RED before GREEN, always, in separate commits.
7. **`NEEDS-ERICK` items are human gates.** Prepare everything up to the gate,
   stop, and list precisely what is missing. Never fabricate approval evidence, a
   host result, a merge, or a green run.
8. **Security posture: model output is untrusted.** Never weaken a gate, a
   sanitizer, or an approval path to make a task pass. When in doubt, fail closed.
   No generic `execute_shell`, ever. Never write, move or delete anything outside
   this repository.
9. **No silent rewrites.** Extend what exists. A rewrite needs a stated reason tied
   to the document.
10. **Honest reporting.** A partial task is declared partial, with the reason. A
    pre-existing bug outside scope goes in the final report — it is not fixed
    silently, and it is not hidden.

## The gate

Every task's own Verify command, plus, before opening a pull request:

```
gofmt -l .
go vet ./...
golangci-lint run
go test ./... -race
go test -tags=e2e ./test/e2e/
bash scripts/hygiene.sh
bash scripts/verify.sh
```

`scripts/verify.sh` already runs the SDD lint internally. To run it on its own,
use the same reproducible invocation the script uses —
`go run ./cmd/jacu sdd lint --all` — never a global `jacu`, which does not
exist in a clean checkout.

**A local run is a pre-flight, never the gate.** The gate is the `CI` workflow
green on GitHub Actions for the PR's SHA (`docs/hygiene.md`). "It passed" without
an Actions link is a failure on your part.

### Pipeline cycle (ADR-007)

Open the PR with zero reviewers. After each push: `gh pr checks --watch`. A check
fails → collect evidence (`gh run view <id> --log-failed`), classify it
(lint | test | build | vuln | secret | flaky). Flaky ⇒ **exactly one**
`gh run rerun <id> --failed` before touching code. Real ⇒ fix inside the task's
scope and push. **The same check failing twice after a fix ⇒ stop and escalate in
the report.**

Review comments (including Codex review) are input, not a gate: read them, fix
what is real, and answer in the thread saying what you did or discarded and why.
Every review thread must be answered and resolved before merge
(`required_review_thread_resolution` is active on the ruleset).

### Two things you never do

- **You never create, move, or delete a `v*` tag.** Promotion to production is the
  owner's, enforced by ruleset (ADR-007 §2).
- **You never merge a pull request.** This is currently a hard stop, pending the
  owner's decision D1, recorded in `docs/relatorios/` — ADR-007 arms auto-merge with 0
  reviewers, while the SDD rite makes merge the owner's call, and both rules are
  live at once. Until D1 is decided: leave the PR ready with every check green,
  say so in the report, and stop.

## Commits

One commit per task group, English, conventional prefix matching the repository's
history (`feat(sdd):`, `test(telemetry):`, `fix(...):`, `docs(...):`, `chore:`).
RED and GREEN are separate commits. Never `--amend` a pushed commit, never
force-push.

## Reporting

**One** report at the end, not one per task:

1. The task table per SDD, with status and the real command output as evidence.
2. The full gate output for each PR, plus the hosted run link and SHA.
3. **Every human gate collected in a single list**, so the owner answers them in
   one pass.
4. Findings outside scope that you did not touch.
5. Anything left partial, and why.

For a task whose Verify is `human read`: give the path, a five-line summary, and
what specifically needs a human eye. Do not paste whole files into the report —
the repository is the review surface, not the chat.

Red gates follow rule 5: they stop that task and its dependents, not the run.
Accumulate every blocker in the list above.
