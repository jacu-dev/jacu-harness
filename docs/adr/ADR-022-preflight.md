# ADR-022 — Governed pre-flight

- Status: proposed; owner ratification required
- Date: 2026-08-14
- Scope: SDD-004 pre-flight checks before mission dispatch

## Decision

JACU evaluates the eight predictable interruption classes before the first
mission tool call:

1. `allowlist`
2. `program_not_on_path`
3. `path_missing`
4. `path_not_writable`
5. `credential_absent`
6. `network_undeclared`
7. `doc_missing`
8. `open_questions`

Every result is typed. A check that cannot establish a safe pass is reported
as an unresolved gap and blocks dispatch. The pre-flight result is assembled
into one batch, so the human receives one interruption rather than a serial
sequence of questions.

The `allowlist` check reads the same policy source used by `jacu_verify`; it
does not maintain a second allowlist implementation. Environment checks test
presence and reachability only. They never read credential values or probe a
network.

Pre-flight emits `preflight.check` for the aggregate result and
`mission.interruption` when a mission stops for a human. Both events use the
telemetry v2 envelope and closed enums/counts only. Model-controlled names,
paths, command text, and free-form explanations are not written to telemetry.

The CLI is diagnostic and bounded: `jacu preflight` returns 0 for a clean
report, 1 for detected gaps, and 2 for invalid input or an unresolvable
request. It reports; it does not install programs, create directories, grant
credentials, or dispatch a mission.

## Consequences

- A mission with several predictable gaps stops once and presents one batch.
- Missing environment state is visible instead of being mistaken for a pass.
- The verifier and pre-flight cannot silently diverge on allowlist policy.
- Some checks require explicit mission declarations, so callers must provide
  enough structured data for a deterministic result.
- Owner ratification is required before this decision is considered final.

## Alternatives rejected

- Discovering gaps during execution: preserves serial human interruptions.
- Letting an unknown check pass: violates deny-by-default behavior.
- Asking a model to predict environment failures: makes the gate non-
  deterministic and outside the security boundary.
