# JACU SDD Conventions

## Language
All agent-facing SDD, spec, design, task, status, template, commit, branch, PR, skill, and CLI text is technical English. PT-BR is reserved for owner conversation and existing plan or ADR documents whose established language is PT-BR.

## Layout
Each change lives in `docs/sdd/<NNN>-<slug>/` with a hand-authored `sdd.md` and generated `sdd.lock.json`. Living capability specifications live in `docs/sdd/specs/<capability>/spec.md`. Closed changes live in `docs/sdd/archive/<YYYY-MM-DD>-<slug>/`. ADRs live in `docs/adr/` and are referenced by `sdd.md`, never duplicated there.

## Deltas
Requirement deltas use exactly `ADDED`, `MODIFIED`, or `REMOVED`. A delta may narrow write scope but never widen it.

## Archive
Close a change with manual `git mv` into `docs/sdd/archive/<YYYY-MM-DD>-<slug>/`, regenerate its lock, update living capability specs, and record the execution report in `docs/relatorios/`. SDD-001 does not provide an `sdd archive` command.

## Executor
The ten executor rules are defined once in `docs/sdd/EXECUTOR.md`; executors must follow that document and it remains the source of truth. Per-change trap lists live in that change's own folder and never restate the rules.

## Spec authoring
Requirements transcribe exact limits, enums, orderings and message strings from the Go code; the code and its tests are the source of truth, not the plan documents. Every tool requirement states its behaviour inside the shared envelope instead of respecifying the envelope. Mutating behaviour requires at least one security scenario: deny-by-default, identity validation, or scope enforcement. A new dependency requires an ADR.

## Design approval
Decisions the owner must approve before implementation are recorded in the change's Open decisions section. Design approval is the entry gate of the change.

## Amendments
An authorized amendment to a governing document invalidates its lock by design; regenerate the lock and continue. The lock records which document version was executed against, not an immutable seal.
