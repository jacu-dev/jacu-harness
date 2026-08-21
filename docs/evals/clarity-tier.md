# Clarity tier calibration

JACU does not call a model. Tier choice is a host decision. This eval
records what the gate itself detects on a seeded corpus so a host can
compare cheap/medium/planner/premium later against the same fixtures.

Seed: SDD-006 write scope `internal/capability/context/**` plus a planted
path `internal/mcpadapter/server.go`.

| Reader | n | Detected extra write_scope | Notes |
|---|---|---|---|
| fixture / exact ingest | 3 | 3/3 | `jacu clarity ingest` on the seeded JSON |
| host cheap | 0 | not run | JACU never invokes the probe |
| host medium | 0 | not run | same |
| host planner | 0 | not run | same |
| host premium | 0 | not run | same |

Cheapest tier that still detects the seed is therefore **not measured in
process**. The mechanical gate detects the seed at n=3. A host that wants
a model-tier table runs `jacu clarity probe` and fills the rows above.
