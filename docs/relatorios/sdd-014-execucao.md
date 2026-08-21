# SDD-014 execution

ADR-031 (localhost viewer) and ADR-032 (embed / no runtime download) are
written. The Go factory maps the eight v1 blocks to byte-identical HTML.
Presentation markup is refused before render. `jacu report render` binds
no port. `jacu report serve` listens on 127.0.0.1 only. No new MCP tool.

Cold-start of `reportgen.HTML` on the golden fixture is recorded below.
The SPA embed is not frozen into the critical path.

Measured `TestHTMLColdStartIsUnderBudget` (five runs): 71µs, 33µs, 32µs,
30µs, 29µs. All under the 150ms floor. No `go:embed` of `web/dist/` is
frozen.

```sh
go test ./internal/reportgen ./cmd/jacu -race -count=1
go test -tags=e2e ./test/e2e/ -run Governed
```
