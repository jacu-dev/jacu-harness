# Triggers — what does not open without a signal

Each item here has a ready plan and **does not open** until the recorded trigger fires.
A fired trigger becomes an SDD.

## Memory FTS5 backend
**Trigger (any of):** corpus > 500 records **or** retrieval eval below the floor recall@3 = 0.70.
**State:** not fired — the store had 5 records / ~20 KiB on 2026-08-15.
**Plan when it fires, in 5 steps:** freeze the real corpus as a fixture; A/B benchmark naive vs `modernc.org/sqlite`+FTS5 on the same corpus and queries (recall@3, recall@10, p95, RSS) on real hardware; decide by number; idempotent JSON→SQLite migration with count and digest verification per record, keeping JSON as the canonical export; full-suite regression without changing any contract test.
**Hidden cost:** first new CGo-free dependency. Requires an ADR and a threat-model update — an SQLite file in place of readable JSON.

## Output artifact store
**Trigger:** some phase needs to **read complete output after the fact** — not "would be nice to have".
**State:** not fired. Autonomy consumes `evidence_digest` and the output tail from verify.
**Standing decision in its place:** pure cap with digest; the handler self-truncates by priority. See `floors-and-limits.md`.
**Why not now:** an artifact store is a subsystem, not a field — storage, retention, GC, path confinement and integrity. It would repeat the hardening `archiveDiff` already paid for.

## Denial classification and network escalation
**Trigger:** a real sandbox exists (seatbelt/gVisor). Without it there is no denial to classify.
**When it fires:** port the fixed order of the four rules in `03-gates.md` §9 — **network before filesystem**, generic `EPERM` for filesystem.
**Recorded trap:** reversing that order turns a write denial into a network-approval prompt. The inherited argv predictor was refused: false negative on `curl` and `git fetch`, false positive on `cargo update`.

## `jacu sdd archive` as a subcommand
**Trigger:** an eval showing that the manual `git mv` costs more than the CLI surface it would add.
**State:** deferred twice.

## `$defs`/`$ref` in the MCP catalogue
**Trigger: ALREADY FIRED.** 20,476 of 20,480 bytes — 4 bytes of headroom, ratchet in `test/e2e/mcp_test.go`.
**Standing rule:** the next capability requires compaction or enters as a CLI subcommand.

## `model-panel`
**Trigger: ALREADY FIRED.** It depended on SDD-003 closing; 003 has been merged since 2026-08-14.
**Becomes SDD 013.** Design in `docs/sdd/013-model-panel/design.md`.

## Storage and retention limits
**Current numbers are chosen, not measured:** 8 MiB per segment, 128 MiB total, 12 months, 30 days, 1,000 records.
**Trigger:** measured production volume. Changing them requires a new SDD/ADR.

## Fusion of short CI jobs
**Trigger: ALREADY FIRED** on 2026-08-15, when August closed at 3,806 billed minutes in this repository alone.
`hygiene` + `mod-hygiene` + `secrets` were fused into the `checks` job — the three summed 48 s of work and were billed as three minutes because Actions rounds each job to a full minute. `mcp-smoke` stayed out: it is conditional on code, and the other three cannot be.
Required status checks are now: `changes, verify, checks, e2e, lint, vuln, mcp-smoke`. The contract of what runs and what is skipped lives in `floors-and-limits.md`.
