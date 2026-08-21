# ADR-024 — Context admission

- Status: proposed; owner ratification required
- Date: 2026-08-21
- Scope: SDD-006 pack, ledger, anchors

## Decision

Admission selects repository bytes for a compiled mission. It never
summarises, compresses, or rewrites content. The unit is bytes, measured
exactly. Token estimates stay out of the ledger.

A pack is the sorted list of items derived from the mission contract:
synthetic required anchors (objective, acceptance) plus files under
`allowed_paths` that are not `forbidden_paths`. `context.refs` names
required files; when it is empty, every packed file is required. Item
order is required-first, then path. Filesystem walk order is discarded.

The pack digest is SHA-256 of the canonical item list, including each
item's content digest, so a one-byte content change changes the pack
digest. Packs are not persisted.

The ledger decides `admit`, `refuse`, or `degrade` before any tool call.
A required item that does not fit is `refuse` with `required_overflow`.
Only optional overflow is `degrade`. Reasons are a closed enum:
`budget_fit`, `required_overflow`, `optional_dropped`, `anchors_lost`.
Events carry `budget_bytes`, `requested_bytes`, `remaining_bytes`. They
never carry a counterfactual cost.

Anchors are extracted from the mission and checked against the pack. A
missing anchor fails `context.anchor` and blocks dispatch.
`coverage_bps` is `items_included * 10000 / items_required`.

CLI: `jacu context pack|explain`. No new MCP tool.

## Consequences

- Tasks that cannot fit required context fail before the first tool call.
- Owner ratification is required before this decision is final.

## Alternatives rejected

- A compressor or summariser: optimises the wrong variable.
- Token budgets: provider-specific estimates, not exact bytes.
- Persisting packs: would create a second source of truth.
