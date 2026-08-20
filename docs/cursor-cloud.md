# Cursor and cloud VMs

JACU is a stdio MCP server. The binary must live on the machine that holds
the repository. Cursor, Claude Code, Codex and OpenCode only register
`jacu serve`; they do not download it.

## In this repository

```bash
bash .cursor/install.sh --from-source
```

That wrapper runs `scripts/cloud-install.sh`. Prefer a signed release over
building from source when the VM can reach github.com:

```bash
bash .cursor/install.sh --version vX.Y.Z
```

Cursor cloud agents read `.cursor/environment.json`, not `install.sh`. It runs
`scripts/dev-setup.sh --phase image` at build time and `--phase session` per
session; the install above happens inside the image phase. See
[SDD-018](sdd/018-cloud-dev-environment/sdd.md).

Then register the host pack without editing an unnamed config:

```bash
jacu init --host cursor
# or, to write a file you named:
jacu init --host cursor --config "$HOME/.cursor/mcp.json"
```

`jacu init --config` rewrites a leftover host pack that still names the
retired binary so the **local IDE** launches `jacu serve`. On an agent VM the
image phase of `scripts/dev-setup.sh` does that repair for
`~/.cursor/mcp.json` without dropping sibling servers.

That file is not what a hosted Cloud Agent uses. Cursor Cloud Agents load
stdio MCP from the [Cloud Agents MCP dropdown](https://cursor.com/agents)
and, on a Team plan, from
[Dashboard → Integrations & MCP](https://cursor.com/dashboard/integrations).
They do not inherit `~/.cursor/mcp.json` from your Mac.

To stop `spawn jacu-mcp ENOENT` on Cloud Agents:

1. Open the MCP dropdown at [cursor.com/agents](https://cursor.com/agents).
2. Delete or disable every stdio server whose command is `jacu-mcp`.
3. If you still want JACU in the cloud VM, add a server named `jacu` with
   command `jacu` and args `["serve"]`. The VM must have `jacu` on PATH
   (this repository's image phase installs it).
4. On a Team plan, repeat the same edit under Team MCP Servers.
5. Start a **new** Cloud Agent. An already-running session keeps the old
   spawn command.

On the Mac, also confirm the local file has no leftover server key:

```bash
python3 - <<'PY'
import json
from pathlib import Path
data = json.loads(Path.home().joinpath(".cursor/mcp.json").read_text())
print(sorted((data.get("mcpServers") or {}).keys()))
PY
```

`jacu` 0.3.0's `init` can add `jacu serve` **and leave** a `jacu-mcp` key in
place. Different names both load. Delete the retired key by hand, or use a
build that rewrites it.

## Restricted egress

Release mode talks only to `github.com` (tarball, checksums, Sigstore
bundle). If a download fails, the installer names the unreachable host.
`--from-source` also needs the Go module proxy.

## What this is not

There is no `curl | sh` bootstrap, no host API call, and no binary network
capability. A Cursor Cloud Agent can build from a checkout, or install a
signed release with `install.sh --version` once a tag is published.
