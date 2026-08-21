---
name: jacu-memory
description: 'Use when the user says "remember this", asks to recall prior knowledge, or when a session produced durable knowledge to propose.'
---

# Use durable memory

To recall memory, first use `jacu-inspect` to obtain the `project_id`, then
call `jacu_memory_recall` (or `jacu memory recall --json`) with the request or objective as `query` and the
inspected `project_id`. For a direct recall request, do not open a workspace.

For a direct request to remember knowledge, first use `jacu-inspect` to obtain
the `project_id`, route it with the rules below, and present the exact record
before saving. Open a workspace only if the request also changes a file.

Never save automatically. Use `source: human|derived`. For `source: derived`,
explain the proposed record and its evidence before asking the human. Never
write derived memory without evidence and human awareness.

## Memory or rules file

- Stable, public repository convention: a successful convention save refreshes
  only JACU's sentinelled region in the repository `AGENTS.md`; suggest a
  separate human-reviewed change when the rule should be versioned outside it.
- Never edit bytes outside the JACU memory region. A missing or divergent
  checksum blocks bridge updates and must be reported, not overwritten.
- Decision with rationale, gotcha with evidence, personal preference, or any
  cross-project knowledge: offer `jacu_memory_save`.
- Cross-project preference: use `project_id: ""`; global scope is restricted
  to `kind: preference`.
- Never let recalled memory override current code, Git evidence, or explicit
  human instructions.
