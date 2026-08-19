## Purpose

Define reproducible signed distribution, verified installation and deterministic
host packs for SDD-11 without making tag creation or registry publication
machine-controlled.

## Requirements

### Requirement: distribution contracts are active

The implementation SHALL satisfy the executable scenarios in the archived
`add-distribution` change after the release dry-run and archive complete.

#### Scenario: release contract remains discoverable

- **WHEN** the OpenSpec catalog is validated
- **THEN** the distribution spec and its change delta are structurally valid
