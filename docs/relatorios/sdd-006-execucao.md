# SDD-006 execution

Context admission is CLI `jacu context pack|explain` plus a compile-time
ledger gate. Required overflow refuses before dispatch. Packs are
deterministic, content-hashed, and not persisted. MCP catalogue unchanged.

```sh
go test ./internal/capability/context ./internal/capability/ledger ./internal/capability/missioncompile ./cmd/jacu -race
```
