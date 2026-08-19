# ADR-011: Plan mode and bounded headless runner

- Status: accepted
- Date: 2026-08-11
- Decider: Erick

## Context

The roadmap must run programs through host CLIs in headless mode without
turning JACU into a generic shell, inheriting parent-process credentials or
accepting an ambiguous plan. The contract also requires process-group
cancellation, output tails and a digest.

## Decision

Plan mode reuses the program field frozen by ADR-008: `program.open_questions`
is the host's structured decision list and the mechanical gate is
`open_questions == []`. Visual interaction stays in the host and is not in v1.

The runner is an internal library with a single `HarnessProcess` port,
allowlisted providers (Claude and Codex), direct argv, prompt on stdin, a
positive environment of nine variables, a process group, and time and output
limits. The `run` CLI accepts only a persisted `run_id` and uses that run's
worktree; it does not expose an arbitrary command.

Every Git subprocess clears the nine `GIT_*` variables before controlled
internal overrides. API keys, interactive login, arbitrary network and shell
are outside the contract.

## Consequences

- Incomplete programs fail closed before consuming a paid session.
- The same executor can serve Claude and Codex without duplicating process
  policy.
- Output is auditable by status, tails, counts and digest, without returning
  an unbounded stream to the host.
- There is no local UI and no model router at this step; both need later
  decisions.
