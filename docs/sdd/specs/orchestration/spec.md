# orchestration Specification

## Purpose

Define the declarative flow capability and its deterministic scheduling
contract. A flow sequences existing capabilities without adding a shell,
provider, or policy bypass.

## Requirements

### Requirement: Closed flow graph

The orchestration capability SHALL accept one flow value containing nodes and
edges. A node SHALL use only `mission`, `workspace`, `verify`, `review`, or
`apply`; node ids SHALL be non-empty and unique; every edge SHALL reference
existing nodes. Unknown capabilities, duplicate ids, dangling edges, malformed
conditions, and unbounded cycles SHALL block before any node executes.

#### Scenario: Invalid graph blocks before execution

- **WHEN** a flow contains an unknown capability or dangling edge
- **THEN** validation returns `blocked` and no node executes

### Requirement: Closed edge conditions

An edge condition SHALL be `verdict == pass|fail|timeout|blocked|not_run`,
`policy == pass|require_approval|blocked|escalate`, or `default`. Verify
outgoing edges SHALL cover every verdict or provide `default`. `not_run` and a
verify `default` SHALL never route to `apply`.

#### Scenario: Not-run cannot reach apply

- **WHEN** an edge routes `verdict == not_run` to an apply node
- **THEN** validation returns a blocking finding

### Requirement: Deterministic waves

The scheduler SHALL use iterative Kahn ordering with lexical tie-breaking,
wait for the latest predecessor, and assign independent nodes to the earliest
non-conflicting wave. Empty scope, bare glob, and any unknown scope SHALL
conflict with every other scope; equal paths and directory prefixes SHALL
conflict; sibling prefixes SHALL not conflict. Dangling edges and cycles SHALL
be returned as scheduling errors.

#### Scenario: Disjoint path scopes share a wave

- **WHEN** two independent nodes declare `src/a` and `src/b`
- **THEN** one lexical wave contains both nodes

### Requirement: Flow fan-out is capped

A scheduled DAG wave SHALL contain at most 4 nodes. A wider wave SHALL block
with finding `fan_out` before any of those nodes execute. Bounded cycles are
not DAG waves and keep their existing walker.

#### Scenario: a wave exceeds the cap

- **WHEN** a flow wave would exceed the cap
- **THEN** the flow blocks with a typed finding and does not spawn the extra nodes

### Requirement: Capability governance

Every executed node SHALL delegate to the existing capability seam. The flow
SHALL preserve verify verdicts, autonomy policy, reviewed-digest checks and
receipt requirements. A policy escalation SHALL stop the flow and preserve its
trace; the flow SHALL not create an alternate apply path.

#### Scenario: Apply keeps the ordinary gate

- **WHEN** an apply node runs without the reviewed digest or required receipt
- **THEN** the flow escalates with the capability refusal and does not bypass it

### Requirement: Task and projection

An asynchronous flow SHALL persist through the task runtime and be readable via
`jacu_status`. The result SHALL contain waves, an ordered node trace and a
deterministic Markdown panel projection. Duplicate review session ids SHALL not
count twice, and the panel SHALL never itself authorize apply.

#### Scenario: Async result is pollable

- **WHEN** a valid flow is submitted with `async: true`
- **THEN** `jacu_status` returns its terminal trace and projection
