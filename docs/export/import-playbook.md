# Public import playbook

ADR-028: a new public repository with curated Conventional Commits, brand
JACU, binary `jacu`. The former private repository is archived and is never
rewritten.

This file is the in-tree record of that import. It does not rewrite git
history. It does not archive a repository that this checkout does not own.

## What already happened

`github.com/jacu-dev/jacu-harness` is the public repository. The living tree
on `main` is English, module `github.com/jacu-dev/jacu-harness`, CI vendored
at `.github/workflows/verify.yml`, check name `verify / verify`.

The first public pull requests that ran that vendored gate green include
#20 through #28 (SDD-009 through SDD-015 stacked onto `main`).

Do not `git filter-repo`, `git rebase` of published `main`, or force-push
`main` to invent a smaller import series after the fact.

## Intended curated series

`internal/export` holds the area-by-area Conventional Commit plan ADR-028
described. Print it:

```
jacu provenance --commit-plan --json
```

Every subject in that plan is English and matches `provenance-lint`. Use the
plan when documenting a fresh export. Do not apply it onto this repository's
existing `main` history.

## Trace scan

```
jacu provenance --files --json
jacu provenance --history origin/main..HEAD --json
```

A clean report is zero `trace` findings. Host names (Claude, Codex, Cursor)
in product docs are `product`, not traces.

## Owner leftover

1. Copy `docs/export/ARCHIVED.md` to the former private repository root.
2. Archive that GitHub repository as read-only.
3. Do not rewrite the private history.

Those three steps are not agent work. This public tree only ships the
template and the playbook.
