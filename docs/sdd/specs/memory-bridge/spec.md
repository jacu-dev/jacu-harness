## Purpose

Define the E8 memory-to-rules projection, eval-gated promotion and explicit
threshold for a future FTS5 migration without changing the current JSON store.

## Requirements

### Requirement: memory bridge contracts are active

The implementation SHALL satisfy the executable scenarios in the archived
`add-memory-bridge` change after CI and archive complete.

#### Scenario: canonical contract remains discoverable

- **WHEN** the OpenSpec catalog is validated
- **THEN** the memory bridge spec and its change delta are structurally valid
