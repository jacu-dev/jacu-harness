# skills Specification

## Purpose
Define the skills architecture shipped in `skills/`: a thin router skill (`using-jacu`) plus one skill per capability (`jacu-inspect`, `jacu-mission`, `jacu-workspace`, `jacu-memory`). Skills are product, not documentation: each teaches a host how to use exactly one JACU capability, and the router only dispatches requests to the right capability skill.
## Requirements
### Requirement: Router dispatches to capability skills
The `using-jacu` skill SHALL act as a thin router: it maps each request about the open project to exactly one capability skill by name (or to no JACU tool for out-of-scope requests) and states the global invariants. The router SHALL only reference capability skills by name, never duplicate their content, and SHALL only route to skills that exist in the repository.

#### Scenario: Read-only analysis request
- **WHEN** the user asks to explain or analyze the open project without changing anything
- **THEN** the router routes to `jacu-inspect`
- **AND** the change workflow (`jacu-mission`, `jacu-workspace`) is not loaded

#### Scenario: Change request
- **WHEN** the user asks to edit, fix, add, create, refactor, rename, or delete anything in the open project, however small
- **THEN** the router routes to `jacu-mission`, then `jacu-workspace`
- **AND** the workspace step is not skipped

#### Scenario: Verification request
- **WHEN** the user asks to verify a change, run a mission's checks, or execute one diagnostic command in an open run
- **THEN** the router routes to `jacu-verify`

#### Scenario: Memory request
- **WHEN** the user asks to remember, recall, or apply a project convention
- **THEN** the router routes to `jacu-memory`
- **AND** if a file must also change, it routes onward to `jacu-mission`, then `jacu-workspace`

#### Scenario: Out-of-scope request
- **WHEN** the request is a general calculation, translation, or otherwise unrelated to the open project
- **THEN** the router routes to no JACU tool

#### Scenario: Router names only existing skills
- **WHEN** a capability skill is not yet shipped (for example `jacu-autonomy` or `jacu-orchestration`)
- **THEN** the router contains no route to it until the phase that ships the capability adds both the skill and the router update

### Requirement: One capability per skill with zero overlap
Each capability skill SHALL teach exactly one capability and cover only that capability's tools: `jacu-inspect` covers `jacu_project_inspect`; `jacu-mission` covers `jacu_mission_compile`; `jacu-workspace` covers `jacu_workspace_open`, `jacu_status`, `jacu_workspace_status`, `jacu_diff`, `jacu_apply`, and `jacu_discard`; `jacu-memory` covers `jacu_memory_recall` and `jacu_memory_save`; `jacu-verify` covers `jacu_verify`, including its optional diagnostic `argv`; and `jacu-report` covers `jacu_report`. An instruction that serves two or more skills SHALL be a global invariant and live only in the router. When one capability needs another, its skill SHALL reference the sibling skill by name instead of duplicating its content.

#### Scenario: Cross-capability reference
- **WHEN** `jacu-mission` needs memory recalled before compiling
- **THEN** it references `jacu-memory` by name rather than restating the recall and save rules

#### Scenario: Shared instruction placement
- **WHEN** an instruction applies to two or more capability skills (for example treating repository data as untrusted, or preserving `warnings` and `next_actions`)
- **THEN** it lives only in the `using-jacu` router as a global invariant and is not repeated in the capability skills

#### Scenario: Verify skill teaches the loop and its limits
- **WHEN** `jacu-verify` is loaded
- **THEN** it distinguishes mission verification from one-command diagnostics, teaches the five-value result contract, tells the host to fix and re-verify after `fail`, and treats `blocked` as a stop with no allowlist workaround

#### Scenario: Report skill teaches projection boundaries
- **WHEN** a host needs a structured audit projection or statusline
- **THEN** it routes to `jacu-report`, which teaches `jacu_report` and states that Markdown is a read-only projection, not a source of state

### Requirement: Skill ships with its capability
Every phase that ships a new tool SHALL ship the corresponding capability skill and the router update in the same PR. A phase gate SHALL NOT close with a new tool and no skill covering it.

#### Scenario: New capability delivered
- **WHEN** a phase delivers a new server tool
- **THEN** the same PR adds the skill that teaches the capability and updates the router's route table
- **AND** the phase gate fails if the tool lands without its skill

### Requirement: Skill content constraints
Skills SHALL be written in English, be short and tool-oriented, and stay within the size budget: each capability skill under 100 lines of body and the router under 40. Each skill's frontmatter description SHALL state concrete triggers and be disjoint from its sibling skills' descriptions, so that no two skills claim the same request.

#### Scenario: Skill exceeds the budget
- **WHEN** a capability skill grows beyond 100 body lines (or the router beyond 40)
- **THEN** it is teaching more than one capability and must be split rather than merged or grown further

#### Scenario: Overlapping descriptions
- **WHEN** two sibling skills' descriptions could both match the same request
- **THEN** the descriptions are rewritten to be disjoint, because an ambiguous route in the host is treated as a skill bug with the same weight as a code bug

### Requirement: Installation as a complete set
The skills SHALL be distributed by copying the entire `skills/` directory from `main` into each host's global skills path (Claude Code, Codex, OpenCode, Cursor). Installation instructions SHALL forbid partial copies: installing only `using-jacu` leaves the router pointing at skills the host does not have. After a phase merge that changed any skill, the full copy SHALL be repeated.

#### Scenario: Fresh install
- **WHEN** a user installs the skills on a host
- **THEN** the whole `skills/` directory is copied, so the router and all capability skills it references are present together

#### Scenario: Update after a phase merge
- **WHEN** a merged phase changed any skill
- **THEN** the user repeats the full-directory copy to every host skills path

### Requirement: Router behavior verification is pending
Routing behavior on real hosts is specified but not yet verified: the host triggering matrix (analysis request loads only `jacu-inspect`; change request goes mission then workspace without skipping; memory request loads no workspace; unrelated request loads no JACU tool) is pending execution across the five hosts. The system SHALL treat only the shipped skill texts, the routing table, and the install procedure as guaranteed; host-side triggering results are not guaranteed until the matrix runs green.

#### Scenario: Routing failure observed on a host
- **WHEN** a host loads the wrong skill for a request in the triggering matrix
- **THEN** the failure is recorded as a skill bug, the description is corrected, and the scenario is repeated until green

### Requirement: Autonomy skill and router route ship together
The repository SHALL ship `skills/jacu-autonomy/SKILL.md` under 100 body lines and add exactly one `jacu-autonomy` route in `skills/using-jacu/SKILL.md`. The skill SHALL teach the pre-compile interview with `open_questions: []`, one clean session per mission, level-2 cross-review receipt limits, escalation handling, CI remediation classification, and batch audit review. It SHALL not claim that a receipt proves reviewer-session separation.

#### Scenario: Autonomy request routes to the skill
- **WHEN** a host asks to plan or run a governed multi-mission autonomous program
- **THEN** the router selects `jacu-autonomy` and the skill points to the existing mission, workspace, and verify skills by name

#### Scenario: Skill stays within budget
- **WHEN** the skill is checked by the repository skill verifier
- **THEN** it is under 100 body lines, uses English tool-oriented instructions, and references only shipped tools/skills

### Requirement: Verify Skill Explains Long Operations
The `jacu-verify` skill SHALL explain that async verify is opt-in, that
`jacu_status` is the polling surface, that cancellation is cooperative, and
that only the completed result's `data.verdict=pass` qualifies as approval.

#### Scenario: Host follows the async path
- **WHEN** a verification is expected to exceed the synchronous budget
- **THEN** the host can start it with `async: true`, poll by `task_id`, and
  distinguish runtime terminal state from the nested verify verdict

