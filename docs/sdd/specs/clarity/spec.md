# clarity Specification

## Purpose
Gate a living SDD by compiling a closed readback, ingesting host answers,
comparing them to the spec, and refusing variance or a growing rewrite.
JACU never runs the probe model.

## Requirements
### Requirement: The readback is a closed structure
The system SHALL accept a readback only as JSON with keys `write_scope`,
`forbidden_paths`, `requirements`, `out_of_scope`, and `tasks`. Prose and
unknown fields SHALL fail with a typed code and SHALL not record a round.

#### Scenario: a prose readback is refused
- **WHEN** ingest receives narrative text
- **THEN** the finding is `prose_readback` and no `clarity.probe` event is emitted

#### Scenario: an unknown field is refused
- **WHEN** the object carries a key the schema does not declare
- **THEN** ingestion fails with `unknown_field`

### Requirement: Divergence is reported per field
Comparison SHALL reuse the SDD parser. A path outside declared write scope
SHALL name `write_scope` and that path. An agreeing readback SHALL report
zero divergences.

#### Scenario: a misread write scope is named
- **WHEN** the readback lists a path outside the spec write scope
- **THEN** `divergence_field` is `write_scope`

### Requirement: Variance across three runs decides the verdict
Three normalized readbacks that disagree on a field SHALL fail with
`variance_runs` even when each would match the spec alone.

#### Scenario: three mutually inconsistent readbacks fail
- **WHEN** `jacu clarity verdict` receives three disagreeing readbacks
- **THEN** the verdict is `fail` and `variance_runs` is 3

### Requirement: The rewrite loop cannot grow the spec
A later round whose spec byte count exceeds the previous round SHALL be
refused with `spec_bytes_delta` positive.

#### Scenario: a longer rewrite is refused
- **WHEN** `--previous-spec-bytes` is smaller than the current spec
- **THEN** ingest exits 1 with `spec_bytes_delta`

### Requirement: CLI surface
`jacu clarity probe|ingest|verdict` SHALL accept `--json`. Exit 0 is pass,
1 is a gate failure, 2 is usage. Diagnostics stay on stderr.
