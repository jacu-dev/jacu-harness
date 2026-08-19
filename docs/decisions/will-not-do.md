# Will not do — refused decisions, with the reason

A record of what was already evaluated and **refused**. It exists so the same idea is not reopened three months later without new evidence.

## Cleanup that looks good and is not

**Deleting `.git/cursor/crepe` (~10.4 MiB):** refused for JACU automation. It is Cursor's property, not JACU's. Storage inspection may report it as excluded external use; it must never remove it.

**Deleting dangling unreachable git objects (~200 KiB):** refused as automatic cleanup. Ordinary git maintenance only, after human review of recovery.

**Deleting `.jacu/bin/jacu` (~10 MiB):** refused. It is an intentional local development binary, ignored and overwritten — not versioned.

**Deleting or compacting history (`docs/heranca`, execution reports, SDD archive):** refused. It is intentional evidence, small relative to the binary and the cache, and cannot be rewritten as living truth.

**Deleting dirty or reviewed orphan worktrees or runs:** refused as automatic behaviour. They may contain recoverable work. Storage reports and routes to explicit recover or discard.

## Quality that would be theatre

**Coverage-percentage gate:** refused. The baseline already passes hundreds of tests across packages; a percentage target with no missing behaviour identified by risk is gameable. Directed regressions on actually discovered failure paths instead.

**Automatic memory TTL / automatic database migration:** refused for now. The store had 5 records. ADR-016 keeps JSON until measured recall quality justifies it.

## False positives already investigated

**"Gitleaks 8.30.1 detects nothing":** refused. Maintainers showed the published reproduction was filtered on purpose by the stopword `abcdefghijklmnopqrstuvwxyz`; a non-placeholder secret matched normally.

**Temporary-directory leak on the normal path:** not found. Scratch paths for the review-index and verify have deferred cleanup and no residue was present.

## Out by decision, with no plan

**HTTP/OAuth, embeddings, generic model-router:** out. Any of them enters only with an eval proving need **and** its own ADR.

This file is not an invitation. It is the list of what has already been decided not to do.
