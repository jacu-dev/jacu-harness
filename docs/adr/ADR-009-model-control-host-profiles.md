# ADR-009: Model control by host profiles and attested CLIs

- Status: accepted
- Date: 2026-08-11
- Decider: Erick

## Context

The inherited SDD mixed routers, API prices, bandit and execution profiles.
JACU must not hold provider credentials or turn a host CLI into its own API.
The runner already uses allowlisted CLIs; this decision covers profile
selection, cost recording and interrupting a degrading CLI.

## Decision

1. The routable object is a named `HostProfile` (`cheap`, `medium`, `planner`,
   `premium`) with capability, contract, max context and CLI identity.
   `complexity::classify` chooses the lane by heuristic without an LLM. No
   remote model, API price table or HTTP endpoint enters the runtime.
2. A CLI is eligible only when the caller presents a SHA-256 digest, an
   absolute path and a signature attestation verified by an injected external
   function. JACU does not invent a signature scheme and does not read, store
   or transmit credentials.
3. Demotion is derived from failure metrics (at least 10 samples and a rate
   strictly greater than 40%); it never leaves a task without a candidate.
   Provider failures follow a bounded escalation policy; budget is terminal.
4. Every controlled invocation goes through a per-profile circuit breaker:
   three consecutive failures open the circuit, cooldown allows one half-open
   probe, success closes it. The callback is the only execution port.
5. `CostTrace` records only ids, tokens, duration, billing mode and integer
   units. `subscription` cannot declare USD or API cost; missing measurement
   is explicit, never silent zero cost. The ledger uses saturating arithmetic
   and a global ceiling.
6. Control is an internal library consumed by later invocations and flows.
   There is no `jacu_model_route` and no new MCP surface. Bandit, gateway,
   embeddings, OAuth and API routing stay out of SDD-009.

## Consequences

- Choice stays local, deterministic, auditable and compatible with host-
  authenticated `claude -p` / `codex exec`.
- The caller must supply a trusted attestation before execute; identity error
  or an open circuit blocks without spawn.
- Signature metrics do not fake a dollar price. A later meter may fill integer
  units without changing the security contract.
