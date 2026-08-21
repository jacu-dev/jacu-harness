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
