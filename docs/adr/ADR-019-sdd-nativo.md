# ADR-019: Native SDD — spec/design/tasks without an external authoring product

- Status: accepted
- Date: 2026-08-13
- Ratified: 2026-08-13
- Decider: Erick

> **Amendment (SDD-001 T2.5):** the layout is a **single file** `sdd.md` plus
> generated `sdd.lock.json`, and the CLI is `new|lint|status`. Where this ADR
> still describes `spec.md` / `design.md` / `tasks.md` / `STATUS.md` —
> including finding codes `sdd_open_decision` and
> `sdd_status_done_without_e2e` — it is superseded by
> `docs/sdd/001-native-sdd/sdd.md`, the executed contract.

## Context

JACU has no SDD-authoring capability. `jacu_mission_compile` is a
deterministic compiler from intent to a mission contract. It is a gate, not a
document. It never produces spec, design, ADR or a task list.

The host writes prose; the binary validates. JACU does not call a model. SDD
authorship that lived in an external product is replaced by a versioned rite
in this repository plus deterministic Go lint.

This ADR does not revoke the product rule against competing in spec-authoring
markets. The change is an internal repository rite plus lint — the same
distinction as hygiene documentation versus a hygiene product.

## Decision

Bring SDD into the repository as a **versioned rite plus deterministic Go
lint exposed as a CLI subcommand**. No new MCP tool. No external authoring
dependency.

1. **Canonical layout — one file per change:**

   ```
   docs/sdd/<NNN>-<slug>/
     sdd.md            # Why · Locked decisions · Out of scope · Write scope ·
                       # Requirements · Tasks · Done
     sdd.lock.json     # generated: ids, hashes, deltas, Verify commands
   docs/sdd/specs/<capability>/spec.md
   docs/sdd/archive/<date>-<slug>/
   docs/sdd/CONVENTIONS.md
   ```

   **One file, not four.** Splitting into `spec.md`/`design.md`/`tasks.md`/
   `STATUS.md` would reimport the layout this change removes.

   ADRs stay in `docs/adr/` and are **referenced** from Locked decisions in
   `sdd.md`, never duplicated.

2. **Language lock:** every text an LLM reads is technical English — spec,
   design, tasks, status, commits, branches, PRs. PT-BR only in live owner
   conversation and in existing plan/ADR documents whose established language
   is PT-BR.

3. **The host writes prose; JACU validates.** The binary gains
   `jacu sdd new|lint|status`, a CLI subcommand with the same precedent as
   `jacu stats` (useful diagnosis, no MCP tool).

   `sdd archive` does **not** enter: archiving is `git mv` plus lock
   regeneration. It stays a follow-up until a manual flow proves the need.

4. **The host reaches lint through `jacu_verify` with `argv`**, authorized in
   `.jacu/verify-allowlist.json` with `required_arg_prefix: ["sdd"]`. Zero new
   tools, zero catalogue bytes, and lint enters CI like any other gate.

5. **Skill `jacu-sdd`** (English, ≤100 lines) plus one new route in
   `using-jacu`.

6. **Delta model preserved:** `ADDED`/`MODIFIED`/`REMOVED` per requirement.
   Deltas only narrow write scope; they never loosen it.

7. **External authoring layout leaves by move, not rewrite:** existing
   Requirement/Scenario files move into `docs/sdd/`.

### Lint findings (stable contract)

Typed finding `{code, severity, target, message}` — **one lint**. Severity
reuses `BLOCK | WARN | INFO`; `blocked` and the presence of a BLOCK always
coincide, as in `jacu_mission_compile`.

| Severity | Code | Fires when |
|---|---|---|
| BLOCK | `sdd_missing_section` | required section missing from `sdd.md` |
| BLOCK | `sdd_task_without_verify` | task row with empty Verify column |
| BLOCK | `sdd_open_decision` | open decision remains while the change requires closure |
| BLOCK | `sdd_out_of_scope_touched` | diff touches a path declared in Out of scope |
| BLOCK | `sdd_git_state_unavailable` | Git state needed to compute the diff is unavailable |
| BLOCK | `sdd_stale_lock` | content hash ≠ hash in `sdd.lock.json` |
| BLOCK | `sdd_done_without_evidence` | task marked done without pasted evidence (command + output) |
| BLOCK | `sdd_status_done_without_e2e` | Done claimed without a green E2E line |
| WARN | `sdd_requirement_without_scenario` | `### Requirement:` without any `#### Scenario:` |
| WARN | `sdd_locked_decision_without_adr` | Locked decisions without an ADR link |
| WARN | `sdd_task_without_red` | implementation task without a prior RED task |
| WARN | `sdd_language_not_english` | tokenized PT-BR heuristic; **never blocks** |
| INFO | `sdd_delta_summary` | count of ADDED/MODIFIED/REMOVED |

Deterministic identity in the same mould as `mission_id`:
`spec_id = "spc_" + hex(sha256(json(normalized))[:8])`,
`sdd_id = "sdd_" + ...`. Whitespace, order and duplicates do not change
identity; content does.

### Options (chosen: D)

- **A — skill and templates only:** no enforcement; documents drift.
- **B — `sdd` field inside `jacu_mission_compile`:** that tool is `safe`,
  read-only, closed-world and never touches files; inflating its schema
  spends catalogue headroom mixing “compile a mission” with “lint documents”.
- **C — new MCP tool `jacu_sdd`:** spends the last slot under the ceiling of
  14 and a large schema. The 16 KiB output cap without an artifact store
  still returns structure and hashes, not the document.
- **D — Go capability exposed by CLI, reached via `jacu_verify` (chosen):**
  deterministic enforcement, zero new tool, zero catalogue bytes. The host
  reaches a command, not a tool; discovery depends on the skill. Promoting D
  to C later is cheap (register a handler). C to D is expensive.

The document does not fit the MCP envelope, so every option returns
structure, ids and findings while the document lives on disk. The host writes
technical English; the binary never authors. D→C is reversible; start there.

## Consequences

Easier: write technical English with machine-checked form; close a phase on
the same `verify` that already fails an orphan skill; audit via
`sdd.lock.json`; leave external authoring by `git mv`.

Harder: claim Done without evidence; open a change outside the layout; write
agent-facing documents in PT-BR (WARN).

Revisit if a triggering eval shows the host cannot reach the CLI: promote to
option C with a successor ADR. An artifact store remains a separate pending
decision and does not block this one.
