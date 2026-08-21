# ADR-030 — Model panel binding

- Status: proposed; owner ratification required
- Date: 2026-08-21
- Scope: SDD-013 profile-to-CLI binding and attested spawn

## Decision

Flow nodes declare a `lane` (`cheap`, `medium`, `planner`, `premium`). They
do not name a CLI binary. The engine calls `Classify`, then `Route`, then
`ApplyDemotionBias`, then `GuardedInvoke`. Profile-to-CLI mapping is
configuration injected as `HostProfile` values, not compiled into the node.

Attestation is fail-closed on every spawn: absolute path, SHA-256 digest,
signer, signature, and an injected verifier. A digest mismatch or missing
field blocks before `exec`. Self-updating CLIs that change their own bytes
break the attested digest; the panel does not cache first-use hashes. The
operator must re-attest after an update. That is break-on-update.

`jacu verify` does not grow a per-CLI allowlist for panel spawns. The
attested absolute path is the spawn gate; widening verify's command
allowlist for host CLIs is refused.

Gemini is not a route target until `test/hosteval` has a passing host
case. `CostTrace` is a v2 `cost.trace` event with ids, tokens as
non-dollar counts, duration, and subscription billing that cannot invent
USD.

Demotion never returns an empty candidate list: if every profile would be
dropped, the original set is kept.

## Consequences

- A node can change CLI without a recompile.
- Owner ratification is required before this decision is final.

## Alternatives rejected

- Caching the first-seen digest: would silently accept a replaced binary.
- Naming CLIs on the flow node: couples graphs to a vendor argv.
- Routing Gemini now: no host-harness case exists.
