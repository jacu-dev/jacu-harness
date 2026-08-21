# SDD-015 execution

Closeout packet is written. Owner-only gates stay owner-only. ADR status
bytes are untouched. Living 009-015 SDD files have zero Portuguese
stopword hits from the SDD heuristic. Net-cost protocol exists and makes
no gain claim.

`sdd close` on 001 and 002 refused: missing archive, then clean-exit
failure off `main`. Evidence is `docs/sdd/015-program-closeout/blocked.md`.

```sh
go run ./cmd/jacu sdd lint docs/sdd/015-program-closeout
go run ./cmd/jacu sdd lint --all
test -f docs/sdd/015-program-closeout/ratification.md
test -f docs/sdd/015-program-closeout/net-cost-protocol.md
```
