# Cursor and cloud VMs

JACU is a stdio MCP server. The binary must live on the machine that holds
the repository. Cursor, Claude Code, Codex and OpenCode only register
`jacu serve`; they do not download it.

## In this repository

```bash
bash .cursor/install.sh --from-source
```

That wrapper runs `scripts/cloud-install.sh`. After `v0.2.0` exists, prefer
the signed binary (github.com egress only):

```bash
bash .cursor/install.sh --version v0.2.0
```

Then register the host pack without editing an unnamed config:

```bash
jacu init --host cursor
# or, to write a file you named:
jacu init --host cursor --config "$HOME/.cursor/mcp.json"
```

## Restricted egress

Release mode talks only to `github.com` (tarball, checksums, Sigstore
bundle). If a download fails, the installer names the unreachable host.
`--from-source` also needs the Go module proxy.

## What this is not

There is no `curl | sh` bootstrap, no host API call, and no binary network
capability. A Cursor Cloud Agent can build from a checkout today; the
verified one-command path is `install.sh --version` after the owner tags
`v0.2.0`.
