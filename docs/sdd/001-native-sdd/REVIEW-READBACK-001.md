# SDD-001 read-back review — owner answers

Read-back accepted. Both ambiguities are real and stopping was correct. Both resolve
in favour of SDD-001; `ADR-019` is the stale document.

## Ambiguity 1 — layout: `sdd.md` + `sdd.lock.json` wins

`ADR-019` contradicts itself. Its Context describes the lost jacu-code rite as
`.jacu/specs/<NNN>-<slug>/sdd.md` — a single hand-authored file — and its Decision
item 1 then lists `spec.md/design.md/tasks.md/STATUS.md`, which is the OpenSpec layout
this very change removes. `docs/sdd/001-native-sdd/sdd.md` is the contract, it is
newer, and it is what exists on disk. Single-file layout stands.

## Ambiguity 2 — `archive` is out of scope for SDD-001

T18 (`new|lint|status`) is correct. Archiving is `git mv` plus a lock regeneration;
adding a subcommand for it widens CLI surface with no eval proving the need, against
the standing rule that nothing ships without an eval proving necessity. `ADR-019`
action item 5 promises a subcommand this phase does not deliver.

## Required correction — amend the ADR, do not let code diverge

Add one task to SDD-001 before T3:

- Amend `docs/adr/ADR-019-sdd-nativo.md`: Decision item 1 becomes the single-file
  layout (`sdd.md` + `sdd.lock.json`); action item 5 becomes `new|lint|status`, with
  `archive` moved to an explicit follow-up.
- Verify: `jacu sdd lint` reports zero BLOCK for `001-native-sdd` after the
  amendment. Not `human read`.
- Record in §7 Follow-ups: `sdd archive` as a subcommand stays deferred until an eval
  shows the manual flow hurts.

`ADR-019` is `Status: Proposto` and its action item 1 is ratification, so amending
before ratification is legitimate — it is not rewriting history. Report back after the
amendment: ratification is the owner's.

## T1 and T2 — `human read` is not a rubber stamp

Both pass the lint because the Verify column is non-empty, but they produce the
templates the other 27 tasks depend on. When you reach them, send the template content
for a real read before moving to T3.

## T23 — `NEEDS-ERICK`

Correct to stop. Prepare everything up to it, leave the eval document ready, and list
exactly what needs the owner present.

Proceed with T1.
