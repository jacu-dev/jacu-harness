---
name: jacu-model
description: Use host-profile routing, signed CLI attestation, resilience, and safe cost evidence.
---

# JACU model control

Use this capability when a flow must choose among named host-native CLI
profiles or decide whether a retry is safe. It is a local control plane, not a
provider API or credential manager.

Before invoking a profile:

1. Classify complexity from the task facts; do not ask a model to choose the
   tier.
2. Require the absolute CLI path, SHA-256 digest, signer and external
   attestation verifier.
3. Route by lane, output contract, context and capability. Demotion is based
   on at least 10 samples and failure rate strictly above 40%.
4. Pass the selected profile through the circuit breaker. An open breaker is a
   blocked result, not permission to fall back silently.
5. Record ids, token counts, duration and integer units only. Never put prompt,
   output, API key, USD subscription cost or raw stderr in a trace.

For a failed attempt, use the bounded escalation policy. Budget exhaustion is
terminal; exhausted retries become human-required or fail-closed according to
policy. A single mission ledger owns the global ceiling.
