# project-inspect Specification

## Purpose
Provide the `jacu_project_inspect` MCP tool: a read-only scan of the project root that reports detected languages, manifests, and test commands without modifying files and without leaking file contents. It gives agents a safe, bounded first look at a project (identity, stack, how to run tests) before any mission is compiled. The tool executes through the shared capability runtime envelope (specified separately) with spec: risk `safe`, read-only, idempotent, closed-world, 10s timeout, 256 KiB max input, 16 KiB max output.

## Requirements

### Requirement: Input Contract
The tool SHALL accept a JSON object with two optional fields: `include` (array of strings) and `max_files` (integer). `max_files` bounds the number of files counted during the scan: when absent or non-positive it SHALL default to 2000, and values above 10000 SHALL be clamped to 10000. The `include` field is accepted for forward compatibility but SHALL have no effect on scan behavior in this version. Malformed input JSON SHALL cause the handler to fail (surfaced by the envelope as status `failed`).

#### Scenario: Default file limit
- **WHEN** the tool is called with `{}` or with `max_files` <= 0
- **THEN** the scan uses a file limit of 2000

#### Scenario: Clamped file limit
- **WHEN** the tool is called with `max_files: 20000` on a project with more than 10000 files
- **THEN** the scan stops counting at 10000 files
- **AND** the summary reports `file_count: 10000` and `truncated: true`

#### Scenario: Malformed input never panics
- **WHEN** the tool receives arbitrary bytes that do not decode into the input schema (e.g. one megabyte of `x`)
- **THEN** the envelope returns status `failed` with a non-empty summary and a `tr_`-prefixed trace ID, never a crash

### Requirement: Stable Project Identity
The tool SHALL compute `project_id` as `"prj_"` followed by the lowercase hex encoding of the first 8 bytes of the SHA-256 digest of the project root's resolved absolute path (symlinks evaluated, then made absolute). The identity SHALL be deterministic: repeated scans of the same root produce the same `project_id`. `name` SHALL be the base name of the resolved root directory.

#### Scenario: Repeated scans yield the same identity
- **WHEN** the same project root is scanned twice
- **THEN** both summaries carry an identical `project_id` starting with `prj_`

#### Scenario: Symlinked root resolves to canonical identity
- **WHEN** the configured root is a symlink to the real project directory
- **THEN** `project_id` is derived from the resolved target path, matching the ID of the real directory

### Requirement: Language, Manifest, and Test Command Detection
The scan SHALL walk the file tree under the root and detect, per regular file: language `go` for files with extension `.go` or a file named `go.mod`; language `typescript` for extensions `.ts` or `.tsx`; manifest `go.mod` (which also adds test command `go test ./...`); manifest `package.json`. For `package.json`, the file content SHALL be parsed as JSON and the test command `npm test` added only when a `scripts.test` entry exists. Detected values SHALL be deduplicated and returned sorted alphabetically. An empty project SHALL yield a valid summary with empty (not null) `languages`, `manifests`, and `test_commands` arrays, `file_count` 0, and `truncated` false.

#### Scenario: Go project
- **WHEN** the root contains `go.mod` and `main.go`
- **THEN** the summary reports `languages: ["go"]`, `manifests: ["go.mod"]`, `test_commands: ["go test ./..."]` with no warnings

#### Scenario: Mixed Go and TypeScript project
- **WHEN** the root contains `go.mod`, `main.go`, `src/index.ts`, and a `package.json` whose `scripts` include `test`
- **THEN** the summary reports `languages: ["go", "typescript"]` and `manifests: ["go.mod", "package.json"]`
- **AND** `test_commands` contains both `go test ./...` and `npm test`, sorted

#### Scenario: Empty project
- **WHEN** an empty directory is scanned
- **THEN** the summary has a valid `project_id` and `name`, `file_count` 0, `truncated` false, and empty arrays (never null) for languages, manifests, and test commands

#### Scenario: Hostile file names
- **WHEN** the root contains files with names holding unicode, emoji, or newline characters (e.g. `ção 💥.go`)
- **THEN** the scan completes without error and still detects the `go` language

### Requirement: Manifest Read Limits
The scan SHALL NOT read a `package.json` larger than 256 KiB; it SHALL still list the manifest, skip script detection, and append the warning `package.json exceeds 256KB (not read)`. A `package.json` that fails JSON parsing SHALL append the warning `malformed package.json` without failing the scan.

#### Scenario: Oversized package.json
- **WHEN** the root contains a `package.json` larger than 256 KiB with a `scripts.test` entry
- **THEN** `manifests` includes `package.json` but `test_commands` does not include `npm test`
- **AND** warnings include `package.json exceeds 256KB (not read)`

#### Scenario: Malformed package.json
- **WHEN** the root contains a `package.json` with invalid JSON (e.g. `{`)
- **THEN** the scan succeeds and warnings include `malformed package.json`

### Requirement: Sensitive Files Are Listed But Never Read
Files whose name starts with `.env`, starts with `id_rsa`, or ends with `.pem` SHALL be treated as sensitive: their contents are never read, they contribute nothing to language/manifest/test detection, and their root-relative slash-separated paths are collected, sorted, and reported in a single warning of the form `sensitive files present (not read): <path1>, <path2>, ...`. No byte of a sensitive file's content may appear anywhere in the tool result.

#### Scenario: Secrets are named but not leaked
- **WHEN** the root contains `.env`, `id_rsa`, and `secret.pem`, each holding a secret value
- **THEN** the warning `sensitive files present (not read): ...` lists all three file names
- **AND** none of the secret values appear anywhere in the serialized summary or warnings

### Requirement: Scan Confinement (Symlinks and .git)
The scan SHALL be confined to the project root. Directory symlinks SHALL NOT be followed (a symlink pointing outside the root contributes nothing to detection). A regular-file entry that is a symlink SHALL be counted in `file_count` but never read or classified, so a manifest reachable only via a symlink (e.g. a `package.json` symlinked from outside the root) is neither listed nor parsed, and its content never appears in the result. A `.git` directory SHALL be skipped entirely (its contents never scanned) and the warning `git directory present (not scanned)` appended.

#### Scenario: Directory symlink escaping the root
- **WHEN** the root contains a symlink to an outside directory holding `go.mod` and `main.go`
- **THEN** the summary reports no languages, manifests, or test commands and no warnings

#### Scenario: Manifest symlink escaping the root
- **WHEN** the root contains `package.json` as a symlink to an outside file containing a secret
- **THEN** `manifests` does not include `package.json`, `test_commands` does not include `npm test`
- **AND** the outside file's content appears nowhere in the serialized result

#### Scenario: .git contents ignored
- **WHEN** the root contains a `.git` directory holding forged `go.mod` and `.go` files
- **THEN** the summary reports no languages, manifests, or test commands
- **AND** warnings include `git directory present (not scanned)`

### Requirement: Truncation and Partial Status
When the file count reaches the effective `max_files` limit, the scan SHALL stop, set `truncated: true`, cap `file_count` at the limit, and append the warning `file limit reached`. The tool result status SHALL be `partial` when the summary is truncated and `ok` otherwise; the summary line is `Project inspection completed.` in both cases.

#### Scenario: Truncated scan
- **WHEN** a root with 4 files is scanned with `max_files: 2`
- **THEN** `file_count` is 2, `truncated` is true, warnings include `file limit reached`
- **AND** the envelope status is `partial`

### Requirement: Cancellation
The scan SHALL check the context on every visited entry and abort with the context's error as soon as the context is cancelled or its deadline passes; the runtime envelope enforces the 10-second tool timeout.

#### Scenario: Cancelled mid-scan
- **WHEN** the context is cancelled while walking a large tree
- **THEN** the scan returns `context.Canceled` instead of a summary

### Requirement: Output Contract
Successful results SHALL be returned inside the shared capability envelope (`status`, `summary`, `data`, `artifacts`, `warnings`, `next_actions`, `trace_id` — specified separately) with `data` carrying the summary object: `project_id`, `name`, `languages`, `manifests`, `test_commands`, `file_count`, `truncated`. `artifacts` and `next_actions` SHALL be empty arrays; `warnings` SHALL carry the scan warnings in the order accumulated (with the sensitive-files warning appended last). Status SHALL be one of `ok`, `partial`, `blocked` (input over the 256 KiB cap), or `failed` (scan or decode error).

#### Scenario: Well-formed envelope for any input
- **WHEN** the tool executes with any input bytes
- **THEN** the result serializes to valid JSON with status in {`ok`, `partial`, `blocked`, `failed`}, a non-empty summary, and a `tr_`-prefixed trace ID
