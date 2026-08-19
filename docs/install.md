# Local installation

## Stable build

With a clean, up-to-date `main`, build the binary into a `PATH` directory and stamp the SHA on the executable:

```bash
git switch main
git pull --ff-only
sha=$(git rev-parse HEAD)
mkdir -p "$HOME/.local/bin"
go build -trimpath -ldflags "-X main.Version=$sha" \
  -o "$HOME/.local/bin/jacu" ./cmd/jacu
jacu version
```

The MCP command is always `$HOME/.local/bin/jacu serve`.

## Per-user registration

Claude Code and Codex:

```bash
claude mcp add --scope user jacu -- "$HOME/.local/bin/jacu" serve
codex mcp add jacu -- "$HOME/.local/bin/jacu" serve
```

Prefer `jacu doctor --emit <host>` for a host pack, and `jacu init` (SDD-017) once that change lands.

See also `docs/distribution.md`.
