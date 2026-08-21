# Blocked-with-evidence

015 does not close 001-004 or 008 in this change. The living SDD files
are left in place. This document is the evidence.

## 001 native-sdd

T23 remains `blocked` (owner-present host-smoke; PROGRAM G-SDD). 015 does
not rewrite that row into an executor task.

```
go run ./cmd/jacu sdd close docs/sdd/001-native-sdd
# sdd close: archive required at docs/sdd/archive/001-native-sdd
# exit 2
```

With a temporary archive copy present:

```
go run ./cmd/jacu sdd close docs/sdd/001-native-sdd
# sdd close: clean exit failed
# exit 1
```

`cleanexit.Detect` fails closed when the current branch is not `main`.
This agent branch is `cursor/sdd-015-program-closeout-1d18`. Close on
`main` after a manual `git mv` remains owner work.

## 002 telemetry-v2

All 002 tasks are `done`. Close still needs a manual archive directory
and a clean-exit pass on `main`. The same two commands fail for the same
two reasons as 001. 015 does not `git mv` 002: `--all` skips `archive/`,
and close cannot exit 0 off `main`.

## 003 clean-exit

T20 remains `todo`: live-path eval to close SDD-002 and attach a receipt.
PROGRAM owner-only row `003 T20`. Not converted.

## 004 preflight

T19 remains `todo`: ten real missions with interruption counts.
PROGRAM owner-only row `004 T19`. Not converted.

## 008 audit-hardening

P6, P7, and P8 remain open (signed tag, asset verification, installable
release report). PROGRAM owner-only rows `P6·P7·P8`. Not converted.
