package clarity

import (
	"testing"

	"github.com/jacu-dev/jacu-harness/internal/capability/sdd"
)

const fixtureSDD = `---
sdd: 006-context-admission
---
# 006
## Write scope
**Allowed**
` + "```" + `
internal/capability/context/**
` + "```" + `
**Forbidden**
` + "```" + `
internal/mcpadapter/**
` + "```" + `
## Out of scope
- Summarising content
## Requirements
### Requirement: The pack is deterministic
Delta: ADDED
## Tasks
| # | Task | Files | Verify | Status | Evidence |
|---|---|---|---|---|---|
| T1 | Write ADR | docs/adr/ADR-024-context-admission.md | wc | todo | |
`

func parseFixture(t *testing.T) sdd.Document {
	t.Helper()
	document, err := sdd.Parse([]byte(fixtureSDD))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return document
}

func TestDivergeNamesWriteScopeForPathOutsideSpec(t *testing.T) {
	document := parseFixture(t)
	readback := Expected(document)
	readback.WriteScope = append(readback.WriteScope, "internal/mcpadapter/server.go")
	div := Diverge(document, readback)
	if len(div) == 0 {
		t.Fatal("expected a write_scope divergence")
	}
	found := false
	for _, item := range div {
		if item.Field == FieldWriteScope && item.Path == "internal/mcpadapter/server.go" {
			found = true
		}
	}
	if !found {
		t.Fatalf("divergences = %#v; want write_scope internal/mcpadapter/server.go", div)
	}
}

func TestDivergeAgreesWhenFieldsMatch(t *testing.T) {
	document := parseFixture(t)
	if div := Diverge(document, Expected(document)); len(div) != 0 {
		t.Fatalf("agreeing readback diverged: %#v", div)
	}
}
