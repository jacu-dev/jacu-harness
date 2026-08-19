# Threat model — JACU v1

> Status: accepted v1. Expanded against MCP spec 2026-07-28.
> Reviewed: 2026-07-30. Updated against the code: 2026-08-04.

## Two honest enforcement levels

1. **Workspace boundary — guaranteed.** Every future mutation happens in an
   isolated git worktree on its own branch. Apply only when the digest matches
   the reviewed diff. This holds whether or not the host cooperates: it is
   git/filesystem, not an agreement.
2. **Per-call gating — cooperative.** Read/execute tools pass the gates, but
   those gates only protect calls the host routes through JACU. The host can
   always use its native tools outside. Level 1 is what makes that safe;
   level 2 is a bonus when the host cooperates. **No doc or skill may promise
   more than this.**

## Threats and architectural controls

| Threat | Control |
|---|---|
| Prompt injection in repository files | Read content is untrusted data; it never changes policy, widens a path or releases a command |
| Path traversal / symlink escape | Project root fixed at start; Clean/Rel + EvalSymlinks; allowlist; reject outside the root; fail closed |
| Command injection | There will never be `execute_shell(string)`; known executable + argv, `exec.CommandContext`, no `sh -c` |
| Secret exfiltration | Env allowlist; redaction before log/output; sensitive file names listable, contents never read |
| Output flood / host OOM | Byte caps in/out per tool (16KB default; 32KB on `jacu_diff` and `jacu_memory_recall`); overflow becomes `status: partial`. There is no artifact store yet. Hosts also truncate; the cap is ours, not theirs |
| Resource exhaustion | Per-tool timeout; cancellation propagated (clean SIGINT, no orphan goroutine); one active tool per process in the MVP |
| Annotations as fake security | Spec: clients MUST treat annotations as untrusted → annotations are metadata; enforcement is the runtime, covered by test |
| Binary/skills supply chain | Release with sha256; skills installed with preview + verify; no silent auto-update |

## Non-negotiable rules

- Deny-by-default on path, command, env and network.
- Read-only before write; write before destructive. **No tool declares
  `risk: destructive` in the runtime** — `jacu_apply` and `jacu_discard` are
  `risk: write` with `destructiveHint: true`, and a mission with
  `risk_hint: destructive` requires `approve_destructive: true` on apply.
  Runtime `RiskDestructive` is defined and unused; promoting a tool to that
  tier needs a recorded decision.
- `serve` stdout is protocol-only; logs go to stderr; no log carries a secret
  or a full file body.
- Roots/Sampling/Logging MCP are unused (deprecated in 2026-07-28); new spec
  resources only via capability negotiation.

## Out of scope for v1 (record, do not solve)

Kernel sandbox (seatbelt/gVisor) — enters if/when arbitrary user command
execution exists. Multi-tenant. Network.
