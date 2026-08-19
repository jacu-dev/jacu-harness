## What

<!-- One paragraph: the behavior change, not the diff. Link the issue or ADR/SDD it serves. -->

## Verification

<!-- Paste the command you ran locally. `verify / verify` must be green. -->

```sh
bash scripts/verify.sh
go test ./... -race
```

## Checklist

- [ ] Commits follow Conventional Commits 1.0.0, English, imperative, subject ≤ 72 chars.
- [ ] No authorship traces (CONTRIBUTING → Provenance; CI refuses them). Check with `go run ./cmd/jacu provenance --history origin/main..HEAD --json`.
- [ ] No new MCP tool. New capability is a CLI subcommand (CONTRIBUTING.md).
- [ ] No network, credential or deploy code paths introduced (README boundary).
- [ ] Behavior changes are covered by a test; security-relevant paths fail closed.
- [ ] Docs updated where the change is observable (README, docs/, skills/).
