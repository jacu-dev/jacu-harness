---
name: jacu-workspace
description: "Use after jacu-mission returns ceremony light or full to isolate a change, review its diff, apply the approved review, abandon the active run, or find which runs are already open."
---

# Work in an isolated workspace

A reviewed diff plus explicit human approval gates the apply.

1. If you do not already hold a `run_id` — a new session, or a session that
   lost it — call `jacu_status` (or `jacu workspace status --json`) before opening anything. The legacy alias
   `jacu_workspace_status` is equivalent. It takes no arguments and lists every
   run with its `run_id`, `status`, `age_seconds`,
   `disk_bytes`, `diff_lines`, and `base_behind`. Resume the run that matches
   the mission instead of opening a second one for the same work. A run
   reported as `corrupted` is not resumable: discard it.

2. Call `jacu_workspace_open` with the returned `mission_id` and the exact
   same `mission_input` used to compile it. An integrity mismatch is a hard
   stop. Keep the returned `run_id`, `branch`, `worktree_path`, and
   `base_sha` as the active run identity.

3. Every read, edit, generated file, and verification command for the mission
   must use the returned `worktree_path`, while respecting allowed and
   forbidden paths.

   **Never edit files outside the worktree_path while a run is open.**

4. Before reviewing, verify: `jacu-verify` runs the mission's checks in the
   worktree. A diff offered for approval without a verdict asks the human to be
   the test suite.

5. Call `jacu_diff` with the active `run_id`. Read the digest, files, scope
   classification, warnings, counts, and diff. Present the reviewed diff to the
   human, including truncation or scope warnings, then **STOP and request
   explicit approval**. If more edits are requested, make them only in
   `worktree_path`, call `jacu_diff` again, and present the new diff; any
   earlier approval is obsolete.

6. **Never call jacu_apply without the human explicitly approving the reviewed
   diff.**

   After approval of the latest reviewed diff, call `jacu_apply` with the
   active `run_id`. Set `approve_destructive: true` only when the human also
   explicitly approves destructive execution. If apply reports that the
   worktree changed after review, return to the diff step and request approval
   again.

7. Report the returned commit, warnings, and branch to the human for merge. Do
   not merge automatically. If the session produced a new decision or gotcha,
   use `jacu-memory` to offer durable knowledge.

8. When abandoning a run — scope rejected in review or mission abandoned —
   call `jacu_discard` with the active `run_id`; never leave an orphaned
   worktree.
