# MCP tool reference

The surface is frozen (ADR-008): thirteen tools, and new capability enters as
a CLI subcommand, never as a new tool. Descriptions on the wire are terse by
design; the contract lives in the specs under
[../sdd/specs/](../sdd/specs/) and the shipped skills under `skills/`.

All tools observe the byte caps from the threat model: 16 KB in/out by
default, 32 KB on `jacu_diff` and `jacu_memory_recall`; overflow returns
`status: partial`, never a truncated lie.

## The governed run

| Tool | Contract |
|---|---|
| `jacu_mission_compile` | Objective, scope and verification argv → a governed mission. Verification commands must clear the allowlist. Missions carry a ceremony level and a risk hint. |
| `jacu_workspace_open` | Create the isolated run: worktree on its own branch, `run_id`, persisted state. Every change happens only inside the returned `worktree_path`. |
| `jacu_verify` | Run the mission's verification commands inside the worktree. Only a `pass` verdict may proceed to review. Also runs one allowlisted diagnostic command on request. |
| `jacu_diff` | The scoped patch and its digest for human review. Any later edit invalidates the review and requires a new diff. |
| `jacu_apply` | After explicit approval: validate the digest against the reviewed diff and commit the tree. May re-run verification for up to 10 minutes — hosts must allow that (`MCP_TOOL_TIMEOUT` in Claude Code). A mission with `risk_hint: destructive` requires `approve_destructive: true`. |
| `jacu_discard` | Abandon the run and release the worktree. The counterpart of `jacu_apply`; never both. |

Never apply an unreviewed diff.

## Observation

| Tool | Contract |
|---|---|
| `jacu_status` | Runs still holding worktrees, across projects. |
| `jacu_workspace_status` | State of the active run in this project. |
| `jacu_project_inspect` | Read-only project analysis; changes nothing. |
| `jacu_report` | Deterministic Markdown projection of workspace/audit state. |

## Memory

| Tool | Contract |
|---|---|
| `jacu_memory_save` | Propose durable knowledge; entries are linted and promoted, not blindly stored. |
| `jacu_memory_recall` | Query the store; recall is capped (32 KB) and integrity-checked. |

## Orchestration

| Tool | Contract |
|---|---|
| `jacu_flow_run` | Execute a bounded multi-step flow with declarative edges, independent path waves and fan-in. Bounded: no unbounded loops, no self-extension. |

## What there will never be

`execute_shell(string)`. Command execution is always a known executable plus
argv, checked against `.jacu/verify-allowlist.json`, run via
`exec.CommandContext` — no `sh -c`, no network
([../threat-model.md](../threat-model.md)).
