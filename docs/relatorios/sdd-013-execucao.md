# SDD-013 execution

`internal/modelcontrol` now has callers in orchestration and runner.
Nodes declare `lane`. Spawn is attested and fail-closed. `cost.trace` is
a v2 event. Gemini stays off the panel.

```sh
go test ./internal/capability/orchestration ./internal/modelcontrol ./internal/runner ./internal/telemetry -race
go test ./test/hosteval/ -run 'TestCatalogue|TestTruncating|TestEmptyHost|TestGemini'
bash scripts/hosteval.sh
```
