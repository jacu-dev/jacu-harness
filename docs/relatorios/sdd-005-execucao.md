# SDD-005 execution

Clarity gate landed as CLI-only `jacu clarity probe|ingest|verdict`.

- Closed readback schema; prose and unknown fields refused.
- Divergence names the field; write-scope extras name the path.
- Three-run variance fails even when each run matches the spec globs.
- Rewrite rounds cannot grow spec bytes.
- `clarity.probe` is a v2 event with closed counts/enums only.
- MCP catalogue is unchanged (13 tools).

Verification:

```sh
go test ./internal/capability/clarity ./internal/telemetry ./cmd/jacu -race
go test ./internal/mcpadapter -run Skills -race
go run ./cmd/jacu sdd lint docs/sdd/005-clarity-gate
```

Owner still ratifies ADR-023. The host, not JACU, runs the cheap probe model.
