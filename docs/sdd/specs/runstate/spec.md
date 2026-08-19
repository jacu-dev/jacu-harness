# runstate Specification

## Purpose
Define the exclusive interprocess coordination required for runstate mutations
and workspace lifecycle critical sections.
## Requirements
### Requirement: Interprocess mutation lock
Runstate mutation SHALL acquire an exclusive lock at `<repo>/.git/jacu/runs.lock`.
The lock SHALL cover the read/validate/write sequence of a Save and the
open/diff/apply/discard critical sections.

#### Scenario: Concurrent Save observes serialized state
- **WHEN** one process holds the run lock while saving a valid transition and a
  second process saves a transition for the same run
- **THEN** the second process waits, observes the first persisted state, and
  either applies a valid next transition or returns a transition error without
  corrupting the JSON state

### Requirement: Versioned Runstate

Every persisted run SHALL carry `schema_version: "1"`. A legacy run without
the field SHALL be read as version 1 and SHALL receive the field on the next
save. An unknown schema version SHALL be rejected fail-closed.

#### Scenario: Legacy run is migrated on save

- **WHEN** a valid run JSON has no `schema_version`
- **THEN** load accepts it as version 1 and the next save persists `"1"`

#### Scenario: Unknown version is blocked

- **WHEN** a run JSON has an unsupported schema version
- **THEN** load returns an error and the run is not treated as valid state

