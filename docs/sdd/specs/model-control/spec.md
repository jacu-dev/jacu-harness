## Purpose

The SDD-9 implementation is a deterministic host-profile control plane. It
classifies tasks without a model, routes only externally attested host-native
CLIs, demotes unreliable profiles, escalates bounded failures, and protects
invocation with a circuit breaker. It never reads credentials or calls a
provider API.

## Requirements

### Requirement: host-profile control is deterministic and fail-closed

The implementation SHALL expose the executable requirements in the active
`add-model-control` change and SHALL not introduce provider API, OAuth,
credential, or MCP-tool dependencies.

#### Scenario: active change carries executable contract

- **WHEN** the canonical spec is inspected
- **THEN** it points to the active change, whose delta contains scenarios for
  classification, routing, resilience and cost evidence

See the active change `add-model-control` for the full executable scenarios;
the change is archived only after CI and the roadmap diary are complete.
