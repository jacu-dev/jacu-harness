# Contributing

## Verify

```sh
bash scripts/verify.sh
go test ./... -race
```

PRs must keep the `verify / verify` check green.

## Commits

Conventional Commits 1.0.0, English only, imperative subject ≤ 72 characters.

Allowed types: `feat`, `fix`, `docs`, `style`, `refactor`, `test`, `chore`, `ci`, `build`, `perf`, `revert`.

## Provenance

Public history has a single author identity. Authorship traces are refused by CI:

- AI `Co-Authored-By` trailers
- `noreply@anthropic.com` and `cursoragent@cursor.com`
- the phrase `Generated with`
- the robot emoji

Host names such as Claude, Codex and Cursor are product domain, not traces.

Do not add a new MCP tool. New capability enters as a CLI subcommand.
