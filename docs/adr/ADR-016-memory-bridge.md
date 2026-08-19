# ADR-016: Full memory and sentinel bridge for AGENTS.md

- Status: accepted
- Date: 2026-08-11
- Decider: Erick

## Context

Local JSON memory already has secret lint, supersede, deterministic recall and
a quality eval. Hosts that do not load MCP still need active conventions to
reach `AGENTS.md` without replacing human-written text. SQLite/FTS5 stays
conditioned on volume or recall loss, not implementation preference.

## Decision

1. The bridge is a region delimited by sentinels in `AGENTS.md`. Content
   outside the region is untouchable. Missing region allows appending a new
   region, including in a pre-existing human file.
2. The region contains a `source-hash` of the canonical projection of active
   conventions and a SHA-256 checksum of the managed body. The first detects
   source drift; the second proves the region was not edited by hand. A
   missing or diverging checksum blocks the update; the region is never
   overwritten under doubt.
3. The projection is byte-deterministic, ordered by `memory_id`, without
   secrets, with each record reduced to one safe line. Writes are atomic and
   preserve human content outside the sentinels.
4. Promotion `derived → convention` requires the caller to supply
   `evalPassed = true`. The function receives `now`, does not inspect eval
   content, and creates a successor that supersedes the previous record.
5. JSON remains the backend. FTS5 is a migration decision only when the
   corpus exceeds 500 records or an explicit recall@3 measurement falls
   strictly below 0.70. Embeddings, HTTP and a model-router stay out.

## Consequences

- The bridge can coexist with a third-party `AGENTS.md` and need not own the
  whole file.
- A human edit inside the region is blocked and auditable by the error;
  recovery requires regenerating the region on purpose.
- Promotion keeps history by supersede and does not turn an eval result into
  implicit authority.
