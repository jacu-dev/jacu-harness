# ADR-015: Declarative flow with contract waves

- Status: accepted
- Date: 2026-08-11
- Decider: Erick

## Context

Orchestration must sequence capabilities without a family of tools per node.
A mined contract already defines deterministic `schedule_waves` by path
independence. The E5 task runtime already offers the persistent cycle needed.

## Decision

1. Register a single tool `jacu_flow_run`. The graph is JSON validated before
   any execution; long mode uses the E5 task store.
2. Port `schedule_waves` by contract: iterative Kahn, lexical order, latest
   predecessor and conservative scope conflict. Empty scope, a pure glob and
   doubt do not parallelize.
3. Close conditions to equality on `verdict`/`policy` and `default`. The five
   verdicts `pass`, `fail`, `timeout`, `blocked` and `not_run` must be
   covered; `not_run` never reaches `apply`.
4. Each node still calls the existing capability. The flow does not create a
   shell, policy bypass, receipt or alternate apply.
5. The multi-model panel is a deterministic Markdown projection of structured
   opinions. JACU does not call models and does not store credentials.

## Consequences

- The surface grows by one tool and stays under the absolute ceiling of 14.
- Independent DAGs can advance in waves; operations without known scope stay
  serialized.
- Cycles need `max_visits` and a bounded walker; unbounded cycles are blocked
  before execute.
- Quorum and model routing are not delivered implicitly; they depend on eval
  and the SDD-009 ADR.
