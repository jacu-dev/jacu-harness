# ADR-023 — Clarity gate

- Status: proposed; owner ratification required
- Date: 2026-08-21
- Scope: SDD-005 spec readback, variance, rewrite cap

## Decision

JACU compiles a closed readback schema and compares host-supplied answers
against the spec's declared fields. It never calls a model to author the
probe or to fill the answer. The host runs the cheap reader.

The accepted JSON object has exactly these keys, each a string array:

- `write_scope`
- `forbidden_paths`
- `requirements`
- `out_of_scope`
- `tasks`

Prose, extra keys, and malformed JSON are typed refusals. Field comparison
reuses `sdd.Parse`, `sdd.WriteScope`, and `sdd.Tasks`. A path outside the
declared write scope is a `write_scope` finding naming that path.

Three runs are required for a verdict. If the normalized readbacks disagree
on any field, the verdict is `fail` with `variance_runs=3`, even when each
run would individually match the spec. A rewrite round whose spec bytes
exceed the previous round is refused (`spec_bytes_delta` positive).

`clarity.probe` is a telemetry v2 event. It carries `round`, `divergences`,
`divergence_field`, `variance_runs`, `spec_bytes`, `spec_bytes_delta`, and
`verdict`. Model-controlled strings and file paths are unrepresentable.

The CLI is `jacu clarity probe|ingest|verdict`. Exit 0 is pass, 1 is a
gate failure, 2 is usage. `--json` occupies stdout; diagnostics stay on
stderr. No MCP tool is added.

## Consequences

- Ambiguous specs fail before dispatch instead of mid-worktree.
- Owner ratification is required before this decision is considered final.
- Probe quality depends on the host's chosen cheap tier; JACU records
  detection, not model identity.

## Alternatives rejected

- Letting JACU call the probe model: crosses the "never call a model"
  program rule and puts credentials in the runtime.
- A global comprehension score: hides which field was misread.
- Allowing the spec to grow to answer a question: stacks prose instead of
  clarifying (ADR-020).
