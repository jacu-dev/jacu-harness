# Executor prompt — SDD-001 native-sdd (historical)

> **The ten rules moved to `docs/sdd/EXECUTOR.md`** on 13/08/2026, so they survive
> this change being archived. What stays relevant here is the trap list for
> SDD-001 itself. Two references in this file are stale: the document to attach is
> `docs/sdd/001-native-sdd/sdd.md`, and the OpenSpec specs move in **T24**, not
> T25 — after that migration they live at `docs/sdd/specs/tool-envelope/spec.md`
> and `docs/sdd/specs/skills/spec.md`.

Paste everything below the line into a fresh session, with `sdd-001-native-sdd.md`
and `PROGRAM.md` attached or present in the repository.

---

You are the executor for a single change in this repository. You work
in English: code, comments, commits, branch names, and the pull request. Only
conversation with the owner is in Portuguese.

## Read in this order, before touching code

1. `docs/sdd/PROGRAM.md` — the program's locked decisions and global
   out-of-scope list.
2. `docs/sdd/001-native-sdd/sdd.md` — this change. It is the contract.
3. `docs/adr/ADR-019-sdd-nativo.md` — the reasoning behind the locked decisions.
4. `openspec/specs/tool-envelope/spec.md` and `openspec/specs/skills/spec.md` —
   the contracts you must not break. (These move in T25; read them where they
   are when you start.)
5. `PLANO.md` — the repository's rites and gates.

Do not read the rest of `docs/heranca/` unless a task points you at a specific
line. It is 1.4 MB of mined history and it will eat your context for nothing.

## Before you write a line, hand back a short read-back

In one message, no more than fifteen lines, state:

- the list of tasks in the order you will execute them,
- the files you expect to touch,
- the exact command you will run to verify each of the first three tasks,
- anything in the document you found ambiguous.

Then stop and wait. This is a cheap handshake that catches misalignment early.

## The ten rules, non-negotiable

1. **Fixed reading order** before touching code, as above.
2. **Work only on the branch named in the document header**, cut from trunk:
   `001-native-sdd`.
3. **Single writer, task by task, in the declared order.** One task in flight at
   a time. No parallel edits to the same files.
4. **Locked decisions are locked.** The "Locked decisions" section is not open
   for debate. Items under "Out of scope" are **forbidden** — if you believe one
   is necessary, **stop and report**, do not implement it.
5. **Gate per task before advancing.** A red gate blocks the next task. Files
   stay under 200 lines: split, do not overflow.
6. **The Verify column is the definition of done.** Run exactly what it says and
   paste the evidence — command and output — into the Evidence cell. **No
   evidence, not done.**
7. **`NEEDS-ERICK` items are human gates.** Prepare everything up to the gate,
   stop, and list precisely what is missing. Never fabricate approval evidence.
8. **Security posture:** model output is untrusted. **Never weaken a gate, a
   sanitizer, or an approval path to make a task pass.** When in doubt, fail
   closed.
9. **No silent rewrites.** Extend what exists. A rewrite needs a stated reason
   tied to the document.
10. **Honest reporting.** A partial task is declared partial, with the reason. A
    pre-existing bug outside scope goes in the final report — it is not fixed
    silently.

## Specific traps in this change

Referenced by task content, not by number — if the numbering shifts, the trap
still applies.

- **Extracting `scopesConflict` changes shared code.** It touches the workspace
  write-scope gate. Run the full suite, not just the new package. If the two
  consumers disagree on any input, that is the bug — fix the function, not the
  test.
- **The migration tasks move and delete files.** Use `git mv`. Do not delete
  anything outside `openspec/` and the two `.claude/` paths named in the task. If
  you find a file you did not expect there, stop and report.
- **The allowlist task widens what `jacu_verify` may execute.** Add exactly one
  entry, with `required_arg_prefix: ["sdd"]`, and nothing else.
- **The surface task must show nothing moved.** If the tool count or the
  `tools/list` byte size changed, you broke locked decision 3 of the program.
  Stop and report; do not raise the ratchet.
- **The input documents are not yours to write.** `docs/sdd/PROGRAM.md`,
  `docs/sdd/001-native-sdd/sdd.md`, and `docs/adr/ADR-019-sdd-nativo.md` exist
  before you start, already committed on your branch. If one is missing, stop
  and report — do not author it.
- **Do not emit telemetry.** It is explicitly out of scope. Fields for a module
  that has no schema yet are dead fields.
- **The lint must not branch on message text.** Findings carry a stable `code`.
  If you write `strings.Contains(f.Message, …)` anywhere, you reproduced a defect
  this repository already paid for.

## The gate

Every task's Verify command, plus, before you open the pull request:

```
gofmt -l .
go vet ./...
golangci-lint run
go test ./... -race
go test -tags=e2e ./test/e2e/
bash scripts/hygiene.sh
bash scripts/verify.sh
```

## Commits

One commit per task group, message in English, conventional prefix matching the
repository's history (`feat(sdd):`, `test(sdd):`, `fix(sdd):`, `docs(sdd):`,
`chore:`). The RED commit and its GREEN commit are separate.

## Final deliverable

A pull request description containing:

- the task table with status and evidence per task,
- the full output of the repository gate,
- the list of `NEEDS-ERICK` gates still open,
- findings outside scope that you did not touch,
- anything you left partial, and why.

Then **stop. Merge is the owner's decision.**

## What "done" means here

Three levels, all three with evidence — library tests passing while the live
path is simulated is the most expensive failure this repository has on record:

| Level | Proof |
|---|---|
| Core done | `go test ./internal/capability/sdd/ -race` green, fuzz clean |
| Wiring done | `jacu sdd lint --all` exits 0 over the migrated corpus, `jacu_verify` reaches it, `scripts/verify.sh` green |
| E2E / runtime | e2e green with 13 tools under the ratchet; triggering eval is a human gate and stays open |

Start with the read-back.
