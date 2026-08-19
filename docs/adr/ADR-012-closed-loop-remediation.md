# ADR-012: CI observation and bounded runner remediation

- Status: accepted
- Date: 2026-08-11
- Decider: Erick

## Context

The runner already executes headless providers, but autonomy that only opened
a pull request with auto-merge had no real contract for why a check failed.
Free-text observation could turn pending, cancellation or a malicious path
into a fix mission.

## Decision

1. Do not create `jacu_ci_status`. The runner uses `gh` with direct argv and
   captures checks, logs and annotations with limits and a digest.
2. Classify typed evidence only. Remediation paths come from relative
   annotations and pass scope validation.
3. Flaky gets at most one rerun. A real failure opens a mission with a budget
   of one round; a second failure after the fix escalates.
4. Pending, collection failure, integration conflict and insufficient evidence
   preserve the worktree and escalate, without auto-apply or a policy bypass.

## Consequences

- The audit package can explain check, job, log-tail digest, safe annotations,
  classification, decision and budget without storing raw or unbounded output.
- The initial watcher is bounded; persistent resumable operation belongs to a
  later SDD.
- GitHub remains an untrusted external source for paths and comments; no
  comment authorizes a change by itself.
