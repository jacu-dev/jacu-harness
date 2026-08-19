# ADR-026 — Audited one-shot delivery

- Status: proposed; owner ratification required
- Date: 2026-08-15
- Scope: SDD-008 audit hardening execution and promotion

## Decision

The audit hardening program is delivered through one branch and one pull
request, but authority is deliberately split across four stages:

1. **Preparation** writes and locks the SDD, its ADRs and the audit plans.
2. **Local execution** may modify only the SDD write scope and create local
   Conventional Commits. It must not push, open or modify a pull request,
   change hosted repository settings, merge, create a tag or publish a release.
3. **Independent validation** reviews the complete local commit series and
   diff, reruns every required local gate from a clean tree, and records a
   validation receipt. Executor-provided evidence is useful input but is not a
   substitute for fresh validation.
4. **Promotion** starts only after independent validation passes. It pushes the
   reviewed branch, opens one pull request, proves the required hosted checks on
   the final SHA, closes every actionable review thread in its original
   discussion, and preserves merge and production-tag decisions as owner gates.

The executor runs tasks in dependency order with one task in flight. A failing
gate blocks that task and its dependants; independent tasks may continue. Every
behavioral change starts with a failing regression test committed separately
from the implementation that makes it pass. Documentation-only and mechanical
configuration changes may use one focused commit when a meaningful RED test does
not exist.

The handoff is acceptable only when the working tree is clean, every completed
task has exact command output, every incomplete or human-gated task is named,
and no out-of-scope file was changed. Any unresolved blocker prevents promotion.

Local preflight, hosted CI, review closure, merge, release publication and
installation smoke are distinct states. No earlier state is evidence for a
later state. The production release is created only by the owner's signed
`v*` tag through the repository release workflow. The published result is not
called installable until the release contains the platform archive,
`checksums.txt` and `checksums.txt.sigstore.json`, and an independent clean-target
installation verifies the Sigstore bundle and checksum before executing the
binary.

The live ruleset update identified by SDD-008 is a promotion-stage mutation, not
an executor task. Before changing it, the publisher captures the complete live
ruleset and proves that the only intended semantic delta is the required status
context set. All targeting, bypass, review, deletion, force-push and conversation
resolution settings are preserved.

## Consequences

- The executor can complete a long remediation pass without serial approval
  requests while remaining unable to publish or promote its own work.
- A single branch and pull request keep cross-module invariants reviewable, but
  the commit series preserves RED/GREEN and module-level rollback boundaries.
- Promotion may stop after local validation, hosted CI, review or merge without
  falsely claiming production completion.
- Owner ratification is required before the local execution stage starts and
  again for merge and the signed production tag.
- A blocked release gate does not invalidate completed local work; it remains a
  precisely named external gate.

## Alternatives rejected

- **One unrestricted executor through production:** rejected because the same
  writer would implement, validate and approve external publication.
- **Sixteen unrelated branches and pull requests:** rejected for this program
  because the retention, storage and living-document changes have deliberate
  cross-module dependencies and the owner requested a one-shot execution.
- **One unstructured mega-commit:** rejected because it removes regression,
  review and rollback boundaries.
- **Automatic merge or tag after green CI:** rejected because CI does not prove
  review closure, owner approval, release identity or installation integrity.
- **Treating a dry-run release as production:** rejected because the repository
  workflow publishes only from the authorized `v*` tag path.
