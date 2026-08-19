# ADR-014: Application-owned task runtime for verify

- Status: accepted
- Date: 2026-08-11
- Decider: Erick

## Context

Synchronous `jacu_verify` of the race suite is slow enough that hosts need an
async path. The Go MCP SDK used by the server does not offer the Tasks API
needed to depend on native negotiation, and ADR-008 already consolidated
state onto `jacu_status`.

## Decision

1. `jacu_verify` accepts optional `async`, `task_id` and `cancel`. Async is
   opt-in; the synchronous path stays compatible.
2. `jacu_status` accepts optional `task_id` and projects task state and
   result. There is no `jacu_task_status`.
3. Tasks persist under `.git/jacu/tasks`, are versioned, protected by the
   runstate lock and written by atomic rename.
4. The queue is FIFO. Default limit is one active execution. States `queued`,
   `running`, `done`, `failed`, `cancelled` and `timeout` have forward-only
   transitions.
5. The executor is the existing `verify`; cancellation reuses its process
   group and never introduces a shell or a second environment policy.

## Consequences

- Hosts without MCP Tasks can start, query, cancel and recover tasks.
- Verification result and runtime state have separate semantics.
- There is one extra persisted directory, without changing run lifecycle.
- A later native API can be adapted without invalidating the persisted
  fallback.
- The terminal payload stays available for 24 hours, then is compacted
  physically while keeping identity, status and digest. Compacted metadata is
  retained for 30 days and capped at 1,000 records. Active and corrupted
  tasks are never removed automatically.
