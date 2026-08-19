# memory Specification

## Purpose
Define the local memory capability shipped in `internal/capability/memory/`: the `jacu_memory_save` and `jacu_memory_recall` tools, the zero-dependency JSON file store they share, the lint gate that guards writes, the naive token-overlap search that answers recalls, and the eval corpus that pins retrieval quality. The store is plain JSON files on disk; there is no SQLite, FTS5, or any external dependency.

## Requirements

### Requirement: Record model and deterministic identity
The system SHALL represent a memory as a record with fields `memory_id`, `project_id`, `kind`, `title`, `body`, `evidence`, `source`, `status`, `superseded_by`, `created_at`, and `updated_at`. `kind` SHALL be one of `decision`, `convention`, `gotcha`, or `preference`; `source` SHALL be one of `human` or `derived`; `status` SHALL be one of `active` or `superseded`. The `memory_id` SHALL be derived deterministically as `mem_` plus the first 8 bytes (16 lowercase hex characters) of the SHA-256 of `project_id`, `kind`, and the lowercased whitespace-collapsed title, joined with NUL separators, so it always matches `^mem_[a-f0-9]{16}$`. Input fields SHALL be normalized before identity and lint: whitespace trimmed, and evidence entries trimmed, deduplicated, and sorted.

#### Scenario: Same identity yields same id
- **WHEN** two save inputs share the same `project_id`, `kind`, and title (differing only in case or internal whitespace of the title)
- **THEN** both produce the same `memory_id`
- **AND** saving the second overwrites the first record in place instead of creating a duplicate

#### Scenario: Identity fields change the id
- **WHEN** any of `project_id`, `kind`, or the normalized title differs between two inputs
- **THEN** the derived `memory_id` differs

#### Scenario: Evidence normalization
- **WHEN** a save input carries evidence entries with surrounding whitespace and duplicates
- **THEN** the stored record's evidence is trimmed, deduplicated, and sorted lexicographically

### Requirement: Lint gate blocks invalid saves
`jacu_memory_save` SHALL lint the normalized input before any write and SHALL return status `blocked` with the lint list in `data.lints` when any lint of level `BLOCK` fires, writing nothing to disk. The lint rules and their exact limits SHALL be: `derived_without_evidence` (source `derived` requires at least one non-empty evidence entry), `invalid_source` (source must be `human` or `derived`), `secret_content` (title or body matches a secret pattern: private-key PEM headers; `ghp_`/`gho_`/`github_pat_`/`sk-`/`AKIA`/`xox[bpsa]-` token prefixes; `Bearer` tokens; URLs with embedded `user:password@` credentials; `password|passwd|secret|token|api_key` assignments), `invalid_kind` (kind must be `decision`, `convention`, `gotcha`, or `preference`), `global_scope_restricted` (empty `project_id` is allowed only for kind `preference`), `project_root_unresolved` and `project_id_mismatch` (a non-empty `project_id` must equal the id derived from the server's project root), `empty_title`, `title_newline` (title must not contain `\r`, `\n`, U+0085, U+2028, or U+2029), `title_too_long` (more than 120 runes), `body_too_large` (more than 4096 bytes), and `invalid_supersedes` (`supersedes` must match `^mem_[a-f0-9]{16}$`).

#### Scenario: Secret content is blocked
- **WHEN** a save input's body contains a GitHub token such as `ghp_xxxx`
- **THEN** the envelope status is `blocked` with a `secret_content` lint of level `BLOCK`
- **AND** a subsequent recall for the project returns zero results, proving nothing leaked to disk

#### Scenario: Derived memory without evidence
- **WHEN** a save input has source `derived` and no non-empty evidence entry
- **THEN** the save is blocked with rule `derived_without_evidence` on field `evidence`

#### Scenario: Global scope restricted to preferences
- **WHEN** a save input has an empty `project_id` and a kind other than `preference`
- **THEN** the save is blocked with rule `global_scope_restricted`

#### Scenario: Exact size limits
- **WHEN** the normalized title exceeds 120 runes or the normalized body exceeds 4096 bytes
- **THEN** the save is blocked with `title_too_long` or `body_too_large` respectively

### Requirement: Save semantics and supersede chain
When no `BLOCK` lint fires, `jacu_memory_save` SHALL persist an `active` record with RFC 3339 UTC timestamps, preserving `created_at` from any existing record with the same `memory_id` and refreshing `updated_at`, then return status `ok` with `data.memory_id`, the full stored record, and the (possibly empty) lint list. When `supersedes` is set, the store SHALL require the target to exist, to belong to the same `project_id`, and to not already be superseded by a different record; it SHALL write the successor first and then mark the target `status: "superseded"` with `superseded_by` pointing at the successor. The store SHALL reject a record whose `memory_id` already exists in a different scope, a `supersedes` value equal to the record's own id, and any record whose stored fields fail validation (`project_id` empty or matching `^prj_[a-f0-9]{16}$`, valid kind, source, and status).

#### Scenario: Successful supersede
- **WHEN** a record is saved with `supersedes` set to an existing active record of the same project
- **THEN** the new record is stored as `active`
- **AND** the target record's status becomes `superseded` with `superseded_by` set to the new `memory_id`

#### Scenario: Supersede target invalid
- **WHEN** `supersedes` references a missing record, a record from another project, or a record already superseded by a different successor
- **THEN** the save fails and no record is written

#### Scenario: Re-save preserves creation time
- **WHEN** an input with an identity already on disk is saved again
- **THEN** the stored record keeps the original `created_at` and gets a new `updated_at`

### Requirement: Durable JSON storage layout under the memory home
The store SHALL persist records as pretty-printed JSON files at `<home>/memory/<scope>/<memory_id>.json`, where `<home>` is `$JACU_HOME` when set and `~/.jacu-harness` otherwise, and `<scope>` is the `project_id` for project records or `_global` for preference records with no project. Directories SHALL be created with mode `0700` and record files with mode `0600`. Writes SHALL be atomic and durable: content is written to a randomly named temp file in the scope directory, fsynced, renamed over the destination, and the directory is fsynced. Reads SHALL be defensive: symlinks and non-regular files are skipped, scope names must be `_global` or match `^prj_[a-f0-9]{16}$`, path identity is re-checked around opens, and a record whose JSON contents disagree with its filename, scope, or field validation is ignored.

#### Scenario: Scope directory selection
- **WHEN** a record with `project_id` `prj_<16 hex>` and a global preference record are saved
- **THEN** the first lands in `memory/prj_<16 hex>/` and the second in `memory/_global/`

#### Scenario: Corrupt or foreign file ignored
- **WHEN** a scope directory contains a file whose JSON fails to parse, whose `memory_id` or `project_id` does not match its path, or that is a symlink
- **THEN** reads and searches skip it without error

### Requirement: Cross-process write serialization via flock
The store SHALL serialize saves at three levels: a per-store-instance mutex, a per-canonical-root in-process mutex shared by all store instances, and a cross-process advisory lock taken with blocking exclusive `flock` on a `.memory.lock` file (mode `0600`) inside the store root, retried through `EINTR` and released with `LOCK_UN` after the save. On platforms other than Linux and darwin, acquiring the file lock SHALL fail with an unsupported-platform error, refusing unsynchronized writes.

#### Scenario: Two processes save to the same root
- **WHEN** two processes save records under the same store root concurrently
- **THEN** the second process blocks on the `.memory.lock` flock until the first releases it
- **AND** both records are durably stored without corruption

#### Scenario: Concurrent in-process saves
- **WHEN** multiple goroutines save distinct records through the same root concurrently
- **THEN** every save succeeds and every record is subsequently readable

### Requirement: Recall search, ranking, and filters
`jacu_memory_recall` SHALL search a single scope (the given `project_id`, or `_global` when it is empty) and return status `ok` with `data.results` as an array of `{record, score}` entries. The query SHALL be tokenized case-insensitively on letter/digit boundaries into a token set; each record scores +3 per query token found in its title tokens, +2 in its kind, +1 in its body tokens, and +1 in its evidence tokens, and records scoring below 3 SHALL be excluded. When the query is empty the call is a listing: all visible records match with score 0. Results SHALL be ordered by descending score, ties broken by ascending `memory_id`, and truncated to `k`, which defaults to 20 when omitted or non-positive at the tool layer (the store itself returns nothing for `k <= 0`). Superseded records SHALL be excluded unless `include_superseded` is true, and a non-empty `kinds` filter SHALL restrict results to those kinds. An invalid `project_id` SHALL yield an empty result set.

#### Scenario: Ranking weights
- **WHEN** a query token appears in one record's title and only in another record's body
- **THEN** the title match scores higher and sorts first

#### Scenario: Minimum score filters weak matches
- **WHEN** a record accumulates a score of less than 3 for a non-empty query
- **THEN** it is excluded from the results

#### Scenario: Listing mode
- **WHEN** recall is called with an empty query
- **THEN** all active records of the scope are returned with score 0, capped at `k`

#### Scenario: Superseded visibility
- **WHEN** a record has been superseded and recall is called with `include_superseded: true` and sufficient `k`
- **THEN** both the successor and the superseded record are returned
- **AND** without the flag only `active` records are returned

### Requirement: Memory tool registration and runtime limits
Both tools SHALL execute through the shared capability runtime with these specs: `jacu_memory_save` is risk `write`, not read-only, idempotent, closed-world, timeout 10s, max input 65536 bytes, max output 16384 bytes; `jacu_memory_recall` is risk `safe`, read-only, idempotent, closed-world, timeout 10s, max input 16384 bytes, max output 32768 bytes. Their MCP annotations SHALL mirror this (save: `readOnlyHint` false, explicit `destructiveHint` false, `idempotentHint` true, explicit `openWorldHint` false; recall: `readOnlyHint` true), and both SHALL publish concrete output schemas whose `data` property is a typed object.

#### Scenario: Listed metadata
- **WHEN** an MCP client lists tools
- **THEN** `jacu_memory_save` and `jacu_memory_recall` appear with the annotations above and non-empty concrete `data` schemas

### Requirement: Recall self-fits within its 32KB output cap
Because the runtime's generic overflow degradation zeroes `data`, the recall handler SHALL pre-fit its own envelope before returning: it measures the JSON-encoded runtime result using a fixed-width trace-id placeholder (`tr_0000000000000000`), and when the encoding exceeds 32768 bytes it SHALL append the warning `recall results truncated to fit 32KB encoded output limit` and keep only the longest highest-score prefix of the results (found by binary search) that fits, preserving status `ok`. If the envelope exceeds the cap even with zero results, the handler SHALL fail with a metadata-exceeds-limit error rather than return a misleading envelope.

#### Scenario: Oversized result set is trimmed, not zeroed
- **WHEN** the full result set encodes to more than 32768 bytes
- **THEN** the returned envelope has status `ok`, carries the truncation warning, retains a non-empty highest-score prefix of the results, and encodes to at most 32768 bytes

#### Scenario: Fitting envelope passes through
- **WHEN** the encoded envelope is at or under 32768 bytes
- **THEN** the results are returned unmodified with no truncation warning

### Requirement: Retrieval eval corpus guards recall quality
The repository SHALL ship a retrieval eval fixture in `internal/capability/memory/evaldata/` (`corpus.json`, `queries.json`) exercised by a regression test. The corpus SHALL contain 40 to 60 records with at least 10 each of `decision`, `convention`, and `gotcha` and at least 5 `preference`; the query set SHALL contain exactly 20 queries with `k: 3`, each expecting 0 to 3 known memory ids from a single scope, including at least 3 negative queries and covering all four kinds across expected records. Fixture decoding SHALL reject unknown fields, trailing JSON, and queries missing the `expected` array. The eval SHALL require aggregate recall@3 of at least 0.70 and zero results for every negative query.

#### Scenario: Regression gate
- **WHEN** the retrieval eval runs against the shipped fixtures
- **THEN** aggregate recall@3 is at least 0.70 and no negative query returns any result

#### Scenario: Fixture integrity
- **WHEN** a fixture record or query carries an unknown field or trailing JSON
- **THEN** decoding fails and the eval does not run on corrupt data

### Requirement: Active conventions bridge to a managed AGENTS region

When a project convention is saved, the system SHALL refresh only the
sentinelled JACU memory region in that project's `AGENTS.md`. The projection
SHALL be deterministic, contain a source hash and checksum trailer, preserve
bytes outside the region, block checksum drift, and write atomically. A human
file without a region SHALL be adopted by appending the managed region.

#### Scenario: Convention save reaches non-MCP hosts

- **WHEN** an active convention is saved for a project
- **THEN** the project's `AGENTS.md` contains the sorted managed projection

#### Scenario: Managed drift is fail-closed

- **WHEN** the managed region checksum is missing or divergent
- **THEN** the save returns an explicit warning and does not overwrite the file

### Requirement: Derived promotion is an explicit audit decision

The system SHALL promote an active `derived` record to a new `convention`
successor only when the caller supplies a passing eval outcome and explicit
timestamp. The promotion SHALL use the existing supersede path and SHALL not
inspect eval content or default the clock.

#### Scenario: Eval outcome gates promotion

- **WHEN** the caller supplies `evalPassed=false`
- **THEN** no successor is written

### Requirement: JSON backend remains until a measured trigger

The system SHALL select JSON while corpus size is at most 500 and measured
recall@3 is at least 0.70. It SHALL select the future FTS5 path only when the
corpus exceeds 500 or measured recall@3 is below 0.70; unknown recall SHALL not
trigger migration.

#### Scenario: Threshold boundary stays dependency-free

- **WHEN** corpus size is 500 and measured recall@3 is 0.70
- **THEN** JSON remains selected
