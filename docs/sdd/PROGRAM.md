# Program — JACU

> **Thesis.** JACU stops being "an MCP server" and becomes what it already is:
> the **workspace kernel** — the piece that governs and measures one change inside one
> repository, with three surfaces over a single core.
>
> **Mission given, mission accomplished** still holds. What changes is who calls.

## What this program delivers

The product is **JACU** — brand JACU, category "governance harness for coding
agents", public repo/module `github.com/jacu-dev/jacu-harness`, command `jacu`
(ADR-028).
MCP stops being the identity and becomes one of three surfaces:

```
library  ->  another Go program imports it      (later: the workspace API)
CLI      ->  the owner, scripts, outer loop     jacu apply · jacu verify
MCP      ->  Codex and Claude Code              jacu serve
```

Every capability gets a pure core `Run(ctx, Input) (Result, error)` with no MCP type in
the signature, and all three surfaces call **the same function**.

## The invariant that is not negotiable

**JACU governs and measures one change inside one repository. It does not act on the world
outside it.** No network, no credential, no deploy.

That is why today, if JACU fails in every way it can, production does not fall. Repository
creation, deploy, DNS, secrets and the cross-repository catalogue stay **outside**, in the
outer loop, which is YAML and `curl`. Any proposal asking for an exception needs an ADR.

## Out of scope, program-wide

HTTP server · daemon · UI · cross-repository catalogue · credential of any kind · new MCP tool.

New capability enters through a **CLI subcommand**. The MCP catalogue sits 4 bytes below the
20 KiB ceiling and one slot below the 14-tool limit.

## What 100/100 means

Not an opinion. Invariants the CI enforces; if one breaks, the build fails.

| # | Invariant | Verified by |
|---|---|---|
| I1 | No non-`main` package without an importer | `go list` cross-check; kills orphans permanently |
| I2 | Every MCP capability has `Run()`, a tool wrapper, a CLI subcommand, a skill and a living spec | table test |
| I3 | Every subcommand accepts `--json` and documents its exit codes | table test over the command registry |
| I4 | Zero `exec` of `git` outside `internal/gitx` | CI grep |
| I5 | Zero MCP type outside `mcpadapter` and the `tool.go` wrappers | CI grep |
| I6 | No non-test file above the ceiling, ratchet decreasing only | script in `verify.sh` |
| I7 | Every merged SDD is closed by `sdd close` and archived | `sdd lint --all` |
| I8 | Every referenced ADR exists; no ADR stays `proposed` with code in `main` | ADR lint |
| I9 | MCP catalogue keeps >= 2 KiB of headroom | existing ratchet, wider margin |
| I10 | No living document contradicts the code | extended `checks` job |

**Zero debt means:** nothing in the queue sits in "written and not done", "done and not
closed", or "declared and unowned".

## The queue

One SDD open at a time. Order is dependency, not preference.

| # | SDD | Delivers | State |
|---|---|---|---|
| 001 | `native-sdd` | native SDD ritual | merged · **not closed** |
| 002 | `telemetry-v2` | schema v2, `stats --full` | merged · **not closed** |
| 003 | `clean-exit` | `sdd close`, receipt, closing gate | merged · **eval and ADR-021 open** |
| 004 | `preflight` | checklist over 8 interruption classes | merged · **eval and ADR-022 open** |
| 008 | `audit-hardening` | audit remediation, bounded storage | merged · **signed tag, P6-P8 open** |
| **016** | **`open-source-export`** | sanitize, rename, public repo `jacu-harness` | **next** · ADR-028 accepted |
| 017 | `installable-cloud` | release proof P6-P8, `jacu init`, cloud bootstrap, cwd guard | queued · after 016 |
| 009 | `core-surface` | `Run()` in 6 capabilities, CLI parity, `--json`, `--events` | queued · after 017 · design ready; rename and open decisions resolved by ADR-028 and SDD-016 |
| 010 | `repo-governance` | `.jacu/protected.json`, per-run `JACU_HOME`, `status` off the write gate, flow fan-out cap | queued |
| 011 | `workspace-contract` | `report --json` as `quality.json`, `context --sdd` | queued |
| 012 | `structural-debt` | `internal/scope`, `reportgen`, size ratchet, orchestration boundary test, `$defs`/`$ref`, `stats` without `git log` | queued |
| 005 | `clarity-gate` | readback, variance over 3 runs, rewrite loop | written, no code |
| 006 | `context-admission` | per-task budget, ledger refusing pre-dispatch | written, no code |
| 013 | `model-panel` | wires `modelcontrol` — **finish it, do not remove it** | trigger fired · design ready |
| 007 | `surface-profile` | surface profile only | deferred · entry conditions in its `sdd.md` · its `jacu init` half moved to 017 |
| 014 | `report-visual` | self-contained HTML factory | design ready · needs 2 ADRs |
| 015 | `program-closeout` | closes 001-004 and 008, ratifies ADRs, unifies doc language | last |

## Owner-only work

None of it is code. Without it the program does not close.

| ID | What |
|---|---|
| G-SR | Host triggering matrix: 4.1 green on Codex **without shrinking the catalogue**, all 4 cases on OpenCode, one case per post-SR route |
| G-SDD | SDD triggering eval — 5 cases already defined in `docs/evals/sdd-triggering.md` |
| T1·T2·T26·T27 | Human reading of 4 documents, ~65 lines total |
| G-06a·G-06c | `verify` behaviour check and eval |
| G-07a·G-07b·G-07c | Autonomy behaviour, corpus of >=10 missions, smoke. Floors in `docs/decisions/floors-and-limits.md` |
| G-08·G-09 | End-to-end async eval; flow comparative (downgraded to non-blocking) |
| G-T | Telemetry baseline — one week without JACU. **Without it no `stats` number can be read as a gain** |
| G-H | Harness metrics — escape, human time at apply, adversarial false positive. Sheet: `docs/evals/harness-metrics.md` |
| G-10a | Social pilot: someone who is not the owner installs from the docs alone |
| 003 T20 · 004 T19 | Live-path evals for clean-exit and preflight |
| Ratification | ADR-021, 022, 026, 027 — code in `main`, decision still `proposed` |
| P6·P7·P8 | Signed tag, asset verification, installable release report |
| ADR-028 | License confirmation, public repo creation and visibility flip, `v0.2.0` tag (release guard requires actor `ecouto`) |

## How the program proves itself

Not by feel. By the net-cost protocol, which **has not been written** and is a
prerequisite for any claim of gain: n, arms, task corpus, quality criterion and
statistical test. Becomes a task of 015.

## Living references

`docs/decisions/triggers.md` — what does not open without a signal
`docs/decisions/will-not-do.md` — what was already refused, and why
`docs/decisions/floors-and-limits.md` — what counts as passing
`docs/evals/` — the sheets the owner-only gates fill in
`docs/relatorios/` — closed-phase evidence, never rewritten
`docs/design/` — design inputs for queued SDDs, promoted into `docs/sdd/NNN-slug/` when each opens
