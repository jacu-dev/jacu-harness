---
name: jacu-sdd
description: "Use when a change needs a native JACU SDD scaffold, structural lint, lock refresh, or status check."
---

# Native SDD

The native SDD rite is a repository contract. The host writes the prose; the
binary only parses, lints, identifies, and gates it.

1. Start a change with `jacu sdd new <lowercase-slug>`. It creates the next
   `docs/sdd/<NNN>-<slug>/sdd.md` from the repository template. Then create
   the integration branch for it from `main`: `git switch -c sdd/<NNN> main`.
   Every run of this SDD opens from that branch's HEAD and merges back into it
   locally. The SDD document itself is the first commit on the branch.
   A `full` ceremony SDD declares an `## Entregas` section: each delivery is
   the smallest set of tasks that leaves `main` working and delivers
   something observable, and names what goes red if it is wrong. One pull
   request per delivery, from `sdd/<NNN>` to `main`. A task is never a merge
   point; a pull request that does not close a declared delivery is a
   fragment and must not be opened.
2. Read the change's `sdd.md`, its referenced ADRs, and its declared scope.
   Do not invent prose, widen scope, or duplicate an ADR inside the SDD.
3. Run `jacu sdd lint` after edits. Use `--all` for the corpus and
   `--json` when a host needs typed `{code, severity, target, message}` findings.
4. A BLOCK is a stop. Fix the document or the implementation that caused it;
   never branch on diagnostic message text or weaken the rule.
5. Use `jacu sdd lint --write-lock` only after reviewing the hand-authored
   document. The generated `sdd.lock.json` contains identity, hash, deltas,
   and Verify commands, not prose.
6. `jacu sdd status` is a summary, not proof. Task Verify output belongs in
   the task Evidence cell, and human gates remain human gates.
7. `jacu context --sdd` (or `--json`) admits the active living SDD path and
   document. It is CLI-only. A typed `no_active_sdd` on stderr means stop.
8. Before executing a living SDD, run `jacu clarity probe --sdd <path> --json`,
   have the host fill the closed JSON, then `ingest` three answers and
   `verdict`. A fail is a stop. Do not grow the spec to pass the gate.
9. `jacu sdd close <directory>` is the final verification step. It refuses
   unfinished tasks, missing evidence, lint BLOCKs, or a missing manual archive;
   it never performs the archive move. It does run the clean-exit removal of
   JACU-owned leftovers (worktrees, receipts) and fails if that removal
   fails; it never deletes user-created paths.
10. After the last delivery merges into `main`, delete `sdd/<NNN>` locally and
   on `origin`. An integration branch that outlives its SDD is drift.

Archive is currently a manual `git mv` plus lock regeneration. The CLI does
not close or archive a change.
