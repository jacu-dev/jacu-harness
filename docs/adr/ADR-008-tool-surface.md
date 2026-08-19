# ADR-008: MCP tool-surface granularity

- Status: accepted
- Date: 2026-08-11
- Decider: Erick

## Context

Giving every future capability its own tool contradicted the first-order
ceiling and grew schema, context and attack surface. Consolidations had to be
decided before later SDDs added tools.

## Decision

1. `jacu_status` is the canonical tool for workspace and task state.
   `jacu_workspace_status` is a compatibility alias; both call the same
   contract and do not create a distinct capability.
2. `jacu_verify` takes `run_id` and optional `argv`. Absent `argv`, it verifies
   the mission; present, it runs one diagnosis. Allowlist, bounded executor,
   environment and verdict are the same. `jacu_run_command` is not registered.
3. The program is input to `jacu_mission_compile` and its state is projected by
   `jacu_status`. There is no `jacu_program` and no `jacu_task_status`.
4. E1 reports, statusline and the memory bridge are internal projections, not
   new MCP tools. An E6 flow, if an eval justifies it, may use a single
   submission tool and counts as one new tool.

Catalogue ceiling: at most 14 tools; the `tools/list` ratchet is 20 KiB.
Any later addition must update this decision and the literal E2E census before
merge.

## Consequences

- Older hosts keep calling `jacu_workspace_status`; new hosts learn
  `jacu_status`.
- Removing the MCP registration of `jacu_run_command` avoids duplicating the
  execution contract; skills, specs, E2E and docs must follow.
- The internal name `RunCommand` may later be deleted without changing this
  architectural decision.
