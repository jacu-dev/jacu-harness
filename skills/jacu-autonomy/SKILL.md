---
name: jacu-autonomy
description: "Use for a mission program, policy-gated auto-apply, batch review, remediation, or an autonomy escalation."
---

# Run an autonomous program

Use this skill when the host has a bounded program of missions and the project
has opted into `.jacu/autonomy-policy.json`.

1. Compile one `jacu_mission_compile` request with a `program` object. Resolve
   every interview item first: `open_questions` must be an explicit empty list.
   Each nested mission keeps its own allowed/forbidden paths, verification
   commands, risk hint, and `after` dependencies.
2. Keep the program's `program_id` and mission ids. Review the whole batch with
   `jacu_diff` only after each mission's `jacu_verify` verdict and evidence are
   available. A program is not a reason to skip the ordinary digest gate.
3. Auto-apply is literal: only verdict `pass` qualifies. `fail`, `timeout`,
   `blocked`, and `not_run` all refuse. The derived risk tier is one of
   `safe`, `write`, or `destructive`, and must be within `risk_max`.
4. `cross_review` means a valid one-use HMAC receipt for this run and digest.
   The receipt proves what this local runtime emitted; it does not prove that
   reviewer and executor were different sessions. Never invent that claim.
5. Call `jacu_apply` only after the policy requirements and receipt pass. The
   runtime commits the run branch, opens a PR to `main`, and arms auto-merge
   with required checks. The machine never creates, moves, or deletes a `v*`
   production tag.
6. If a mission escalates, stop that mission and preserve its worktree, receipt,
   diff, and audit package. Continue independent missions; do not stop the
   whole program. A dependent mission waits for its prerequisites.
7. For a failed CI check, collect the evidence, classify it, rerun a flaky
   check once, then create a separate policy-gated remediation mission. A
   second failure of the same check after correction escalates to Erick.
   A dedicated CI-status MCP tool is intentionally absent: the MCP byte cap is
   already full, so the bounded runner observes the PR with direct `gh` calls.
   Pending checks are not failures; remediation paths come only from safe
   relative annotations, and a missing/unsafe scope escalates.
8. Keep GitHub logs as bounded evidence: retain digests and safe metadata in
   the audit, never paste credentials or unbounded log output into a mission.
9. Report each mission's objective, diff digest, verdict, evidence digest,
   receipt reference, iterations, warnings, and escalation state. Real-host
   behavior checks remain pending until Erick runs the prepared eval sheet.
