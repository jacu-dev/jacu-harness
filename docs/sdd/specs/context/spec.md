# context Specification

## Purpose
Admit repository context for a host without adding an MCP tool. SDD-011 owns
`jacu context --sdd`. SDD-006 owns admission, ledger and pack.

## Requirements
### Requirement: context --sdd admits the active living SDD
`jacu context --sdd` SHALL emit the active living native SDD path
(`docs/sdd/<NNN>-<slug>/sdd.md`) and the document body. Active means a living
SDD whose lock has a `doing` task, otherwise the PROGRAM queue row marked
`doing` or `next`. `--json` SHALL print `{path, document}` on stdout.
Diagnostics stay on stderr. Missing `--sdd` is usage (exit 2). No active
living SDD is `no_active_sdd` (exit 1). Two lock-`doing` SDDs is
`multiple_active_sdd` (exit 1).

#### Scenario: a living SDD is active
- **WHEN** `--sdd` runs and one living SDD is active
- **THEN** stdout names that path and includes the document

#### Scenario: no active SDD
- **WHEN** `--sdd` runs and no living SDD is active
- **THEN** the command exits 1 with `context: no_active_sdd:` on stderr and
  writes nothing to stdout

### Requirement: The pack is deterministic
`jacu context pack` SHALL emit a byte-identical pack for the same mission
and repository state. Items SHALL be ordered required-first then path.
The digest SHALL include content hashes.

#### Scenario: two runs produce identical bytes
- **WHEN** the same mission is packed twice against an unchanged repository
- **THEN** the two packs are byte-identical, including item order

### Requirement: The ledger refuses before dispatch
The ledger SHALL decide `admit`, `refuse`, or `degrade` before any tool
call. Required overflow is `refuse` with `required_overflow` true.

#### Scenario: a required item that does not fit refuses the task
- **WHEN** a required item exceeds the remaining budget
- **THEN** the decision is `refuse` and no tool call is made

### Requirement: Anchor preservation is proven
Every mission anchor SHALL appear in the pack. `coverage_bps` SHALL use
`items_required` and `items_included`.

