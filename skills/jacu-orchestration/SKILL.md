---
name: jacu-orchestration
description: "Use for a bounded multi-step flow with declarative edges, independent path waves, or a Markdown review panel."
---

# Run a declarative flow

Use `jacu_flow_run` or `jacu flow --json --input '{...}'` when one mission has real dependent steps. A single
mission should use the ordinary mission/workspace/verify path; a graph is not
free complexity.

1. Describe `flow.nodes` with one of the existing capabilities: `mission`,
   `workspace`, `verify`, `review`, or `apply`. Put capability input in `with`
   and declare `allowed_paths` conservatively.
2. Connect nodes with closed conditions only: `verdict == pass|fail|timeout|
   blocked|not_run`, `policy == pass|require_approval|blocked|escalate`, or
   `default`. A verify graph must cover every verdict (or use `default`).
3. Never route `not_run` or an unsafe default to `apply`. Apply still requires
   its normal reviewed digest, verification and autonomy receipt policy.
4. Start long flows with `async: true`; poll `jacu_status` at a measured
   interval and cancel with the task id through the normal task path.
5. Use exact references such as `$node.compile.mission_id` only after the
   referenced node has completed. A missing reference blocks the flow.
6. Read `trace`, `waves`, and `panel_markdown` as evidence. The panel is a
   deterministic projection of structured opinions; it does not select models,
   create credentials, or approve an apply.
7. Cycles require `max_visits`; an unbounded cycle is rejected before any node
   runs. Unknown or empty path scope is serialized conservatively.
