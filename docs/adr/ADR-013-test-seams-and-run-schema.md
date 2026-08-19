# ADR-013: Git seam, mutation allowlist and runstate version

- Status: accepted
- Date: 2026-08-11
- Decider: Erick

## Decision

1. The `gitx` process boundary is injectable through a minimal interface. The
   production adapter stays direct, without a shell, with a clean Git
   environment.
2. Mutation testing is an explicit, allowlisted gate only for
   `missioncompile`, `runtime` and `runstate`. Acceptance is killing a real
   mutant, not hitting a coverage percentage.
3. `runstate.Run` persists `schema_version: "1"`. Absence is a compatible
   implicit migration; an unknown version is a fail-closed error.

## Consequences

- Previously unreachable Git failures can be tested without relaxing
  expectations.
- Mutation cost stays predictable and does not block the workspace package.
- Future format changes have an explicit rejection point instead of silently
  treating old runs as accepted JSON with the wrong semantics.
