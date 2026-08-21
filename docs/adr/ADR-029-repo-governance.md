# ADR-029 — Repository-local governance seams

- Status: accepted
- Date: 2026-08-21
- Decider: Erick
- SDD: 010-repo-governance

## Context

CODEOWNERS (ADR-007) is GitHub merge enforcement. It cannot stop `jacu_apply`
from committing a protected path inside a worktree. Four local seams were still
open: protected paths, shared `JACU_HOME` writes across runs, `jacu_status`
taking the workspace mutation gate, and unbounded flow wave width.

## Decision

1. **Protected paths.** Apply loads `.jacu/protected.json`:
   `{"paths":["glob",...]}`. A reviewed file matching any entry blocks apply
   (`protected path: <path>`). Missing file means an empty list. Malformed
   JSON, unknown fields, or an unreadable file fail closed. CODEOWNERS is not
   this list.
2. **Per-run home.** Each open run owns
   `<JACU_HOME|/home>/.jacu-harness/run-homes/<project_id>/<run_id>/`.
   Verify's toolchain `HOME` is `<run-home>/toolchain-home`, not a
   project-wide directory. Two open runs do not share those writes.
3. **Status is not a writer.** `jacu_status` / `jacu workspace status` do not
   acquire the workspace mutation gate. Apply may hold the gate while status
   still returns.
4. **Fan-out cap.** A scheduled flow wave wider than **4** nodes blocks with
   finding `fan_out` and does not start the extra nodes.

## Consequences

- Harness paths GitHub already covers can also be named in `protected.json`
  when the kernel must refuse them.
- Owner ratifies this ADR separately from the code landing.
