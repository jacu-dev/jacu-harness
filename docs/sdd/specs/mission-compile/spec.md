# mission-compile Specification

## Purpose
Provide the `jacu_mission_compile` MCP tool: a pure, deterministic compiler that turns a stated intent (objective, criteria, verification commands, path scopes, risk hint) into a linted mission contract — or refuses to. It never modifies files: it normalizes the input, classifies the required ceremony (`direct` | `light` | `full`), runs lint rules (`BLOCK` | `WARN` | `INFO`), and either blocks compilation or emits a mission with a content-derived stable ID. The tool executes through the shared capability runtime envelope (specified separately) with spec: risk `safe`, read-only, idempotent, closed-world, 10s timeout, 256 KiB max input, 16 KiB max output.
## Requirements
### Requirement: Input Contract
The tool SHALL accept a JSON object with: `objective` (string), optional `context` (`project_id` string, `refs` array of strings), optional `acceptance_criteria` (array of strings), optional `verification_commands` (array of argv arrays — each command is an array of strings, never a shell string), optional `allowed_paths` and `forbidden_paths` (arrays of strings), and optional `risk_hint` declared in the input schema as an enum `safe | write | destructive`. Malformed input JSON SHALL cause the handler to fail (surfaced by the envelope as status `failed`); any input that decodes SHALL compile to status `ok` or `blocked`, never a crash.

#### Scenario: Any decodable input yields ok or blocked
- **WHEN** the tool receives arbitrary JSON that decodes into the input schema (including empty `{}`, a 1 MiB objective within the input cap, or unicode/control characters in the objective)
- **THEN** compilation returns status `ok` or `blocked` and the result serializes to a valid JSON envelope with `data` as an object

#### Scenario: Enum advertised in tools/list
- **WHEN** a client lists tools and inspects `jacu_mission_compile`'s input schema
- **THEN** `risk_hint` carries `"enum": ["safe", "write", "destructive"]`

### Requirement: Input Normalization
Before classification, hashing, and linting, the compiler SHALL normalize the input: trim whitespace from `objective`; and for each of `acceptance_criteria`, `allowed_paths`, and `forbidden_paths`, trim each entry, remove duplicates, and sort the list lexicographically (nil lists stay nil). `verification_commands` SHALL be passed through unmodified. The emitted mission SHALL carry the normalized values.

#### Scenario: Trim, dedupe, and sort
- **WHEN** the input has objective `"  Fix the API bug  "`, criteria `[" tests pass ", "bug fixed", "tests pass"]`, allowed paths `[" internal/api ", "cmd", "cmd"]`, and forbidden paths `["secrets", " .env ", "secrets"]`
- **THEN** the normalized values are objective `"Fix the API bug"`, criteria `["bug fixed", "tests pass"]`, allowed paths `["cmd", "internal/api"]`, forbidden paths `[".env", "secrets"]`

### Requirement: Deterministic Mission Identity
The mission ID SHALL be `"msn_"` followed by the lowercase hex encoding of the first 8 bytes of the SHA-256 digest of the JSON encoding of the normalized input. Inputs that differ only in whitespace, list ordering, or duplicate entries SHALL produce the same mission ID; inputs with different content SHALL produce different IDs.

#### Scenario: Order and whitespace do not change identity
- **WHEN** two inputs have the same content but different list ordering, duplicates, and surrounding spaces
- **THEN** both compile to the same `mission_id`

#### Scenario: Different content changes identity
- **WHEN** two inputs have different objectives (e.g. "Fix the API bug" vs "Add the API feature")
- **THEN** they compile to different `mission_id` values

### Requirement: Ceremony Classification
The compiler SHALL classify ceremony on the normalized input, evaluated in this order:
1. `direct` — when `allowed_paths` is empty AND `verification_commands` is empty AND the risk hint is absent or `safe` AND the objective contains no mutation verb. Mutation verbs (case-insensitive, matched on letter/digit word boundaries) are: `add`, `create`, `fix`, `refactor`, `remove`, `rename`, `adicionar`, `corrigir`, `criar`, `refatorar`, `remover`, `renomear`.
2. `full` — otherwise, when `risk_hint` is `destructive`, OR there are 3 or more acceptance criteria, OR the allowed paths are broad: any entry equal to `.` or `**`, or 3 or more distinct entries.
3. `light` — in all remaining cases.

#### Scenario: Read-only intent is direct
- **WHEN** the objective is "Explain how this project works" with no paths, no verification commands, and risk hint absent or `safe`
- **THEN** the ceremony is `direct`

#### Scenario: Full ceremony triggers
- **WHEN** the input has risk hint `destructive`, or exactly three acceptance criteria, or allowed paths `["."]`, or `["**"]`, or three distinct directories like `["cmd", "internal", "pkg"]`
- **THEN** the ceremony is `full`

#### Scenario: Light ceremony
- **WHEN** the objective contains a mutation verb (e.g. "Fix the parser output now") with no paths and no full-ceremony trigger
- **THEN** the ceremony is `light`

#### Scenario: Invalid risk hint is non-safe but not full
- **WHEN** the objective is read-only but `risk_hint` is an unknown value like `banana`
- **THEN** the ceremony is `light` (classification; the invalid hint is handled by lint)

### Requirement: BLOCK Lint Rules
The compiler SHALL emit `BLOCK`-level lint items for:
- `empty_objective` (field `objective`) — objective is empty after trimming.
- `shell_string_command` (field `verification_commands`) — a command with exactly one element containing whitespace (space, tab, CR, or LF); commands must be argv arrays.
- `shell_interpreter_command` (field `verification_commands`) — a command of 2+ elements whose first element's base name (lowercased) is `sh`, `bash`, `zsh`, or `cmd.exe` and any later argument equals `-c` or `/c` case-insensitively.
- `path_outside_root` (field `allowed_paths` or `forbidden_paths`) — a path that does not stay within the resolved project root: relative paths are joined to the resolved root and cleaned; the result must not escape the root, and if the path exists its symlinks are resolved and the resolved target must also stay within the root.
- `allowed_forbidden_overlap` (field `allowed_paths`) — a cleaned path present in both `allowed_paths` and `forbidden_paths`.

An invalid `risk_hint` SHALL NOT block: a malformed hint is never load-bearing and is handled by the WARN rules.

#### Scenario: Empty objective
- **WHEN** the objective is only whitespace
- **THEN** lint contains BLOCK `empty_objective` with message "objective is required"

#### Scenario: Shell-string command
- **WHEN** `verification_commands` contains `["go test ./..."]` as a single string
- **THEN** lint contains BLOCK `shell_string_command` with message "shell-string command; use argv array"

#### Scenario: Shell interpreter command
- **WHEN** `verification_commands` contains `["sh", "-c", "rm -rf /"]`, `["bash", "-c", ...]`, `["zsh", "-c", ...]`, or `["cmd.exe", "/C", ...]`
- **THEN** lint contains BLOCK `shell_interpreter_command` with message "shell interpreter command is not allowed"

#### Scenario: Path escapes the root
- **WHEN** an allowed path is `../outside`, an absolute path outside the root, or an existing symlink whose target lies outside the root
- **THEN** lint contains BLOCK `path_outside_root` with message "path must stay within project root"

#### Scenario: Allowed and forbidden overlap
- **WHEN** `internal/parser` appears in both `allowed_paths` and `forbidden_paths`
- **THEN** lint contains BLOCK `allowed_forbidden_overlap` with message "path cannot be both allowed and forbidden"

#### Scenario: Invalid risk hint
- **WHEN** an out-of-enum `risk_hint` reaches the compiler behind schema validation
- **THEN** it produces no BLOCK item for `risk_hint`, warns that the hint was ignored, and still compiles

### Requirement: WARN and INFO Lint Rules
The compiler SHALL emit `WARN`-level lint items for:
- `full_without_criteria` (field `acceptance_criteria`) — ceremony is `full` and no acceptance criteria were provided.
- `full_without_verification` (field `verification_commands`) — ceremony is `full` and no verification commands were provided.
- `ambiguous_objective` (field `objective`) — the objective has fewer than 4 words (split on non-letter/non-digit runes, lowercased) or contains no action verb from: `add`, `change`, `correct`, `create`, `fix`, `implement`, `refactor`, `remove`, `rename`, `update`, `adicionar`, `alterar`, `atualizar`, `corrigir`, `criar`, `implementar`, `refatorar`, `remover`, `renomear`.
- `project_id_mismatch` (field `context.project_id`) — `context.project_id` is set and does not match the project root's computed ID.
- `invalid_risk_hint_ignored` (field `risk_hint`) — the hint did not decode into the enum; it is ignored and risk is derived from mission content.
- `weak_risk_hint_ignored` (field `risk_hint`) — the hint is weaker than the derived risk; derived risk prevails.

And one `INFO`-level item:
- `risk_defaulted` (field `risk`) — `risk_hint` is absent and the ceremony is `light` or `full`, noting the derived value.

A fully specified input (clear objective, criteria, argv verification commands, in-root paths, valid risk hint) SHALL produce no lint items.

#### Scenario: Full ceremony missing criteria or verification
- **WHEN** the ceremony is `full` and acceptance criteria (or verification commands) are absent
- **THEN** lint contains WARN `full_without_criteria` (or WARN `full_without_verification`)

#### Scenario: Ambiguous objective
- **WHEN** the objective is "Fix parser" (fewer than four words) or "The parser output is incorrect" (no action verb)
- **THEN** lint contains WARN `ambiguous_objective`

#### Scenario: Project ID mismatch
- **WHEN** `context.project_id` is `prj_wrong` for the current root
- **THEN** lint contains WARN `project_id_mismatch`

#### Scenario: Invalid hint is advisory
- **WHEN** an out-of-enum `risk_hint` reaches the compiler
- **THEN** lint contains WARN `invalid_risk_hint_ignored`, risk is derived from content, and status is `ok`

#### Scenario: Risk defaulted
- **WHEN** `risk_hint` is absent on a `light` or `full` mission
- **THEN** lint contains INFO `risk_defaulted` and the mission uses the derived risk

#### Scenario: Weaker hint ignored
- **WHEN** derived risk is `destructive` and `risk_hint` is `write`
- **THEN** mission risk is `destructive` and lint contains WARN `weak_risk_hint_ignored`

#### Scenario: Happy path has no lint
- **WHEN** the input has a clear four-word objective with an action verb, criteria, argv verification commands, in-root allowed/forbidden paths, and `risk_hint: "write"`
- **THEN** the lint list is empty

### Requirement: Blocked Compilation Yields No Mission Identity
When any lint item has level `BLOCK`, the compiler SHALL return status `blocked` with an empty mission that carries the complete lint list but no `mission_id`, no `ceremony`, and empty (not null) criteria, command, and path lists; `next_actions` SHALL be empty. Status `blocked` and the presence of a BLOCK lint item SHALL always coincide.

#### Scenario: Blocked mission is inert
- **WHEN** any BLOCK rule fires (e.g. a shell interpreter command)
- **THEN** the status is `blocked`, `mission_id` and `ceremony` are empty strings, all list fields are empty arrays, and the lint list explains every violation

#### Scenario: BLOCK and blocked are equivalent
- **WHEN** compilation completes for any input
- **THEN** the status is `blocked` if and only if the lint list contains at least one BLOCK item

### Requirement: Direct Ceremony Produces No Mission
When the ceremony is `direct` (and nothing blocked), the compiler SHALL return status `ok` with an empty mission whose `ceremony` is `direct` and no `mission_id`, plus the lint list; `next_actions` SHALL be exactly `["host answer suffices; answer the user directly"]` and the envelope summary SHALL be "No mission required; host answer suffices."

#### Scenario: Direct short-circuit
- **WHEN** the input classifies as `direct`
- **THEN** the status is `ok`, the mission has `ceremony: "direct"` and empty `mission_id`, and next actions tell the host to answer the user directly

### Requirement: Compiled Mission Contract
For `light` and `full` ceremonies with no BLOCK lint, the compiler SHALL return status `ok` and a mission containing: the deterministic `mission_id`; the `ceremony`; the normalized `objective`, `acceptance_criteria`, `verification_commands`, `allowed_paths`, and `forbidden_paths` (as fresh copies); a `risk` computed as `max(derived, hint)`, where derived risk comes from mission content and a valid hint can only raise it (risk enum: `safe` | `write` | `destructive`); and the full lint list. If any WARN lint is present, `next_actions` SHALL contain exactly one entry: "refine the fields identified by WARN lint and resubmit"; otherwise it SHALL be empty. The envelope summary SHALL be "Mission compiled."

#### Scenario: Mission with defaulted risk
- **WHEN** a `light` mission compiles without a risk hint
- **THEN** the mission's `risk` is the derived value and lint carries INFO `risk_defaulted`

#### Scenario: Hint raises risk
- **WHEN** derived risk is `write` and `risk_hint` is `destructive`
- **THEN** mission risk is `destructive` with no warning about the hint

#### Scenario: Incident regression — invalid hint on destructive cleanup
- **WHEN** `risk_hint: "high"` is injected past schema validation with a destructive cleanup objective
- **THEN** status is `ok`, lint warns that the hint was ignored, and derived risk is `destructive`

#### Scenario: WARN prompts refinement
- **WHEN** a compiled mission carries one or more WARN lint items
- **THEN** `next_actions` is exactly `["refine the fields identified by WARN lint and resubmit"]`

### Requirement: Output Contract
Results SHALL be returned inside the shared capability envelope (`status`, `summary`, `data`, `artifacts`, `warnings`, `next_actions`, `trace_id` — specified separately) with `data` carrying the mission object (`mission_id`, `ceremony`, `objective`, `acceptance_criteria`, `verification_commands`, `allowed_paths`, `forbidden_paths`, `risk`, `lint`; each lint item has `level`, `rule`, `message`, and optional `field`). `artifacts` and `warnings` SHALL be empty arrays. The compiler itself only produces statuses `ok` and `blocked`; the envelope may additionally surface `failed` (decode error) or `blocked` for input over the 256 KiB cap. Summaries are exactly "Mission compiled.", "Mission compilation blocked.", or "No mission required; host answer suffices."

#### Scenario: Envelope round-trips as JSON
- **WHEN** any compilation result is wrapped in the envelope
- **THEN** it serializes and re-parses as valid JSON with `data` as an object and status in {`ok`, `blocked`, `failed`}

### Requirement: Program input is compiled as part of a mission
`jacu_mission_compile` SHALL accept an optional `program` object with `open_questions` (an array of strings) and `missions` (an ordered array of mission inputs, each with an optional `after` array of zero-based mission indexes). Every nested mission SHALL be compiled by the same mission compiler rules, and the result SHALL include deterministic `program_id`, ordered nested `mission_ids`, and the normalized program data without adding a new MCP tool.

#### Scenario: Valid program compiles in order
- **WHEN** a program contains valid missions with `open_questions: []` and acyclic `after` indexes
- **THEN** compilation returns `status: "ok"` with a stable `program_id`, nested mission IDs, and the declared order

#### Scenario: Open question blocks compilation
- **WHEN** `program.open_questions` contains any non-empty question
- **THEN** compilation returns `status: "blocked"` with a BLOCK lint identifying the open question gate

#### Scenario: Invalid nested mission blocks compilation
- **WHEN** any nested mission fails ordinary mission compilation
- **THEN** compilation returns `status: "blocked"` with the nested index and lint reason, and no program is executable

#### Scenario: Cyclic ordering blocks compilation
- **WHEN** the `after` indexes contain a cycle or an out-of-range index
- **THEN** compilation returns `status: "blocked"` with a deterministic dependency lint

### Requirement: Program identity is content-derived
The `program_id` SHALL be `prg_` followed by the first 8 bytes of SHA-256 over normalized program JSON; list whitespace, duplicate normalization, and ordering differences that do not change dependencies SHALL not change the identity, while mission content or dependency changes SHALL change it.

#### Scenario: Equivalent normalized programs share identity
- **WHEN** two program inputs differ only in surrounding whitespace or duplicate list entries inside nested missions
- **THEN** they produce the same `program_id`

### Requirement: Plan Mode

`jacu_mission_compile` SHALL treat `program.open_questions` as the typed plan
decision list. The program SHALL be executable only when
`open_questions == []`; the host owns the decision questions and choices.

#### Scenario: Open decisions block execution

- **WHEN** a program contains one or more open questions
- **THEN** compilation returns `status: "blocked"` with BLOCK
  `program_open_questions` and no executable mission id

#### Scenario: Closed plan is ready

- **WHEN** `program.open_questions` is empty and the mission queue is valid
- **THEN** compilation returns `status: "ok"` and `PlanReady(program)` is true

#### Scenario: Invalid decision is fail-closed

- **WHEN** an open question is present, including an empty normalized question
- **THEN** compilation returns `status: "blocked"` and no executable mission id

